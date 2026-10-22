package live

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobo/KOTV/internal/model"
)

func TestParseTXT_GroupPassSplitsAtFirstUnderscore(t *testing.T) {
	live := &model.Live{}
	Parse(live, "央视_a_b,#genre#\nCCTV1,http://x/1.m3u8\n")
	if len(live.Groups) != 1 {
		t.Fatalf("groups=%d", len(live.Groups))
	}
	g := live.Groups[0]
	if g.Name != "央视" || g.Pass != "a_b" {
		t.Fatalf("name=%q pass=%q", g.Name, g.Pass)
	}
}

func TestParseTXT_LivePassDisablesGroupPassword(t *testing.T) {
	live := &model.Live{Pass: true}
	Parse(live, "央视_123,#genre#\nCCTV1,http://x/1.m3u8\n")
	g := live.Groups[0]
	if g.Name != "央视" || g.Pass != "" {
		t.Fatalf("name=%q pass=%q", g.Name, g.Pass)
	}
}

func TestParse_AutoNumberIsZeroPadded(t *testing.T) {
	live := &model.Live{}
	Parse(live, "组,#genre#\nA,http://x/a\nB,http://x/b\n")
	chs := live.Groups[0].Channels
	if chs[0].Number != "001" || chs[1].Number != "002" {
		t.Fatalf("numbers=%q %q", chs[0].Number, chs[1].Number)
	}
}

func TestParseM3U_ClickAndCookie(t *testing.T) {
	live := &model.Live{}
	text := "#EXTM3U tvg-url=http://epg/x.xml\n" +
		"#EXTINF:-1 group-title=\"G\",C1\n" +
		"click=document.querySelector('a').click()\n" +
		"#EXTVLCOPT:http-cookie=\"sid=1\"\n" +
		"http://x/c1.m3u8\n"
	Parse(live, text)
	if live.EPG != "http://epg/x.xml" {
		t.Fatalf("unquoted tvg-url not read: %q", live.EPG)
	}
	ch := live.Groups[0].Channels[0]
	if ch.Click != "document.querySelector('a').click()" {
		t.Fatalf("click=%q", ch.Click)
	}
	if ch.Header["Cookie"] != "sid=1" {
		t.Fatalf("cookie header=%#v", ch.Header)
	}
}

func TestParseM3U_HeaderEpgOrderTvgUrlFirst(t *testing.T) {
	live := &model.Live{}
	Parse(live, "#EXTM3U url-tvg=\"http://b\" tvg-url=\"http://a\"\n#EXTINF:-1,C\nhttp://x\n")
	if live.EPG != "http://a" {
		t.Fatalf("epg=%q", live.EPG)
	}
}

func TestParseJSON_FullChannelFields(t *testing.T) {
	live := &model.Live{}
	text := `[{"name":"G","pass":"p","channel":[{"name":"C","logo":"l.png","number":"7","epg":"http://e",
	  "ua":"UA","click":"js","format":"mpd","origin":"http://o","referer":"http://r","tvgId":"id","tvgName":"tn",
	  "parse":1,"header":{"X":"1"},"catchup":{"source":"?s"},"drm":{"key":"http://k","type":"clearkey"},"urls":["http://u"]}]}]`
	Parse(live, text)
	if len(live.Groups) != 1 || live.Groups[0].Pass != "p" {
		t.Fatalf("group=%+v", live.Groups)
	}
	c := live.Groups[0].Channels[0]
	if c.Number != "7" || c.EPG != "http://e" || c.UA != "UA" || c.Click != "js" ||
		c.Format != "application/dash+xml" || c.Origin != "http://o" || c.Referer != "http://r" ||
		c.TvgID != "id" || c.TvgName != "tn" || c.Parse != 1 || c.Header["X"] != "1" ||
		c.Catchup == nil || c.Catchup.Source != "?s" || c.Drm == nil || c.Drm.Key != "http://k" ||
		len(c.URLs) != 1 {
		t.Fatalf("channel=%+v", c)
	}
}

func TestParseTXT_ParseZeroSticky(t *testing.T) {
	live := &model.Live{}
	Parse(live, "G,#genre#\nparse=1\nA,http://a\nparse=0\nB,http://b\n")
	chs := live.Groups[0].Channels
	if chs[0].Parse != 1 {
		t.Fatalf("A.Parse=%d want 1", chs[0].Parse)
	}
	if chs[1].Parse != 0 {
		t.Fatalf("B.Parse=%d want 0", chs[1].Parse)
	}
}

func TestParseTXT_HeaderJSONNumber(t *testing.T) {
	live := &model.Live{}
	Parse(live, "G,#genre#\nheader={\"Referer\":1,\"User-Agent\":\"VLC\"}\nC,http://a/1\n")
	h := live.Groups[0].Channels[0].Header
	if h["Referer"] != "1" || h["User-Agent"] != "VLC" {
		t.Fatalf("hdr=%v", h)
	}
}

func TestParseTXT_UAStripQuotes(t *testing.T) {
	live := &model.Live{}
	Parse(live, "G,#genre#\nua=\"VLC/3.0\"\nreferer=\"http://r/\"\nC,http://c\n")
	ch := live.Groups[0].Channels[0]
	if ch.UA != "VLC/3.0" {
		t.Fatalf("UA=%q", ch.UA)
	}
	if ch.Referer != "http://r/" {
		t.Fatalf("Referer=%q", ch.Referer)
	}
}

func TestParseTXT_PipeHeadersOnURL(t *testing.T) {
	live := &model.Live{}
	Parse(live, "G,#genre#\nC,http://cdn/a.m3u8|User-Agent=\"VLC\"|Referer=\"http://r/\"\n")
	ch := live.Groups[0].Channels[0]
	if ch.Header["User-Agent"] != "VLC" {
		t.Fatalf("UA header=%q hdr=%v", ch.Header["User-Agent"], ch.Header)
	}
	if ch.Header["Referer"] != "http://r/" {
		t.Fatalf("Referer header=%q", ch.Header["Referer"])
	}
}

func TestParseTXT_PipeHeadersPreferPipeOverAmp(t *testing.T) {
	live := &model.Live{}
	Parse(live, "G,#genre#\nC,http://cdn/a.m3u8|User-Agent=VLC|Referer=http://x?a=1&b=2\n")
	ch := live.Groups[0].Channels[0]
	if ch.Header["User-Agent"] != "VLC" {
		t.Fatalf("UA header=%q hdr=%v", ch.Header["User-Agent"], ch.Header)
	}
	// 值内 & 仍再切：Referer 只保留到第一个 &。
	if ch.Header["Referer"] != "http://x?a=1" {
		t.Fatalf("Referer header=%q hdr=%v", ch.Header["Referer"], ch.Header)
	}
	if ch.Header["b"] != "2" {
		t.Fatalf("b header=%q hdr=%v", ch.Header["b"], ch.Header)
	}
}

func TestParseJSON_ParseZero(t *testing.T) {
	live := &model.Live{}
	text := `[{"name":"G","channel":[{"name":"C","parse":0,"urls":["http://u"]}]}]`
	Parse(live, text)
	if live.Groups[0].Channels[0].Parse != 0 {
		t.Fatalf("Parse=%d", live.Groups[0].Channels[0].Parse)
	}
}

func TestParseJSON_GroupNameKeepsUnderscoreAndDoesNotMerge(t *testing.T) {
	live := &model.Live{}
	text := `[{"name":"CCTV_HD","channel":[{"name":"A","urls":["http://a"]}]},{"name":"CCTV_HD","pass":"p","channel":[{"name":"B","urls":["http://b"]}]}]`
	Parse(live, text)
	if len(live.Groups) != 2 {
		t.Fatalf("groups=%d", len(live.Groups))
	}
	if live.Groups[0].Name != "CCTV_HD" || live.Groups[0].Pass != "" {
		t.Fatalf("g0=%+v", live.Groups[0])
	}
	if live.Groups[1].Name != "CCTV_HD" || live.Groups[1].Pass != "p" {
		t.Fatalf("g1=%+v", live.Groups[1])
	}
	if live.Groups[0].Channels[0].Name != "A" || live.Groups[1].Channels[0].Name != "B" {
		t.Fatalf("channels=%s %s", live.Groups[0].Channels[0].Name, live.Groups[1].Channels[0].Name)
	}
}

func TestParseJSON_StringURLDoesNotDropSiblings(t *testing.T) {
	live := &model.Live{}
	text := `[{"name":"G","channel":[{"name":"A","urls":["http://a"]},{"name":"B","url":"http://b"}]}]`
	Parse(live, text)
	if len(live.Groups) != 1 || len(live.Groups[0].Channels) != 2 {
		t.Fatalf("groups=%+v", live.Groups)
	}
	a := live.Groups[0].Channels[0]
	b := live.Groups[0].Channels[1]
	if len(a.URLs) != 1 || a.URLs[0] != "http://a" || len(b.URLs) != 1 || b.URLs[0] != "http://b" {
		t.Fatalf("A=%v B=%v", a.URLs, b.URLs)
	}
}

func TestParseTXT_RepeatedGenreStartsNewGroup(t *testing.T) {
	live := &model.Live{}
	Parse(live, "央视,#genre#\nA,http://a\n央视,#genre#\nB,http://b\n")
	if len(live.Groups) != 2 {
		t.Fatalf("groups=%d", len(live.Groups))
	}
	if live.Groups[0].Name != "央视" || live.Groups[1].Name != "央视" {
		t.Fatalf("names=%q %q", live.Groups[0].Name, live.Groups[1].Name)
	}
	if live.Groups[0].Channels[0].Number != "001" || live.Groups[1].Channels[0].Number != "002" {
		t.Fatalf("numbers=%q %q", live.Groups[0].Channels[0].Number, live.Groups[1].Channels[0].Number)
	}
}

func TestParse_EmptyGroupNameIsZhibo(t *testing.T) {
	txt := &model.Live{}
	Parse(txt, "CCTV1,http://x/1.m3u8\n")
	if len(txt.Groups) != 1 || txt.Groups[0].Name != "直播" {
		t.Fatalf("txt group=%+v", txt.Groups)
	}
	m3u := &model.Live{}
	Parse(m3u, "#EXTM3U\n#EXTINF:-1,C\nhttp://x\n")
	if len(m3u.Groups) != 1 || m3u.Groups[0].Name != "直播" {
		t.Fatalf("m3u group=%+v", m3u.Groups)
	}
	blank := &model.Live{}
	Parse(blank, "#EXTM3U\n#EXTINF:-1 group-title=\"\",C\nhttp://x\n")
	if len(blank.Groups) != 1 || blank.Groups[0].Name != "直播" {
		t.Fatalf("blank title=%+v", blank.Groups)
	}
}

func TestParseM3U_PasswordOnlyGroupName(t *testing.T) {
	live := &model.Live{}
	Parse(live, "#EXTM3U\n#EXTINF:-1 group-title=\"_abc\",C\nhttp://x\n")
	g := live.Groups[0]
	if g.Name != "直播" || g.Pass != "abc" {
		t.Fatalf("name=%q pass=%q", g.Name, g.Pass)
	}
}

func TestParseM3U_KeepsSourceCatchupWhenHeaderHasNone(t *testing.T) {
	live := &model.Live{Catchup: model.Catchup{Type: "append", Source: "?s=1"}}
	Parse(live, "#EXTM3U\n#EXTINF:-1,C\nhttp://x\n")
	if live.Catchup.Source != "?s=1" {
		t.Fatalf("live catchup=%q", live.Catchup.Source)
	}
	ch := live.Groups[0].Channels[0]
	if ch.Catchup == nil || ch.Catchup.Source != "?s=1" {
		t.Fatalf("channel catchup=%v", ch.Catchup)
	}
}

func TestParseM3U_HeaderCatchupReplacesSource(t *testing.T) {
	live := &model.Live{Catchup: model.Catchup{Source: "?old"}}
	Parse(live, "#EXTM3U catchup-source=\"?new\"\n#EXTINF:-1,C\nhttp://x\n")
	if live.Catchup.Source != "?new" {
		t.Fatalf("live catchup=%q", live.Catchup.Source)
	}
	ch := live.Groups[0].Channels[0]
	if ch.Catchup == nil || ch.Catchup.Source != "?new" {
		t.Fatalf("channel catchup=%v", ch.Catchup)
	}
}

func TestLivePlayTimeout(t *testing.T) {
	if got := (&model.Live{}).PlayTimeout(); got != 15*time.Second {
		t.Fatalf("default=%s", got)
	}
	var live model.Live
	if err := json.Unmarshal([]byte(`{"timeout":0}`), &live); err != nil {
		t.Fatal(err)
	}
	if got := live.PlayTimeout(); got != time.Second {
		t.Fatalf("floor=%s", got)
	}
	if err := json.Unmarshal([]byte(`{"timeout":8}`), &live); err != nil {
		t.Fatal(err)
	}
	if got := live.PlayTimeout(); got != 8*time.Second {
		t.Fatalf("set=%s", got)
	}
}
