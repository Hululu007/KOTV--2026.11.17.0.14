package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestDecidePlayFormat(t *testing.T) {
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })
	sniffPlayFormatFn = func(string, map[string]string) string {
		t.Fatal("sniff should not run")
		return ""
	}

	got, hls := decidePlayFormat("https://cdn/a.php", "application/x-mpegURL", nil, true)
	if got != model.FormatHLS || !hls {
		t.Fatalf("trust hls got=%q hls=%v", got, hls)
	}
	got, hls = decidePlayFormat("https://cdn/a.m3u8", "", nil, true)
	if got != model.FormatHLS || !hls {
		t.Fatalf("strong url got=%q hls=%v", got, hls)
	}
	got, hls = decidePlayFormat("https://cdn/a.mpd", "mpd", nil, true)
	if got != model.FormatDASH || hls {
		t.Fatalf("dash got=%q hls=%v", got, hls)
	}
	got, hls = decidePlayFormat("https://cdn/a.php", "video/mp4", nil, true)
	if got != "video/mp4" || hls {
		t.Fatalf("trust progressive got=%q hls=%v", got, hls)
	}
	got, hls = decidePlayFormat("https://cdn/a.m3u8", "video/mp4", nil, true)
	if got != model.FormatHLS || !hls {
		t.Fatalf("url strong overrides progressive got=%q hls=%v", got, hls)
	}
	got, hls = decidePlayFormat("https://cdn/a.php", "hls", nil, false)
	if got != model.FormatHLS || !hls {
		t.Fatalf("literal hls got=%q hls=%v", got, hls)
	}
}

func TestDecidePlayFormatSniff(t *testing.T) {
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })

	hlsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nseg.ts\n"))
	}))
	t.Cleanup(hlsSrv.Close)
	sniffPlayFormatFn = sniffPlayFormatHTTP
	got, hls := decidePlayFormat(hlsSrv.URL+"/play.php", "", nil, true)
	if got != model.FormatHLS || !hls {
		t.Fatalf("php sniff got=%q hls=%v", got, hls)
	}

	flvSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("FLV\x01\x01\x00\x00\x00"))
	}))
	t.Cleanup(flvSrv.Close)
	raw := flvSrv.URL + "/fengshows?id=2.m3u8"
	got, hls = decidePlayFormat(raw, "", nil, true)
	if got != "" || hls {
		t.Fatalf("flv query got=%q hls=%v", got, hls)
	}

	got, hls = decidePlayFormat(hlsSrv.URL+"/play.php", "", nil, false)
	if got != "" || hls {
		t.Fatalf("sniff disabled got=%q hls=%v", got, hls)
	}
}

func TestPlanPlaybackVod(t *testing.T) {
	withAdFilter(t, false)
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })
	sniffPlayFormatFn = func(string, map[string]string) string { return model.FormatHLS }

	a := &App{}
	plan := a.planPlayback("https://cdn.example/disguise.php", "", nil, playModeVod)
	if plan.Format != model.FormatHLS {
		t.Fatalf("format=%q", plan.Format)
	}
	if !strings.Contains(plan.Play, "/proxy/play?id=") {
		t.Fatalf("disguised hls should proxy, play=%q", plan.Play)
	}

	sniffPlayFormatFn = func(string, map[string]string) string { return "" }
	plain := a.planPlayback("https://cdn.example/disguise.php", "", nil, playModeVod)
	if strings.Contains(plain.Play, "/proxy/play?") {
		t.Fatalf("unknown php should not proxy, play=%q", plain.Play)
	}
}

func TestPlanPlaybackLiveNoProxy(t *testing.T) {
	withAdFilter(t, true)
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })
	sniffPlayFormatFn = func(string, map[string]string) string {
		t.Fatal("declared hls must not sniff")
		return ""
	}
	a := &App{}
	plan := a.planPlayback("https://cdn.example/live.m3u8", "hls", nil, playModeLive)
	if plan.Format != model.FormatHLS {
		t.Fatalf("format=%q", plan.Format)
	}
	if strings.Contains(plan.Play, "/proxy/play?") {
		t.Fatalf("live must not proxy, play=%q", plan.Play)
	}
}

func TestPlanPlaybackLiveSniffsOpaque(t *testing.T) {
	withAdFilter(t, true)
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })
	sniffPlayFormatFn = func(string, map[string]string) string { return model.FormatHLS }
	a := &App{}
	plan := a.planPlayback("https://cdn.example/live.php", "", nil, playModeLive)
	if plan.Format != model.FormatHLS {
		t.Fatalf("format=%q", plan.Format)
	}
	if strings.Contains(plan.Play, "/proxy/play?") {
		t.Fatalf("live sniffed hls must not proxy, play=%q", plan.Play)
	}
}

func TestShouldRegisterVodHls(t *testing.T) {
	withAdFilter(t, true)
	reg, filter := shouldRegisterVodHls("https://cdn/a.m3u8", true)
	if !reg || !filter {
		t.Fatalf("ad on strong: reg=%v filter=%v", reg, filter)
	}
	reg, filter = shouldRegisterVodHls("https://cdn/fengshows?id=2.m3u8", false)
	if reg {
		t.Fatalf("non-hls must not register")
	}

	withAdFilter(t, false)
	reg, filter = shouldRegisterVodHls("https://cdn/a.m3u8", true)
	if reg {
		t.Fatalf("ad off strong should direct")
	}
	reg, filter = shouldRegisterVodHls("https://cdn/a.php", true)
	if !reg || filter {
		t.Fatalf("ad off disguise: reg=%v filter=%v", reg, filter)
	}
}
