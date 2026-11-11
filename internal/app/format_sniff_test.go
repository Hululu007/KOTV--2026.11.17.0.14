package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestResolvePlayFormatTrustAndStrong(t *testing.T) {
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })
	sniffPlayFormatFn = func(string, map[string]string) string {
		t.Fatal("sniff should not run")
		return ""
	}

	got, hls := resolvePlayFormat("https://cdn/a.php", "application/x-mpegURL", nil)
	if got != model.FormatHLS || !hls {
		t.Fatalf("trust format got=%q hls=%v", got, hls)
	}
	got, hls = resolvePlayFormat("https://cdn/a.m3u8", "", nil)
	if got != model.FormatHLS || !hls {
		t.Fatalf("strong url got=%q hls=%v", got, hls)
	}
	got, hls = resolvePlayFormat("https://cdn/a.mpd", "mpd", nil)
	if got != model.FormatDASH || hls {
		t.Fatalf("dash got=%q hls=%v", got, hls)
	}
}

func TestResolvePlayFormatSniffPHP(t *testing.T) {
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nseg.ts\n"))
	}))
	t.Cleanup(srv.Close)

	sniffPlayFormatFn = sniffPlayFormatHTTP
	got, hls := resolvePlayFormat(srv.URL+"/play.php", "", nil)
	if got != model.FormatHLS || !hls {
		t.Fatalf("php sniff got=%q hls=%v", got, hls)
	}
}

func TestResolvePlayFormatSniffFLVQuery(t *testing.T) {
	prev := sniffPlayFormatFn
	t.Cleanup(func() { sniffPlayFormatFn = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("FLV\x01\x01\x00\x00\x00"))
	}))
	t.Cleanup(srv.Close)

	sniffPlayFormatFn = sniffPlayFormatHTTP
	raw := srv.URL + "/fengshows?id=2.m3u8"
	got, hls := resolvePlayFormat(raw, "", nil)
	if got != "" || hls {
		t.Fatalf("flv query got=%q hls=%v", got, hls)
	}
}

func TestPreparePlaybackHLSForcesProxy(t *testing.T) {
	withAdFilter(t, false)
	a := &App{}
	raw := "https://cdn.example/disguise.php"
	play, _, _ := a.PreparePlaybackHLS(raw, nil, true)
	if !strings.Contains(play, "/proxy/play?id=") {
		t.Fatalf("expected proxy for sniffed hls, play=%q", play)
	}
	plain, _, _ := a.PreparePlaybackHLS(raw, nil, false)
	if strings.Contains(plain, "/proxy/play?") {
		t.Fatalf("plain php should not proxy, play=%q", plain)
	}
}

func TestProxyHlsFormatKeepsSniffed(t *testing.T) {
	withAdFilter(t, false)
	a := &App{}
	raw := "https://cdn.example/disguise.php"
	play, _, _ := a.PreparePlaybackHLS(raw, nil, true)
	if got := proxyHlsFormat(raw, play, model.FormatHLS); got != model.FormatHLS {
		t.Fatalf("sniffed format=%q", got)
	}
}
