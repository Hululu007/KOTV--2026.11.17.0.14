package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestIsLocalMediaURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		u    string
		want bool
	}{
		{"file:///storage/emulated/0/Movies/a.mp4", true},
		{"file:///storage/emulated/0/Movies/中文.mkv", true},
		{"content://media/external/video/media/12", true},
		{"/storage/emulated/0/DCIM/Camera/a.mp4", true},
		{"/storage/emulated/0/Download/show.mkv", true},
		{"https://cdn.example.com/a.mp4", false},
		{"/api/v1/vod", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsLocalMediaURL(c.u); got != c.want {
			t.Fatalf("IsLocalMediaURL(%q)=%v want %v", c.u, got, c.want)
		}
	}
}

func TestApiLooksUnplayableLocal(t *testing.T) {
	t.Parallel()
	if apiLooksUnplayable("file:///storage/emulated/0/Movies/a.mp4") {
		t.Fatal("file:// mp4 should be playable")
	}
	if apiLooksUnplayable("/storage/emulated/0/Movies/a.mkv") {
		t.Fatal("absolute local mkv should be playable")
	}
	if apiLooksUnplayable("content://media/external/video/media/1") {
		t.Fatal("content:// should be playable")
	}
	if !apiLooksUnplayable("https://example.com/play.html") {
		t.Fatal("html page should be unplayable")
	}
	if !apiLooksUnplayable("") {
		t.Fatal("empty should be unplayable")
	}
}

func TestPreparePlaybackURLSkipsLocal(t *testing.T) {
	t.Parallel()
	a := &App{}
	in := "file:///storage/emulated/0/a.mp4"
	if got := a.PreparePlaybackURL(in, map[string]string{"User-Agent": "x"}); got != in {
		t.Fatalf("local url was proxied: %s", got)
	}
}

func TestPreparePlaybackURLLocalFileStaysPath(t *testing.T) {
	hostclient.SetPublicBase("")
	f, err := os.CreateTemp("", "kotv-local-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	a := &App{}
	if got := a.PreparePlaybackURL(name, nil); got != name {
		t.Fatalf("local playback changed path: %s", got)
	}
	if got := a.PreparePlaybackURL("content://media/1", nil); got != "content://media/1" {
		t.Fatalf("content uri changed: %s", got)
	}
}

func TestPreparePlaybackURLRemoteLocalFile(t *testing.T) {
	hostclient.SetPublicBase("http://203.0.113.8:9978")
	defer hostclient.SetPublicBase("")
	f, err := os.CreateTemp("", "kotv-remote-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	a := &App{}
	got := a.PreparePlaybackURL(name, nil)
	prefix := "http://203.0.113.8:9978/file/"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("remote local file = %q", got)
	}
	if !strings.Contains(got, filepath.Base(name)) {
		t.Fatalf("remote local file missing name: %s", got)
	}
	if got := a.PreparePlaybackURL("content://media/1", nil); got != "content://media/1" {
		t.Fatalf("content uri changed: %s", got)
	}
	missing := "/tmp/kotv-missing-not-there.mp4"
	if got := a.PreparePlaybackURL(missing, nil); got != missing {
		t.Fatalf("missing file rewritten: %s", got)
	}
}

func TestPreparePlaybackURLLocalKeepsQuarkSpiderProxy(t *testing.T) {
	a := &App{}
	cdn := "https://cdn-quark.example/01.mkv"
	hdrJSON := `{"Cookie":"qk=1","User-Agent":"Quark"}`
	raw := "proxy://do=quark&type=video&url=" +
		base64.StdEncoding.EncodeToString([]byte(cdn)) +
		"&header=" + base64.StdEncoding.EncodeToString([]byte(hdrJSON))
	got := a.PreparePlaybackURL(raw, nil)
	if strings.Contains(got, "/proxy/play?id=") {
		t.Fatalf("local must keep /proxy for jar, got %q", got)
	}
	if !strings.Contains(got, "/proxy?") {
		t.Fatalf("local expected spider /proxy, got %q", got)
	}
}

func TestPreparePlaybackURLRemoteExpandsQuarkProxy(t *testing.T) {
	done := hostclient.EnterSession("test-client", "", false)
	defer done()
	hostclient.SetPublicBase("http://192.168.1.8:9978")

	a := &App{}
	cdn := "https://cdn-quark.example/01.mkv"
	hdrJSON := `{"Cookie":"qk=1","User-Agent":"Quark"}`
	raw := "proxy://do=quark&type=video&url=" +
		base64.StdEncoding.EncodeToString([]byte(cdn)) +
		"&header=" + base64.StdEncoding.EncodeToString([]byte(hdrJSON))
	got, direct, headers := a.PreparePlayback(raw, nil)
	if got != cdn {
		t.Fatalf("public media should stay the original url, got %q", got)
	}
	if strings.Contains(got, "/proxy/play?") {
		t.Fatalf("public media must not be proxied, got %q", got)
	}
	if direct != "" {
		t.Fatalf("direct=%q want empty", direct)
	}
	if headers["Cookie"] != "qk=1" || headers["User-Agent"] != "Quark" {
		t.Fatalf("headers=%v", headers)
	}
}

func TestPreparePlaybackURLRelaysOtherLoopbackPort(t *testing.T) {
	done := hostclient.EnterSession("relay-client", "", false)
	defer done()
	hostclient.SetPublicBase("http://192.168.1.8:9978")

	a := &App{}
	raw := "http://127.0.0.1:5266/fishplay/go/quark/32t/abc"
	got := a.PreparePlaybackURL(raw, nil)
	if !strings.Contains(got, "http://192.168.1.8:9978/proxy/play?id=") {
		t.Fatalf("other loopback port should be relayed, got %q", got)
	}
	upstream, _ := playproxy.Resolve(got)
	if upstream != raw {
		t.Fatalf("upstream=%q want %q", upstream, raw)
	}
}

func TestPreparePlaybackURLRemoteKeepsEnginePort(t *testing.T) {
	done := hostclient.EnterSession("engine-port", "", false)
	defer done()
	hostclient.SetPublicBase("http://192.168.1.8:9978")

	a := &App{}
	raw := "http://127.0.0.1:9978/file/a.jpg"
	got := a.PreparePlaybackURL(raw, nil)
	if strings.Contains(got, "/proxy/play?") {
		t.Fatalf("engine port must not be relayed, got %q", got)
	}
	if got != "http://192.168.1.8:9978/file/a.jpg" {
		t.Fatalf("got %q", got)
	}
}

func TestPreparePlaybackURLLocalKeepsOtherLoopbackPort(t *testing.T) {
	done := hostclient.EnterSession("local-side", "", false)
	defer done()
	hostclient.SetPublicBase("")

	a := &App{}
	raw := "http://127.0.0.1:5266/fishplay/go/quark/32t/abc"
	if got := a.PreparePlaybackURL(raw, nil); got != raw {
		t.Fatalf("local playback should keep %q, got %q", raw, got)
	}
}

func TestPreparePlaybackURLRemoteExpandsM3U8Proxy(t *testing.T) {
	withAdFilter(t, false)
	done := hostclient.EnterSession("m3u8-proxy", "", false)
	defer done()
	hostclient.SetPublicBase("http://192.168.1.8:9978")

	a := &App{}
	cdn := "http://127.0.0.1:1/a.m3u8"
	hdrJSON := `{"Cookie":"qk=1"}`
	raw := "proxy://do=quark&type=video&url=" +
		base64.StdEncoding.EncodeToString([]byte(cdn)) +
		"&header=" + base64.StdEncoding.EncodeToString([]byte(hdrJSON))
	got, _, headers := a.PreparePlayback(raw, nil)
	if !strings.Contains(got, "http://192.168.1.8:9978/proxy/play?id=") {
		t.Fatalf("remote loopback m3u8 should be relayed, got %q", got)
	}
	upstream, stored := playproxy.Resolve(got)
	if upstream != cdn {
		t.Fatalf("upstream=%q want %q", upstream, cdn)
	}
	if stored["Cookie"] != "qk=1" || headers["Cookie"] != "qk=1" {
		t.Fatalf("headers=%v stored=%v", headers, stored)
	}
}

func TestPreparePlaybackDirectKeepsPublicURL(t *testing.T) {
	a := &App{}
	raw := "https://cdn.example/a.mp4"
	play, direct, headers := a.PreparePlayback(raw, map[string]string{"Cookie": "a=b"})
	if play != raw {
		t.Fatalf("play=%q want %q", play, raw)
	}
	if direct != "" {
		t.Fatalf("direct=%q want empty", direct)
	}
	if headers["Cookie"] != "a=b" {
		t.Fatalf("headers=%v", headers)
	}
}

func TestPreparePlaybackDirectSkipsLoopback(t *testing.T) {
	a := &App{}
	raw := "http://127.0.0.1:5266/a.mp4"
	play, direct, headers := a.PreparePlayback(raw, map[string]string{"Cookie": "a=b"})
	if play != raw {
		t.Fatalf("local loopback should stay %q, got %q", raw, play)
	}
	if direct != "" {
		t.Fatalf("direct=%q", direct)
	}
	if headers["Cookie"] != "a=b" {
		t.Fatalf("headers=%v", headers)
	}
}

// 点播无请求头、不去广告时 url 就是原地址。
func TestPreparePlaybackLivePlainURL(t *testing.T) {
	withAdFilter(t, false)
	a := &App{}
	raw := "https://cdn.example/live.m3u8"
	play, direct, _ := a.PreparePlayback(raw, nil)
	if play != raw {
		t.Fatalf("play=%q want %q", play, raw)
	}
	if direct != "" {
		t.Fatalf("direct=%q want empty", direct)
	}
}

func TestPreparePlaybackAdFilterUsesRoute(t *testing.T) {
	withAdFilter(t, true)
	a := &App{}
	raw := "https://cdn.example/live.m3u8"
	play, direct, headers := a.PreparePlayback(raw, map[string]string{"Cookie": "a=b"})
	if !strings.Contains(play, "/proxy/play?id=") {
		t.Fatalf("play=%q", play)
	}
	if direct != "" {
		t.Fatalf("direct=%q want empty while ad filter is on", direct)
	}
	if !playproxy.RegisteredFilter(play) {
		t.Fatal("vod ad registration must filter")
	}
	if headers["Cookie"] != "a=b" {
		t.Fatalf("headers=%v", headers)
	}

	done := hostclient.EnterSession("ad-remote", "", false)
	defer done()
	hostclient.SetPublicBase("http://192.168.1.8:9978")
	remote, remoteDirect, _ := a.PreparePlayback(raw, map[string]string{"User-Agent": "Quark"})
	if !strings.HasPrefix(remote, "http://192.168.1.8:9978/proxy/play?id=") {
		t.Fatalf("remote=%q", remote)
	}
	if remoteDirect != "" {
		t.Fatalf("remote direct=%q", remoteDirect)
	}
}

// 列表不拉流。直播不去广告，有请求头也下发原地址，由播放器自己带上。
func TestPreparePlaybackListedSkipsM3U8Resolve(t *testing.T) {
	withAdFilter(t, false)
	a := &App{}
	raw := "https://tv.bobohome.store:20250/migu?id=608807420.m3u8"
	start := time.Now()
	play, direct, _ := a.PreparePlaybackListed(raw, nil)
	if dur := time.Since(start); dur > 200*time.Millisecond {
		t.Fatalf("listed prepare too slow: %s (must not fetch)", dur)
	}
	if play != raw {
		t.Fatalf("play=%q want %q", play, raw)
	}
	if direct != "" {
		t.Fatalf("direct=%q want empty", direct)
	}

	play, direct, got := a.PreparePlaybackListed(raw, map[string]string{"Cookie": "a=b", "User-Agent": "okhttp/5.5.0"})
	if play != raw || direct != "" {
		t.Fatalf("headed list play=%q direct=%q", play, direct)
	}
	if got["Cookie"] != "a=b" || got["User-Agent"] != "okhttp/5.5.0" {
		t.Fatalf("headers=%v", got)
	}

	withAdFilter(t, true)
	play, direct, _ = a.PreparePlaybackListed(raw, nil)
	if play != raw || direct != "" {
		t.Fatalf("ad list play=%q direct=%q want raw", play, direct)
	}
	mp4 := "https://cdn.example/live.mp4"
	play, direct, _ = a.PreparePlaybackListed(mp4, nil)
	if play != mp4 || direct != "" {
		t.Fatalf("mp4 play=%q direct=%q", play, direct)
	}
}

// 直播不去广告。有请求头也返回原地址，头留给播放器，不登记透传。
func TestPrepareLivePlaybackHeadersUseProxy(t *testing.T) {
	withAdFilter(t, true)
	a := &App{}
	raw := "https://cdn.example/live.m3u8"
	play, direct, _ := a.PrepareLivePlayback(raw, nil)
	if play != raw || direct != "" {
		t.Fatalf("plain play=%q direct=%q", play, direct)
	}
	headers := map[string]string{"Cookie": "a=b", "User-Agent": "okhttp/5.5.0"}
	play, direct, got := a.PrepareLivePlayback(raw, headers)
	if play != raw || direct != "" {
		t.Fatalf("play=%q direct=%q", play, direct)
	}
	if strings.Contains(play, "/proxy/play") {
		t.Fatalf("live must not proxy, play=%q", play)
	}
	if got["Cookie"] != "a=b" || got["User-Agent"] != "okhttp/5.5.0" {
		t.Fatalf("headers=%v", got)
	}
}
