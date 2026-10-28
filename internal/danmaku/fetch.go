package danmaku

import (
	"bytes"
	"cmp"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/bobo/KOTV/internal/util"
)

// 视频页地址（B 站 / 爱奇艺 / 芒果 / 腾讯 / 优酷）不是弹幕文件：按平台接口每 60 秒一段拉回。
const (
	segmentMs       = 60_000
	segmentsMax     = 360 // 6 小时
	segmentParallel = 4
	browserUA       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"
)

type fetchSession interface {
	segments() int
	fetch(ctx context.Context, n int) ([]Item, error) // n 从 1 开始
}

type fetcher struct {
	accepts func(host string) bool
	prepare func(ctx context.Context, page string, durationMs int64) (fetchSession, error)
}

var fetchers = []fetcher{
	{func(h string) bool { return h == "www.bilibili.com" || h == "m.bilibili.com" }, prepareBili},
	{func(h string) bool { return strings.HasSuffix(h, ".iqiyi.com") }, prepareIQIYI},
	{func(h string) bool { return strings.HasSuffix(h, ".mgtv.com") }, prepareMGTV},
	{func(h string) bool { return h == "v.qq.com" || strings.HasSuffix(h, ".v.qq.com") }, prepareQQ},
	{func(h string) bool { return strings.HasSuffix(h, ".youku.com") }, prepareYouku},
}

func fetcherFor(src string) *fetcher {
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	for i := range fetchers {
		if fetchers[i].accepts(u.Hostname()) {
			return &fetchers[i]
		}
	}
	return nil
}

// fetchAll 准备会话后并发拉全部分段；个别分段失败不影响其它段，全部失败才报错。
func fetchAll(ctx context.Context, f *fetcher, page string, durationMs int64) ([]Item, error) {
	s, err := f.prepare(ctx, page, durationMs)
	if err != nil {
		return nil, err
	}
	n := s.segments()
	if n <= 0 {
		return nil, fmt.Errorf("弹幕分段数无效: %d", n)
	}
	n = min(n, segmentsMax)
	results := make([][]Item, n)
	errs := make([]error, n)
	sem := make(chan struct{}, segmentParallel)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i], errs[i] = s.fetch(ctx, i+1)
		}(i)
	}
	wg.Wait()
	var out []Item
	var firstErr error
	ok := false
	for i := range results {
		if errs[i] != nil {
			firstErr = cmp.Or(firstErr, errs[i])
			continue
		}
		ok = true
		out = append(out, results[i]...)
	}
	if !ok {
		return nil, firstErr
	}
	sortByTime(out)
	return capItems(out), nil
}

func segmentCount(durationSec float64) int {
	return int(math.Ceil(durationSec / 60))
}

// ---- HTTP ----

func platformHeaders(origin string) map[string]string {
	return map[string]string{"Origin": origin, "Referer": origin + "/", "User-Agent": browserUA}
}

func fetchBytes(ctx context.Context, client *http.Client, method, u string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if client == nil {
		client = util.GetClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, u)
	}
	return util.DecodeContentEncoding(resp.Header.Get("Content-Encoding"), raw), nil
}

func fetchString(ctx context.Context, u string, headers map[string]string) (string, error) {
	b, err := fetchBytes(ctx, nil, http.MethodGet, u, nil, headers)
	return string(b), err
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func firstGroup(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// ---- B 站：页面里取 cid，整部一段 XML ----

var biliCidRe = regexp.MustCompile(`"cid"\s*:\s*(\d+)`)

type biliSession struct{ cid string }

func prepareBili(ctx context.Context, page string, _ int64) (fetchSession, error) {
	html, err := fetchString(ctx, page, platformHeaders("https://www.bilibili.com"))
	if err != nil {
		return nil, err
	}
	cid := firstGroup(biliCidRe, html)
	if cid == "" {
		return nil, fmt.Errorf("B 站页面里找不到 cid: %s", page)
	}
	return biliSession{cid}, nil
}

func (s biliSession) segments() int { return 1 }

func (s biliSession) fetch(ctx context.Context, _ int) ([]Item, error) {
	b, err := fetchBytes(ctx, nil, http.MethodGet, "https://comment.bilibili.com/"+s.cid+".xml", nil, platformHeaders("https://www.bilibili.com"))
	if err != nil {
		return nil, err
	}
	return parseBili(b)
}

// ---- 腾讯视频 ----

var (
	qqVidQueryRe = regexp.MustCompile(`[?&]vid=([^&]+)`)
	qqVidPathRe  = regexp.MustCompile(`/([^/]+)\.html`)
	qqDurationRe = regexp.MustCompile(`"duration":"(\d+)"`)
)

const qqInfoURL = "https://union.video.qq.com/fcgi-bin/data?otype=json&tid=1804&appid=20001238&appkey=6c03bbe9658448a4&union_platform=1&idlist="

type qqSession struct {
	vid   string
	count int
}

func prepareQQ(ctx context.Context, page string, durationMs int64) (fetchSession, error) {
	vid := firstGroup(qqVidQueryRe, page)
	if vid == "" {
		vid = firstGroup(qqVidPathRe, page)
	}
	if vid == "" {
		return nil, fmt.Errorf("腾讯视频地址里找不到 vid: %s", page)
	}
	sec := durationMs / 1000
	if sec <= 0 {
		info, err := fetchString(ctx, qqInfoURL+vid, platformHeaders("https://v.qq.com"))
		if err != nil {
			return nil, err
		}
		sec, _ = strconv.ParseInt(firstGroup(qqDurationRe, info), 10, 64)
		if sec <= 0 {
			return nil, fmt.Errorf("腾讯视频取不到时长: %s", vid)
		}
	}
	return qqSession{vid, segmentCount(float64(sec))}, nil
}

func (s qqSession) segments() int { return s.count }

func (s qqSession) fetch(ctx context.Context, n int) ([]Item, error) {
	start := int64(n-1) * segmentMs
	u := fmt.Sprintf("https://dm.video.qq.com/barrage/segment/%s/t/v1/%d/%d", s.vid, start, start+segmentMs)
	b, err := fetchBytes(ctx, nil, http.MethodGet, u, nil, platformHeaders("https://v.qq.com"))
	if err != nil {
		return nil, err
	}
	return parseQQ(b)
}

// ---- 爱奇艺：accelerator.js 取 tvid，分段为 brotli 压缩的 protobuf ----

var (
	iqiyiTvidRe     = regexp.MustCompile(`(?:"tvId"|"tv_id")\s*:\s*"?(\d{8,})"?`)
	iqiyiDurationRe = regexp.MustCompile(`"videoDuration"\s*:\s*"?(\d+)"?`)
)

type iqiyiSession struct {
	tvid  string
	count int
}

func prepareIQIYI(ctx context.Context, page string, durationMs int64) (fetchSession, error) {
	headers := platformHeaders("https://www.iqiyi.com")
	if strings.Contains(page, "mesh.if.iqiyi.com") {
		body, err := fetchString(ctx, page, headers)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Data struct {
				PageURL string `json:"pageurl_iqiyi_pc"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(body), &doc) != nil || doc.Data.PageURL == "" {
			return nil, fmt.Errorf("解析不了爱奇艺跳转地址: %s", page)
		}
		page = doc.Data.PageURL
	}
	headers["Referer"] = page
	js, err := fetchString(ctx, "https://mesh.if.iqiyi.com/player/lw/lwplay/accelerator.js?apiVer=3", headers)
	if err != nil {
		return nil, err
	}
	tvid := firstGroup(iqiyiTvidRe, js)
	if len(tvid) < 4 {
		return nil, errors.New("爱奇艺 accelerator.js 里找不到 tvid")
	}
	sec := durationMs / 1000
	if sec <= 0 {
		sec, _ = strconv.ParseInt(firstGroup(iqiyiDurationRe, js), 10, 64)
		if sec <= 0 {
			return nil, errors.New("爱奇艺取不到时长")
		}
	}
	return iqiyiSession{tvid, max(1, segmentCount(float64(sec)))}, nil
}

func (s iqiyiSession) segments() int { return s.count }

func (s iqiyiSession) fetch(ctx context.Context, n int) ([]Item, error) {
	t := s.tvid
	token := md5Hex(fmt.Sprintf("%s_60_%dcbzuw1259a", t, n))
	u := fmt.Sprintf("https://cmts.iqiyi.com/bullet/%s/%s/%s_60_%d_%s.br", t[len(t)-4:len(t)-2], t[len(t)-2:], t, n, token[len(token)-8:])
	b, err := fetchBytes(ctx, nil, http.MethodGet, u, nil, platformHeaders("https://www.iqiyi.com"))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(b)), bodyBytesMax))
	if err != nil {
		return nil, err
	}
	return parseIQIYI(raw)
}

// ---- 芒果 TV ----

var (
	mgtvURLRe      = regexp.MustCompile(`mgtv\.com/b/(\d+)/(\d+)\.html`)
	mgtvDurationRe = regexp.MustCompile(`(\d{1,2}:\d{2}:\d{2}|\d+:\d{2})`)
)

// mgtvNuxtBlock 播放页 <script>window.__NUXT__=…</script> 的内容（最多 51200 字）。
func mgtvNuxtBlock(html string) string {
	const marker = ">window.__NUXT__="
	i := strings.Index(html, marker)
	if i < 0 {
		return ""
	}
	rest := html[i+len(marker):]
	end := strings.Index(rest, "</script>")
	if end <= 0 || end > 51200 {
		return ""
	}
	return rest[:end]
}

type mgtvSession struct {
	vid, cid string
	count    int
	cdnBase  string // 空时走 opbarrage 接口
}

func prepareMGTV(ctx context.Context, page string, durationMs int64) (fetchSession, error) {
	m := mgtvURLRe.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("不是芒果 TV 播放页: %s", page)
	}
	cid, vid := m[1], m[2]
	headers := platformHeaders("https://www.mgtv.com")
	sec := durationMs / 1000
	if sec <= 0 {
		html, err := fetchString(ctx, page, headers)
		if err != nil {
			return nil, err
		}
		sec = clockSeconds(firstGroup(mgtvDurationRe, mgtvNuxtBlock(html)))
		if sec <= 0 {
			return nil, fmt.Errorf("芒果 TV 页面里取不到时长: %s", page)
		}
	}
	s := mgtvSession{vid: vid, cid: cid, count: segmentCount(float64(sec))}
	info := fmt.Sprintf("https://galaxy.bz.mgtv.com/getctlbarrage?version=8.1.39&abroad=0&os=10.0&platform=0&deviceid=5408f92e-8772-4f65-a8ec-1699b6764bcb&vid=%s&cid=%s", vid, cid)
	if body, err := fetchString(ctx, info, headers); err == nil {
		s.cdnBase = mgtvCDNBase(body)
	}
	return s, nil
}

func clockSeconds(s string) int64 {
	var total int64
	for _, p := range strings.Split(s, ":") {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return 0
		}
		total = total*60 + v
	}
	return total
}

func mgtvCDNBase(body string) string {
	var doc struct {
		Data *struct {
			CDNList    string `json:"cdn_list"`
			CDNVersion string `json:"cdn_version"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &doc) != nil || doc.Data == nil || doc.Data.CDNList == "" || doc.Data.CDNVersion == "" {
		return ""
	}
	for _, host := range strings.Split(doc.Data.CDNList, ",") {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" {
			continue
		}
		base := host + "/" + doc.Data.CDNVersion
		if !strings.HasPrefix(base, "http") {
			base = "https://" + base
		}
		if strings.HasPrefix(base, "https://") {
			return base
		}
	}
	return ""
}

func (s mgtvSession) segments() int { return s.count }

func (s mgtvSession) fetch(ctx context.Context, n int) ([]Item, error) {
	u := fmt.Sprintf("https://galaxy.bz.mgtv.com/cdn/opbarrage?vid=%s&cid=%s&time=%d", s.vid, s.cid, int64(n-1)*segmentMs)
	if s.cdnBase != "" {
		u = fmt.Sprintf("%s/%d.json", s.cdnBase, n-1)
	}
	b, err := fetchBytes(ctx, nil, http.MethodGet, u, nil, platformHeaders("https://www.mgtv.com"))
	if err != nil {
		return nil, err
	}
	return parseMGTV(b)
}

// ---- 优酷：取 _m_h5_tk 令牌，按分段签名调 mopen.youku.danmu.list ----

var (
	youkuPathVidRe  = regexp.MustCompile(`/id_([^./?#]+)(?:\.html)?(?:[/?#]|$)`)
	youkuQueryVidRe = regexp.MustCompile(`[?&]vid=([^&#]+)`)
	youkuShowIDRe   = regexp.MustCompile(`[?&]s=([^&]+)`)
)

const (
	youkuGUID     = "NJnMGnrls3wCAXQaiNsMGIsY"
	youkuSecret   = "MkmC9SoIw6xCkSKHhJ7b5D2r51kBiREr"
	youkuAppKey   = "24679788"
	youkuTokenURL = "https://acs.youku.com/h5/mtop.youku.favorite.query.isfavorite/1.0/?appKey=" + youkuAppKey
	youkuInfoURL  = "https://openapi.youku.com/v2/videos/show.json?client_id=53e6cc67237fc59a&video_id="
)

type youkuSession struct {
	vid, token string
	count      int
	client     *http.Client
}

func youkuVidIn(s string) string {
	for _, re := range []*regexp.Regexp{youkuPathVidRe, youkuQueryVidRe} {
		if v := firstGroup(re, s); v != "" {
			if dec, err := url.QueryUnescape(v); err == nil {
				return dec
			}
			return v
		}
	}
	return ""
}

func prepareYouku(ctx context.Context, page string, durationMs int64) (fetchSession, error) {
	jar, _ := cookiejar.New(nil)
	base := util.GetClient()
	client := &http.Client{Transport: base.Transport, Timeout: base.Timeout, Jar: jar}
	ua := map[string]string{"User-Agent": browserUA}
	vid := youkuVidIn(page)
	if youkuShowIDRe.MatchString(page) {
		html, err := fetchBytes(ctx, client, http.MethodGet, page, nil, ua)
		if err != nil {
			return nil, err
		}
		vid = youkuVidIn(string(html))
	}
	if vid == "" {
		return nil, fmt.Errorf("优酷地址里找不到 vid: %s", page)
	}
	sec := float64(durationMs) / 1000
	if sec <= 0 {
		body, err := fetchBytes(ctx, client, http.MethodGet, youkuInfoURL+vid, nil, ua)
		if err != nil {
			return nil, err
		}
		var info struct {
			Duration string `json:"duration"`
		}
		_ = json.Unmarshal(body, &info)
		if sec, _ = strconv.ParseFloat(info.Duration, 64); sec <= 0 {
			return nil, fmt.Errorf("优酷取不到时长: %s", vid)
		}
	}
	if _, err := fetchBytes(ctx, client, http.MethodGet, youkuTokenURL, nil, ua); err != nil {
		return nil, err
	}
	token := ""
	tokenURL, _ := url.Parse(youkuTokenURL)
	for _, c := range jar.Cookies(tokenURL) {
		if c.Name == "_m_h5_tk" {
			token, _, _ = strings.Cut(c.Value, "_")
		}
	}
	if token == "" {
		return nil, errors.New("优酷没有返回 _m_h5_tk")
	}
	return youkuSession{vid: vid, token: token, count: segmentCount(sec), client: client}, nil
}

func (s youkuSession) segments() int { return s.count }

func (s youkuSession) fetch(ctx context.Context, n int) ([]Item, error) {
	mat := n - 1
	ts := time.Now().UnixMilli()
	msg := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"ctime": %d, "ctype": 10004, "cver": "v1.0", "guid": "%s", "mat": %d, "mcount": 1, "pid": 0, "sver": "3.1.0", "vid": "%s"}`,
		ts, youkuGUID, mat, s.vid)))
	data := fmt.Sprintf(
		`{"pid": 0, "ctype": 10004, "sver": "3.1.0", "cver": "v1.0", "ctime": %d, "guid": "%s", "vid": "%s", "mat": %d, "mcount": 1, "type": 1, "msg": "%s", "sign": "%s"}`,
		ts, youkuGUID, s.vid, mat, msg, md5Hex(msg+youkuSecret))
	sign := md5Hex(fmt.Sprintf("%s&%d&%s&%s", s.token, ts, youkuAppKey, data))
	u := fmt.Sprintf("https://acs.youku.com/h5/mopen.youku.danmu.list/1.0/?jsv=2.7.0&appKey=%s&t=%d&api=mopen.youku.danmu.list&v=1.0&type=originaljson&dataType=jsonp&timeout=20000&jsonpIncPrefix=utility&sign=%s", youkuAppKey, ts, sign)
	body, err := fetchBytes(ctx, s.client, http.MethodPost, u, strings.NewReader("data="+url.QueryEscape(data)), map[string]string{
		"User-Agent":   browserUA,
		"Referer":      "https://v.youku.com",
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Result string `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data.Result == "" {
		return nil, errors.New("优酷弹幕接口返回格式不对")
	}
	var code struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal([]byte(resp.Data.Result), &code) == nil && (code.Code == nil || *code.Code != 1) {
		return nil, nil
	}
	return parseYouku([]byte(resp.Data.Result))
}
