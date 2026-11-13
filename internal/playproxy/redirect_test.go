package playproxy_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/playproxy"
	"github.com/bobo/KOTV/internal/settings"
)

func withAdFilter(t *testing.T, on bool) {
	t.Helper()
	prev := settings.Get(settings.AdFilter)
	v := "false"
	if on {
		v = "true"
	}
	settings.Set(settings.AdFilter, v)
	t.Cleanup(func() { settings.Set(settings.AdFilter, prev) })
}

func TestHandleKeepsCookieAcrossRedirect(t *testing.T) {
	t.Parallel()
	var sawCookie bool
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := r.Header.Get("Cookie"); strings.Contains(c, "qk=1") {
			sawCookie = true
		}
		w.Header().Set("Content-Type", "video/x-matroska")
		_, _ = w.Write([]byte("MKVDATA"))
	}))
	defer final.Close()

	bounce := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/file.mkv", http.StatusFound)
	}))
	defer bounce.Close()

	proxyURL := playproxy.Register(bounce.URL+"/start", map[string]string{
		"Cookie":     "qk=1",
		"User-Agent": "QuarkTest",
	}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", res.StatusCode, body)
	}
	if !sawCookie {
		t.Fatal("cookie was stripped on redirect")
	}
	if string(body) != "MKVDATA" {
		t.Fatalf("body=%q", body)
	}
}

// 跨 Host 302：Go 会把 Referer 设成中间网关；xhscdn 等会因此防盗链 403。
// 代理跟跳须去掉 Referer，同时仍带上 Cookie。
func TestHandleDropsRefererOnCrossHostRedirect(t *testing.T) {
	t.Parallel()
	var gotRef, gotCookie string
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		gotCookie = r.Header.Get("Cookie")
		if gotRef != "" {
			http.Error(w, "forbid referer", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer final.Close()

	bounce := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/seg.png", http.StatusFound)
	}))
	defer bounce.Close()

	proxyURL := playproxy.Register(bounce.URL+"/nby/ts", map[string]string{
		"Cookie":  "qk=1",
		"Referer": bounce.URL + "/",
	}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d body=%s ref=%q", res.StatusCode, body, gotRef)
	}
	if gotRef != "" {
		t.Fatalf("cross-host redirect must drop Referer, got %q", gotRef)
	}
	if !strings.Contains(gotCookie, "qk=1") {
		t.Fatalf("cookie must still be kept, got %q", gotCookie)
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("body=%q", body)
	}
}

func TestTryHandleEmbeddedProxy(t *testing.T) {
	t.Parallel()
	defer hostclient.Bind(hostclient.Request{ClientID: "embedded-proxy", PublicBase: "http://192.168.1.8:9978"})()

	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "qk=1" {
			http.Error(w, "no cookie", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("MP4"))
	}))
	defer final.Close()

	hdr, _ := json.Marshal(map[string]string{"Cookie": "qk=1"})
	q := url.Values{}
	q.Set("do", "quark")
	q.Set("type", "video")
	q.Set("url", base64.StdEncoding.EncodeToString([]byte(final.URL+"/a.mp4")))
	q.Set("header", base64.StdEncoding.EncodeToString(hdr))
	req := httptest.NewRequest(http.MethodGet, "/proxy?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	if !playproxy.TryHandleEmbeddedProxy(rr, req) {
		t.Fatal("expected handle on remote")
	}
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.Header.Get("Accept-Ranges") != "" {
		t.Fatalf("loopback file must not invent Accept-Ranges, got %q", res.Header.Get("Accept-Ranges"))
	}
	if res.StatusCode != 200 || string(body) != "MP4" {
		t.Fatalf("status=%d body=%s", res.StatusCode, body)
	}
}

func TestTryHandleEmbeddedProxySkipsLocal(t *testing.T) {
	t.Parallel()
	hdr, _ := json.Marshal(map[string]string{"Cookie": "qk=1"})
	q := url.Values{}
	q.Set("do", "quark")
	q.Set("url", base64.StdEncoding.EncodeToString([]byte("https://cdn.example/a.mp4")))
	q.Set("header", base64.StdEncoding.EncodeToString(hdr))
	req := httptest.NewRequest(http.MethodGet, "/proxy?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	if playproxy.TryHandleEmbeddedProxy(rr, req) {
		t.Fatal("local must leave /proxy to jar")
	}
}

func TestHandleTwoRanges(t *testing.T) {
	t.Parallel()
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Range"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		switch r.Header.Get("Range") {
		case "bytes=0-3":
			w.Header().Set("Content-Range", "bytes 0-3/8")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("abcd"))
		case "bytes=4-7":
			w.Header().Set("Content-Range", "bytes 4-7/8")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("efgh"))
		default:
			http.Error(w, "range", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/movie.mp4", map[string]string{"Cookie": "qk=1"}, false)
	for _, rg := range []string{"bytes=0-3", "bytes=4-7"} {
		req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
		req.Header.Set("Range", rg)
		rr := httptest.NewRecorder()
		playproxy.Handle(rr, req)
		res := rr.Result()
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("range %s status=%d body=%s", rg, res.StatusCode, body)
		}
		if res.Header.Get("Accept-Ranges") != "bytes" {
			t.Fatalf("Accept-Ranges=%q", res.Header.Get("Accept-Ranges"))
		}
	}
	if len(got) != 2 || got[0] != "bytes=0-3" || got[1] != "bytes=4-7" {
		t.Fatalf("upstream ranges=%v", got)
	}
}

func TestHandlePassthroughKeepsStatusHeadersAndBody(t *testing.T) {
	t.Parallel()
	const payload = "\x00\x00\x00\x18ftypmp42BODY"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-" {
			t.Errorf("range=%q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("X-Play-State", "buffering")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/movie.mp4", map[string]string{"Cookie": "qk=1"}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	req.Header.Set("Range", "bytes=0-")
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status=%d", res.StatusCode)
	}
	if res.Header.Get("X-Play-State") != "buffering" {
		t.Fatalf("state=%q", res.Header.Get("X-Play-State"))
	}
	if res.Header.Get("Accept-Ranges") != "" {
		t.Fatalf("invented Accept-Ranges=%q", res.Header.Get("Accept-Ranges"))
	}
	if res.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("type=%q", res.Header.Get("Content-Type"))
	}
	if string(body) != payload {
		t.Fatalf("body=%q", body)
	}
}

func TestHandleForwardsGzipBodyWithoutRequestingIt(t *testing.T) {
	t.Parallel()
	payload := []byte{0x1f, 0x8b, 0x08, 0x00, 'b', 'o', 'd', 'y'}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enc := r.Header.Get("Accept-Encoding"); strings.Contains(strings.ToLower(enc), "gzip") {
			t.Errorf("Accept-Encoding=%q", enc)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/movie.mp4", map[string]string{"Cookie": "qk=1"}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%q", res.StatusCode, body)
	}
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding=%q", res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("Content-Length") != strconv.Itoa(len(payload)) {
		t.Fatalf("Content-Length=%q", res.Header.Get("Content-Length"))
	}
	if string(body) != string(payload) {
		t.Fatalf("body=%q", body)
	}
}

func TestFetchUsesPlayerEncodingAndIfRange(t *testing.T) {
	t.Parallel()
	var gotAE, gotIfRange, gotUA, gotConn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAE = r.Header.Get("Accept-Encoding")
		gotIfRange = r.Header.Get("If-Range")
		gotUA = r.Header.Get("User-Agent")
		gotConn = r.Header.Get("Connection")
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("BODY"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/movie.mp4", map[string]string{
		"Cookie":          "qk=1",
		"Accept-Encoding": "gzip",
		"User-Agent":      "QuarkTest",
	}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("If-Range", `"etag"`)
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "Lavf/60.0.0")
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	if gotAE != "identity" {
		t.Fatalf("Accept-Encoding=%q", gotAE)
	}
	if gotIfRange != `"etag"` {
		t.Fatalf("If-Range=%q", gotIfRange)
	}
	if gotUA != "QuarkTest" {
		t.Fatalf("User-Agent=%q", gotUA)
	}
	if !strings.EqualFold(gotConn, "close") {
		t.Fatalf("Connection=%q", gotConn)
	}
}

func TestHandleLeavesCDNPlaylistBytes(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	var gotCookie, gotPath string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/a.ts" {
			gotCookie = r.Header.Get("Cookie")
			gotPath = r.URL.Path
			_, _ = w.Write([]byte("seg"))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("X-Play-State", "open")
		_, _ = w.Write([]byte("#EXTM3U\n" + srv.URL + "/a.ts\n"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/v.m3u8", map[string]string{"Cookie": "qk=1"}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d", res.StatusCode)
	}
	text := string(body)
	if !strings.Contains(text, "/proxy/play?") || !strings.Contains(text, "u=") {
		t.Fatalf("playlist child should use the same proxy, body=%q", text)
	}
	if strings.Contains(text, srv.URL+"/a.ts\n") {
		t.Fatalf("raw segment url left in playlist: %q", text)
	}
	if res.Header.Get("X-Play-State") != "open" {
		t.Fatalf("state=%q", res.Header.Get("X-Play-State"))
	}
	if res.Header.Get("Accept-Ranges") != "" {
		t.Fatalf("invented Accept-Ranges=%q", res.Header.Get("Accept-Ranges"))
	}
	child := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "/proxy/play?") {
			child = line
			break
		}
	}
	req = httptest.NewRequest(http.MethodGet, child, nil)
	rr = httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res = rr.Result()
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("child status=%d", res.StatusCode)
	}
	if gotCookie != "qk=1" || gotPath != "/a.ts" {
		t.Fatalf("cookie=%q path=%q", gotCookie, gotPath)
	}
}

func TestHandleKeepsAbsoluteCDNSegmentsWithoutHeaders(t *testing.T) {
	// 无登记头、无去广告：只保留 CDN 绝对地址，不改写进 /proxy/play。
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nhttps://cdn.example/seg.png\n"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/v.m3u8", nil, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	body, _ := io.ReadAll(rr.Result().Body)
	text := string(body)
	if !strings.Contains(text, "https://cdn.example/seg.png") {
		t.Fatalf("cdn segment should stay absolute: %q", text)
	}
	if strings.Contains(text, "/proxy/play?") {
		t.Fatalf("should not proxy children without headers/filter, got %q", text)
	}
}

func TestHandleProxiesChildrenWhenFilterOn(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nhttps://cdn.example/seg.ts\n"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/v.m3u8", nil, true)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	body, _ := io.ReadAll(rr.Result().Body)
	text := string(body)
	if strings.Contains(text, "https://cdn.example/seg.ts\n") {
		t.Fatalf("filter on should proxy children, got %q", text)
	}
	if !strings.Contains(text, "/proxy/play?") || !strings.Contains(text, "u=") {
		t.Fatalf("expected proxied child when filter on, got %q", text)
	}
}

func TestHandlePassthroughLeavesSegmentBody(t *testing.T) {
	t.Parallel()
	const payload = "not-really-ts-but-passthrough"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/a.ts" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(payload))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n" + srv.URL + "/a.ts\n"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/v.m3u8", map[string]string{"Cookie": "qk=1"}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	pl, _ := io.ReadAll(rr.Result().Body)
	child := ""
	for _, line := range strings.Split(string(pl), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "/proxy/play?") {
			child = line
			break
		}
	}
	if child == "" {
		t.Fatalf("missing proxied child in %q", pl)
	}
	req = httptest.NewRequest(http.MethodGet, child, nil)
	rr = httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	got, _ := io.ReadAll(res.Body)
	if string(got) != payload {
		t.Fatalf("passthrough body=%q want=%q", got, payload)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Fatalf("content-type should stay upstream, got %q", ct)
	}
}

func TestHandleRewritesPlaylistWithoutM3U8InURL(t *testing.T) {
	withAdFilter(t, false)
	defer hostclient.Bind(hostclient.Request{ClientID: "playlist-rewrite", PublicBase: "http://192.168.1.8:9978"})()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nseg.ts\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n"))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/video", map[string]string{"Cookie": "qk=1"}, false)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	text := string(body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", res.StatusCode, text)
	}
	if strings.Contains(text, "\nseg.ts\n") || strings.Contains(text, `URI="key"`) {
		t.Fatalf("relative child left in playlist: %q", text)
	}
	if !strings.Contains(text, "http://192.168.1.8:9978/proxy/play?") || !strings.Contains(text, "seg.ts") || !strings.Contains(text, "key") {
		t.Fatalf("children should stay on the proxy, got %q", text)
	}
}

func TestHandleAbsolutizesRedirectedPlaylist(t *testing.T) {
	withAdFilter(t, true)
	var gotCookie string
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("X-Upstream", "1")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\n/seg.ts\n"))
	}))
	defer dst.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dst.URL+"/timeshift.m3u8", http.StatusFound)
	}))
	defer src.Close()

	proxyURL := playproxy.Register(src.URL+"/entry", map[string]string{"Cookie": "qk=1", "User-Agent": "Quark"}, true)
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	text := string(body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", res.StatusCode, text)
	}
	if gotCookie != "qk=1" {
		t.Fatalf("cookie=%q", gotCookie)
	}
	if !strings.Contains(text, "/proxy/play?") || !strings.Contains(text, url.QueryEscape(dst.URL+"/seg.ts")) {
		t.Fatalf("body=%s", text)
	}
	if res.Header.Get("X-Upstream") != "1" {
		t.Fatalf("upstream header=%q", res.Header.Get("X-Upstream"))
	}
}

func TestHandleSkipsFilterWhenUnset(t *testing.T) {
	withAdFilter(t, true)
	const raw = "#EXTM3U\n#EXTINF:1,\n/seg.ts\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(raw))
	}))
	defer srv.Close()

	proxyURL := playproxy.Register(srv.URL+"/live.m3u8", map[string]string{"Cookie": "qk=1"}, false)
	if playproxy.RegisteredFilter(proxyURL) {
		t.Fatal("live registration must not filter")
	}
	req := httptest.NewRequest(http.MethodGet, proxyURL, nil)
	rr := httptest.NewRecorder()
	playproxy.Handle(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("status=%d", res.StatusCode)
	}
	text := string(body)
	if text == raw || strings.Contains(text, "\n/seg.ts\n") {
		t.Fatalf("relative segment must not stay, body=%q", text)
	}
	if !strings.Contains(text, "/proxy/play?") || !strings.Contains(text, "seg.ts") {
		t.Fatalf("body=%q", text)
	}
}
