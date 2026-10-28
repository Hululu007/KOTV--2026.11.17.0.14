package danmaku

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func mustParse(t *testing.T, body string) []Item {
	t.Helper()
	got, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func TestParseBili(t *testing.T) {
	got := mustParse(t, "\xEF\xBB\xBF<?xml version=\"1.0\"?><i>\n"+
		`<d p="3,5,36,16711680,0,2">顶部 &amp; &lt;红&gt; &#x4F60;</d>`+
		`<d p="1.5,2,18,255">滚动</d>`+
		`<d p="2">缺字段丢弃</d>`+
		`<d p="4,8,25,0">高级弹幕丢弃</d>`+
		`<d p="5,7,25,65280">[0.5,0.25,"1-1",4.5,"定位"]</d>`+
		`<d p="6,1,25,0"></d></i>`)
	want := []Item{
		{Time: 1.5, Mode: ModeScroll, Size: 18, Color: 255, Content: "滚动"},
		{Time: 3, Mode: ModeTop, Size: 36, Color: 16711680, Content: "顶部 & <红> 你", Pool: PoolSpecial},
		{Time: 5, Mode: ModePositioned, Size: 25, Color: 65280, Content: "定位", Pool: PoolSpecial, X: 0.5, Y: 0.25, DurationMs: 4500},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestParseTxtAndLive(t *testing.T) {
	got := mustParse(t, "[0.5,4,18,65280]底部\n无前缀\n[1,1,25]缺颜色\n")
	if len(got) != 1 || got[0] != (Item{Time: 0.5, Mode: ModeBottom, Size: 18, Color: 65280, Content: "底部"}) {
		t.Errorf("txt = %+v", got)
	}
	if it := ParseLive("[0.0,5,25,16777215]你好"); it.Content != "你好" || it.Mode != ModeTop {
		t.Errorf("live prefixed = %+v", it)
	}
	if it := ParseLive("  纯文本 "); it.Content != "纯文本" || it.Mode != ModeScroll || it.Color != defaultColor {
		t.Errorf("live plain = %+v", it)
	}
}

func TestParsePlatformJSON(t *testing.T) {
	qq := mustParse(t, `{"barrage_list":[{"time_offset":"61500","content":"腾讯","content_style":"{\"color\":\"ff0000\"}"},`+
		`{"time_offset":"1000","content":"渐变","content_style":"{\"gradient_colors\":[\"00ff00\",\"0000ff\"]}"}]}`)
	if len(qq) != 2 || qq[0].Content != "渐变" || qq[0].Color != 0x00ff00 || qq[1].Time != 61.5 || qq[1].Color != 0xff0000 {
		t.Errorf("qq = %+v", qq)
	}
	youku := mustParse(t, `{"data":{"result":[{"playat":2000,"content":"优酷","propertis":"{\"color\":16711935}"}]}}`)
	if len(youku) != 1 || youku[0].Time != 2 || youku[0].Color != 16711935 {
		t.Errorf("youku = %+v", youku)
	}
	mgtv := mustParse(t, `{"status":0,"data":{"items":[{"time":3000,"content":"芒果","v2_color":{"color_left":{"r":255,"g":0,"b":0},"color_right":{"r":0,"g":0,"b":255}}}]}}`)
	if len(mgtv) != 1 || mgtv[0].Time != 3 || mgtv[0].Color != 0x800080 {
		t.Errorf("mgtv = %+v", mgtv)
	}
}

func TestParseIQIYIProtobuf(t *testing.T) {
	field := func(n int, v []byte) []byte {
		return append([]byte{byte(n<<3 | wireLen), byte(len(v))}, v...)
	}
	bullet := append(append(field(2, []byte("爱奇艺")), field(6, []byte("12.5"))...), field(8, []byte("00FF00"))...)
	entry := field(2, bullet)
	data := append(field(6, entry), []byte{1 << 3, 5}...) // 末尾一个 varint 字段应被跳过
	got := mustParse(t, string(data))
	if len(got) != 1 || got[0] != (Item{Time: 12.5, Mode: ModeScroll, Size: defaultSize, Color: 0x00FF00, Content: "爱奇艺"}) {
		t.Errorf("iqiyi = %+v", got)
	}
}

func TestParseUnsupported(t *testing.T) {
	if _, err := Parse([]byte("<html><body>视频页</body></html>")); err == nil {
		t.Error("html page should be unsupported")
	}
}

func TestFetcherFor(t *testing.T) {
	cases := map[string]bool{
		"https://www.bilibili.com/video/BV1xx":            true,
		"https://v.qq.com/x/cover/abc/def.html":           true,
		"https://www.iqiyi.com/v_19rr.html":               true,
		"https://www.mgtv.com/b/1/2.html":                 true,
		"https://v.youku.com/v_show/id_XNz.html":          true,
		"https://comment.bilibili.com/1.xml":              false,
		"https://example.com/a.xml":                       false,
		"/tmp/a.xml":                                      false,
		"https://evilv.qq.com.example.com/x/cover/a.html": false,
	}
	for u, want := range cases {
		if got := fetcherFor(u) != nil; got != want {
			t.Errorf("fetcherFor(%s) = %v, want %v", u, got, want)
		}
	}
}

func TestPlatformIDs(t *testing.T) {
	if v := firstGroup(qqVidPathRe, "https://v.qq.com/x/cover/mzc00/v0047abc.html"); v != "v0047abc" {
		t.Errorf("qq vid = %q", v)
	}
	if v := youkuVidIn("https://v.youku.com/v_show/id_XNjQ%3D%3D.html?s=1"); v != "XNjQ==" {
		t.Errorf("youku vid = %q", v)
	}
	if s := clockSeconds("1:02:03"); s != 3723 {
		t.Errorf("clock = %d", s)
	}
	if b := mgtvCDNBase(`{"data":{"cdn_list":"bullet-ali.hitv.com/,bullet-ws.hitv.com","cdn_version":"v1/x"}}`); b != "https://bullet-ali.hitv.com/v1/x" {
		t.Errorf("mgtv cdn = %q", b)
	}
}

func TestParseSources(t *testing.T) {
	cases := map[string][]model.DanmakuItem{
		`[{"name":"A","url":"http://a/1.xml"},{"title":"B","url":"http://b"},{"name":"空"}]`: {
			{Name: "A", URL: "http://a/1.xml"}, {Name: "B", URL: "http://b"},
		},
		`["http://x/1.xml"]`:            {{Name: "源 1", URL: "http://x/1.xml"}},
		`{"name":"一","url":"http://y"}`: {{Name: "一", URL: "http://y"}},
		`<i></i>`:                       nil,
	}
	for body, want := range cases {
		if got := ParseSources(body); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("ParseSources(%s) = %+v, want %+v", body, got, want)
		}
	}
}

func TestResolveLoopback(t *testing.T) {
	api, _ := url.Parse("http://192.168.0.103:9978/proxy?do=danmaku&name=x")
	got := resolveLoopback([]model.DanmakuItem{
		{URL: "http://127.0.0.1:9978/proxy?do=danmu&site=js&url=vodurl://牧神记@106@107@bilibili"},
		{URL: "http://localhost/a.xml"},
		{URL: "https://comment.bilibili.com/1.xml"},
	}, api)
	want := []string{
		"http://192.168.0.103:9978/proxy?do=danmu&site=js&url=vodurl://牧神记@106@107@bilibili",
		"http://192.168.0.103:9978/a.xml",
		"https://comment.bilibili.com/1.xml",
	}
	for i := range want {
		if got[i].URL != want[i] {
			t.Errorf("item %d = %s, want %s", i, got[i].URL, want[i])
		}
	}
	local, _ := url.Parse("http://127.0.0.1:9978/proxy?do=danmaku")
	if out := resolveLoopback([]model.DanmakuItem{{URL: "http://127.0.0.1:9978/x"}}, local); out[0].URL != "http://127.0.0.1:9978/x" {
		t.Errorf("api on this engine must keep loopback: %s", out[0].URL)
	}
}

func TestMerge(t *testing.T) {
	spider := []model.DanmakuItem{{Name: "片源", URL: "http://s"}}
	found := model.DanmakuItem{Name: "搜到", URL: "http://f"}
	if got, sel := Merge(spider, found, true); len(got) != 2 || got[1].URL != "http://f" || sel != 0 {
		t.Errorf("add: %+v sel=%d", got, sel)
	}
	if got, sel := Merge(spider, found, false); len(got) != 2 || got[0].URL != "http://f" || sel != 0 {
		t.Errorf("set: %+v sel=%d", got, sel)
	}
	if got, sel := Merge(spider, spider[0], false); len(got) != 1 || sel != 0 {
		t.Errorf("set existing: %+v sel=%d", got, sel)
	}
}

func TestSearchGetTemplate(t *testing.T) {
	var gotName, gotEp, rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		gotName, gotEp = r.URL.Query().Get("n"), r.URL.Query().Get("e")
		_, _ = w.Write([]byte(`[{"name":"命中","url":"https://v.qq.com/x/cover/a/b.html"}]`))
	}))
	defer srv.Close()
	got, err := Search(context.Background(), "c:test", srv.URL+"/s?n={name}&e={episode}", "東 京", "第1集")
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "东 京" || gotEp != "第1集" {
		t.Errorf("query name=%q ep=%q", gotName, gotEp)
	}
	if want := "n=%E4%B8%9C%20%E4%BA%AC"; rawQuery[:len(want)] != want {
		t.Errorf("raw query %q, want prefix %q", rawQuery, want)
	}
	if len(got) != 1 || got[0].URL != "https://v.qq.com/x/cover/a/b.html" {
		t.Errorf("sources = %+v", got)
	}
}

func TestSearchPostForm(t *testing.T) {
	var method, name, ep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_ = r.ParseForm()
		name, ep = r.PostForm.Get("name"), r.PostForm.Get("episode")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	if _, err := Search(context.Background(), "c:test", srv.URL, "片名", "01"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || name != "片名" || ep != "01" {
		t.Errorf("method=%s name=%q ep=%q", method, name, ep)
	}
	if _, err := Search(context.Background(), "c:test", "  ", "a", "b"); err == nil {
		t.Error("empty api should fail")
	}
}

func TestLoadHTTPAndLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<i><d p="1,1,25,255">来了</d></i>`))
	}))
	defer srv.Close()
	got, err := Load(context.Background(), srv.URL+"/dm.xml", 0)
	if err != nil || len(got) != 1 || got[0].Content != "来了" {
		t.Errorf("http: %+v err=%v", got, err)
	}

	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txt, []byte("[1,1,25,0]本地\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(context.Background(), txt, 0); err != nil || len(got) != 1 {
		t.Errorf("local txt: %+v err=%v", got, err)
	}
	other := filepath.Join(dir, "secret.conf")
	if err := os.WriteFile(other, []byte("[1,1,25,0]x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), other, 0); err == nil {
		t.Error("non-danmaku file should be rejected")
	}
	if _, err := Load(context.Background(), "a.xml", 0); err == nil {
		t.Error("relative path should be rejected")
	}
}
