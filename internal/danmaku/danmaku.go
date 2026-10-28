// Package danmaku 弹幕的拉取、解析与搜索。前端只负责渲染，弹幕正文一律经引擎解析。
package danmaku

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/spider"
	"github.com/bobo/KOTV/internal/util"
)

// Item 单条弹幕。
type Item struct {
	Time    float64 `json:"time"` // 秒
	Mode    int     `json:"mode"` // ModeScroll / ModeBottom / ModeTop / ModeReverse / ModePositioned
	Size    int     `json:"size"` // B 站字号：18 以下小号，36 以上大号，其余默认
	Color   int     `json:"color"`
	Content string  `json:"content"`
	Pool    int     `json:"pool,omitempty"`
	// 定位弹幕：画面内相对坐标 0~1，停留时长。
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	DurationMs int64   `json:"durationMs,omitempty"`
}

const (
	itemsMax     = 200000   // 单个弹幕源条数上限（一集常见数万条）
	bodyBytesMax = 32 << 20 // 读入字节上限（解码后同样拒绝更大正文）
)

// localExts 引擎本机弹幕文件只认这些后缀，避免借接口读其它文件。
var localExts = map[string]bool{".xml": true, ".json": true, ".txt": true}

// Load 拉取并解析一个弹幕源：B 站/爱奇艺/芒果/腾讯/优酷视频页按平台分段拉取，
// 其它 http(s) 地址或引擎本机弹幕文件按内容识别格式。durationMs 为片长，未知传 0（由平台接口补）。
func Load(ctx context.Context, src string, durationMs int64) ([]Item, error) {
	src = strings.TrimSpace(src)
	if f := fetcherFor(src); f != nil {
		return fetchAll(ctx, f, src, durationMs)
	}
	body, err := loadBody(ctx, src)
	if err != nil {
		return nil, err
	}
	return Parse(body)
}

type searchSlot struct{ cancel context.CancelFunc }

var (
	searchMu sync.Mutex
	searches = map[string]*searchSlot{}
)

// Search 调弹幕搜索接口，返回 [{name,url}]。片名、集名先繁转简；
// 接口地址含 {name}/{episode} 时代入后 GET，否则 POST 表单 name、episode。
// 同一 scope 发起新搜索时取消上一次。
func Search(ctx context.Context, scope, api, name, episode string) ([]model.DanmakuItem, error) {
	api = strings.TrimSpace(api)
	if api == "" {
		return nil, errors.New("未设置弹幕搜索接口")
	}
	ctx, cancel := context.WithCancel(ctx)
	slot := &searchSlot{cancel: cancel}
	searchMu.Lock()
	if prev := searches[scope]; prev != nil {
		prev.cancel()
	}
	searches[scope] = slot
	searchMu.Unlock()
	defer func() {
		cancel()
		searchMu.Lock()
		if searches[scope] == slot {
			delete(searches, scope)
		}
		searchMu.Unlock()
	}()

	req, err := searchRequest(ctx, api, spider.T2S(strings.TrimSpace(name)), spider.T2S(strings.TrimSpace(episode)))
	if err != nil {
		return nil, err
	}
	body, err := doRequest(req)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("弹幕搜索接口返回为空")
	}
	return resolveLoopback(ParseSources(string(body)), req.URL), nil
}

// resolveLoopback 搜索接口给的 127.0.0.1/localhost 地址指接口所在那台机器（接口通常是那台的
// /proxy?do=danmaku）。接口不在本机时换成接口的主机，否则引擎会去请求自己。
func resolveLoopback(items []model.DanmakuItem, api *url.URL) []model.DanmakuItem {
	if api == nil || isLoopbackHost(api.Hostname()) {
		return items
	}
	for i, it := range items {
		u, err := url.Parse(it.URL)
		if err != nil || !isLoopbackHost(u.Hostname()) {
			continue
		}
		host := api.Hostname()
		if p := u.Port(); p != "" {
			host = net.JoinHostPort(host, p)
		} else if p := api.Port(); p != "" {
			host = net.JoinHostPort(host, p)
		}
		u.Host = host
		items[i].URL = u.String()
	}
	return items
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func searchRequest(ctx context.Context, api, name, episode string) (*http.Request, error) {
	if strings.Contains(api, "{name}") || strings.Contains(api, "{episode}") {
		u := strings.NewReplacer("{name}", queryEscape(name), "{episode}", queryEscape(episode)).Replace(api)
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	}
	form := url.Values{"name": {name}, "episode": {episode}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

// queryEscape 空格编成 %20（不是 +），模板地址里放在路径段或查询串都能用。
func queryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// Merge 自动搜到的一条弹幕源并入片源弹幕，返回源列表和默认选中项。
// add=true（片源优先且片源有弹幕）：追加在后面，仍选片源第一条；
// 否则选中它，不在列表里时放到第一位。
func Merge(spiderSources []model.DanmakuItem, found model.DanmakuItem, add bool) ([]model.DanmakuItem, int) {
	out := append([]model.DanmakuItem(nil), spiderSources...)
	if strings.TrimSpace(found.URL) == "" {
		return out, 0
	}
	idx := -1
	for i, s := range out {
		if s.URL == found.URL {
			idx = i
			break
		}
	}
	switch {
	case add && idx < 0:
		return append(out, found), 0
	case add:
		return out, 0
	case idx >= 0:
		return out, idx
	default:
		return append([]model.DanmakuItem{found}, out...), 0
	}
}

// ParseSources 解析源列表：[{name|title,url}]、["url"] 或单个 {name,url}；不是源列表时返回空。
func ParseSources(body string) []model.DanmakuItem {
	t := strings.TrimSpace(body)
	if t == "" || (t[0] != '[' && t[0] != '{') {
		return nil
	}
	var raw any
	if err := json.Unmarshal([]byte(t), &raw); err != nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		list = []any{raw}
	}
	var out []model.DanmakuItem
	for _, v := range list {
		var it model.DanmakuItem
		switch x := v.(type) {
		case string:
			it.URL = strings.TrimSpace(x)
		case map[string]any:
			it.URL = strings.TrimSpace(fmt.Sprint(valueOr(x["url"], "")))
			it.Name = strings.TrimSpace(fmt.Sprint(valueOr(x["name"], valueOr(x["title"], ""))))
		}
		if it.URL == "" {
			continue
		}
		if it.Name == "" {
			it.Name = fmt.Sprintf("源 %d", len(out)+1)
		}
		out = append(out, it)
	}
	return out
}

func valueOr(v, def any) any {
	if v == nil {
		return def
	}
	return v
}

// ParseLive 遥控即时弹幕：网页端发 [t,mode,size,color]text，也可能只有正文。
func ParseLive(text string) Item {
	text = strings.TrimSpace(text)
	if it, ok := parseTxtLine(text); ok {
		return it
	}
	return Item{Mode: ModeScroll, Size: defaultSize, Color: defaultColor, Content: text}
}

func loadBody(ctx context.Context, src string) ([]byte, error) {
	if src == "" {
		return nil, errors.New("弹幕地址为空")
	}
	if local, ok := util.FileURLPath(src); ok {
		return readLocal(local)
	}
	if !strings.Contains(src, "://") {
		return readLocal(src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	return doRequest(req)
}

func doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("User-Agent", util.OkHttpUA)
	resp, err := util.GetClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("弹幕请求失败: http=%d", resp.StatusCode)
	}
	b := util.DecodeContentEncoding(resp.Header.Get("Content-Encoding"), raw)
	if len(b) > bodyBytesMax {
		return nil, errBodyTooLarge()
	}
	return b, nil
}

func readLocal(path string) ([]byte, error) {
	if !filepath.IsAbs(path) || !localExts[strings.ToLower(filepath.Ext(path))] {
		return nil, fmt.Errorf("不是弹幕文件: %s", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > bodyBytesMax {
		return nil, errBodyTooLarge()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLimited(f)
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, bodyBytesMax+1))
	if err != nil {
		return nil, err
	}
	if len(b) > bodyBytesMax {
		return nil, errBodyTooLarge()
	}
	return b, nil
}

func errBodyTooLarge() error {
	return fmt.Errorf("弹幕正文超过 %d 字节", bodyBytesMax)
}
