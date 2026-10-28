package danmaku

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestParseXML(t *testing.T) {
	body := `<?xml version="1.0"?><i>
<d p="1.5,5,30,16711680,0">顶部 &amp; &lt;红&gt; &#x4F60;</d>
<d id="x" p="2">只有时间</d>
<d p="3,1,25,0"></d>
</i>`
	got := Parse(body)
	if len(got) != 2 {
		t.Fatalf("got %d items: %+v", len(got), got)
	}
	want0 := Item{Time: 1.5, Mode: 5, Size: 30, Color: 16711680, Content: "顶部 & <红> 你"}
	if got[0] != want0 {
		t.Errorf("item0 = %+v, want %+v", got[0], want0)
	}
	want1 := Item{Time: 2, Mode: defaultMode, Size: defaultSize, Color: defaultColor, Content: "只有时间"}
	if got[1] != want1 {
		t.Errorf("item1 = %+v, want %+v", got[1], want1)
	}
}

func TestParseText(t *testing.T) {
	got := Parse("[0.5,4,18,65280]底部\n无前缀\n[x,y]坏的元数据")
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0] != (Item{Time: 0.5, Mode: 4, Size: 18, Color: 65280, Content: "底部"}) {
		t.Errorf("item0 = %+v", got[0])
	}
	if got[1] != (Item{Mode: defaultMode, Size: defaultSize, Color: defaultColor, Content: "坏的元数据"}) {
		t.Errorf("item1 = %+v", got[1])
	}
}

func TestParseLive(t *testing.T) {
	if got := ParseLive("[0.0,5,25,16777215]你好"); got.Content != "你好" || got.Mode != 5 {
		t.Errorf("prefixed = %+v", got)
	}
	if got := ParseLive("  纯文本 "); got.Content != "纯文本" || got.Mode != defaultMode || got.Color != defaultColor {
		t.Errorf("plain = %+v", got)
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
		`[0.1,1,25,0]文本弹幕`:              nil,
	}
	for body, want := range cases {
		got := ParseSources(body)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("ParseSources(%s) = %+v, want %+v", body, got, want)
		}
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
	if got, sel := Merge(nil, found, false); len(got) != 1 || got[0].URL != "http://f" || sel != 0 {
		t.Errorf("no spider: %+v sel=%d", got, sel)
	}
}

func TestSearchGetTemplate(t *testing.T) {
	var gotName, gotEp, rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		gotName, gotEp = r.URL.Query().Get("n"), r.URL.Query().Get("e")
		_, _ = w.Write([]byte(`[{"name":"命中","url":"http://d/1.xml"}]`))
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
		t.Errorf("raw query %q, want prefix %q（空格编成 %%20）", rawQuery, want)
	}
	if len(got) != 1 || got[0].URL != "http://d/1.xml" {
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

func TestLoadFollowsSourceList(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"name":"x","url":"%s/dm.xml"}]`, srv.URL)
	})
	mux.HandleFunc("/dm.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<i><d p="1,1,25,255">来了</d></i>`))
	})
	got, err := Load(context.Background(), srv.URL+"/list")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "来了" || got[0].Color != 255 {
		t.Errorf("got %+v", got)
	}
}

func TestLoadLocalOnlyDanmakuFiles(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "a.xml")
	if err := os.WriteFile(xmlPath, []byte(`<i><d p="1">本地</d></i>`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), xmlPath)
	if err != nil || len(got) != 1 || got[0].Content != "本地" {
		t.Errorf("xml: %+v err=%v", got, err)
	}
	other := filepath.Join(dir, "secret.conf")
	if err := os.WriteFile(other, []byte("[1]x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), other); err == nil {
		t.Error("non-danmaku file should be rejected")
	}
	if _, err := Load(context.Background(), "a.xml"); err == nil {
		t.Error("relative path should be rejected")
	}
}
