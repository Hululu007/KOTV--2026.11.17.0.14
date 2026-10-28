// Package danmaku 弹幕的拉取、解析与搜索。前端只负责渲染，弹幕正文一律经引擎解析。
package danmaku

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/spider"
	"github.com/bobo/KOTV/internal/util"
)

// Item 单条弹幕。
type Item struct {
	Time    float64 `json:"time"` // 秒
	Mode    int     `json:"mode"`
	Size    int     `json:"size"`
	Color   int     `json:"color"`
	Content string  `json:"content"`
}

const (
	itemsMax     = 20000   // 单次解析条数上限
	bodyBytesMax = 8 << 20 // 读入字节上限（解码后同样拒绝更大正文）

	defaultMode  = 1
	defaultSize  = 25
	defaultColor = 0xFFFFFF
)

// localExts 引擎本机弹幕文件只认这些后缀，避免借接口读其它文件。
var localExts = map[string]bool{".xml": true, ".json": true, ".txt": true}

// Load 拉取并解析一个弹幕源：http(s) 地址或引擎本机弹幕文件。
// 正文若是源列表 [{name,url}]，跟进第一条。
func Load(ctx context.Context, src string) ([]Item, error) {
	body, err := loadBody(ctx, src)
	if err != nil {
		return nil, err
	}
	if sources := ParseSources(body); len(sources) > 0 {
		if body, err = loadBody(ctx, sources[0].URL); err != nil {
			return nil, err
		}
	}
	return Parse(body), nil
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
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("弹幕搜索接口返回为空")
	}
	return ParseSources(body), nil
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
	if m := textLineRe.FindStringSubmatch(text); m != nil {
		if it, ok := itemFromMeta(m[1], m[2]); ok {
			return it
		}
	}
	return Item{Mode: defaultMode, Size: defaultSize, Color: defaultColor, Content: text}
}

// Parse 解析 B 站 XML 或 [t,mode,size,color]text 格式。
func Parse(body string) []Item {
	body = strings.TrimSpace(body)
	if strings.Contains(body, "<d ") || strings.Contains(body, "<i>") {
		return parseXML(body)
	}
	return parseText(body)
}

type danmakuD struct {
	P    string `xml:"p,attr"`
	Text string `xml:",chardata"`
}

func parseXML(body string) []Item {
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var out []Item
	sawD := false
	for len(out) < itemsMax {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "d" {
			continue
		}
		var d danmakuD
		if err := dec.DecodeElement(&d, &se); err != nil {
			break
		}
		sawD = true
		if it, ok := itemFromMeta(d.P, d.Text); ok {
			out = append(out, it)
		}
	}
	if !sawD {
		return parseText(body)
	}
	return out
}

// itemFromMeta meta 为「时间,模式,字号,颜色,…」，缺项或解析失败用默认值。
func itemFromMeta(meta, text string) (Item, bool) {
	it := Item{Content: strings.TrimSpace(text), Mode: defaultMode, Size: defaultSize, Color: defaultColor}
	if it.Content == "" {
		return Item{}, false
	}
	parts := strings.Split(meta, ",")
	if v, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err == nil {
		it.Time = v
	}
	for i, dst := range []*int{&it.Mode, &it.Size, &it.Color} {
		if len(parts) <= i+1 {
			break
		}
		if v, err := strconv.Atoi(strings.TrimSpace(parts[i+1])); err == nil {
			*dst = v
		}
	}
	return it, true
}

var textLineRe = regexp.MustCompile(`^\[([^\]]+)\](.*)$`)

func parseText(body string) []Item {
	var out []Item
	for _, line := range strings.Split(body, "\n") {
		if len(out) >= itemsMax {
			break
		}
		m := textLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if it, ok := itemFromMeta(m[1], m[2]); ok {
			out = append(out, it)
		}
	}
	return out
}

func loadBody(ctx context.Context, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("弹幕地址为空")
	}
	if local, ok := util.FileURLPath(src); ok {
		return readLocal(local)
	}
	if !strings.Contains(src, "://") {
		return readLocal(src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", err
	}
	return doRequest(req)
}

func doRequest(req *http.Request) (string, error) {
	req.Header.Set("User-Agent", util.OkHttpUA)
	resp, err := util.GetClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("弹幕请求失败: http=%d", resp.StatusCode)
	}
	b := util.DecodeContentEncoding(resp.Header.Get("Content-Encoding"), raw)
	if len(b) > bodyBytesMax {
		return "", errBodyTooLarge()
	}
	return string(b), nil
}

func readLocal(path string) (string, error) {
	if !filepath.IsAbs(path) || !localExts[strings.ToLower(filepath.Ext(path))] {
		return "", fmt.Errorf("不是弹幕文件: %s", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() > bodyBytesMax {
		return "", errBodyTooLarge()
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := readLimited(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
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
