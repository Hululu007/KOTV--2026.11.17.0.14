package playproxy

import (
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
)

func TestPublicizeURLRewritesPrivateAndLoopback(t *testing.T) {
	defer hostclient.Bind(hostclient.Request{ClientID: "pub-test", PublicBase: "https://tv.example.com"})()

	cases := []struct {
		in   string
		want string
	}{
		{"http://127.0.0.1:9978/proxy/play?id=a", "https://tv.example.com/proxy/play?id=a"},
		{"http://192.168.1.8:9978/proxy?do=quark", "https://tv.example.com/proxy?do=quark"},
		{"http://10.0.0.2:9978/proxy/play?id=1", "https://tv.example.com/proxy/play?id=1"},
		{"https://cdn.example/a.m3u8", "https://cdn.example/a.m3u8"},
		{"https://tv.example.com/proxy/play?id=z", "https://tv.example.com/proxy/play?id=z"},
	}
	for _, c := range cases {
		got := PublicizeURL(c.in)
		if got != c.want {
			t.Fatalf("PublicizeURL(%q)=%q want %q", c.in, got, c.want)
		}
	}
	if !strings.HasPrefix(PublicizeURL("http://tv.example.com/proxy/x"), "https://") {
		t.Fatal("same host should unify scheme to https")
	}
}

func TestPublicizeURLUsesClientOriginPort(t *testing.T) {
	defer hostclient.Bind(hostclient.Request{ClientID: "pub-port", PublicBase: "https://tv.example.com:8443"})()

	got := PublicizeURL("http://192.168.0.113:9978/proxy/play?id=a")
	if got != "https://tv.example.com:8443/proxy/play?id=a" {
		t.Fatalf("got %q", got)
	}
	got = PublicizeURL("http://tv.example.com:9978/file/a.jpg")
	if got != "https://tv.example.com:8443/file/a.jpg" {
		t.Fatalf("listen port leaked: %q", got)
	}
	got = PublicizeURL("http://10.0.0.2:8080/live.m3u8")
	if got != "http://10.0.0.2:8080/live.m3u8" {
		t.Fatalf("other host must stay, got %q", got)
	}
	got = PublicizeURL("http://127.0.0.1:5266/fishplay/go/quark/x")
	if !strings.Contains(got, "https://tv.example.com:8443/proxy/play?id=") {
		t.Fatalf("loopback port should be relayed on the client origin, got %q", got)
	}
}

func TestPublicizeURLAddsMediaKey(t *testing.T) {
	SetEngineRoutes([]string{"/proxy", "/proxy/play", "/file/", "/image/", "/api/v1/home"})
	defer SetEngineRoutes(nil)
	defer hostclient.Bind(hostclient.Request{PublicBase: "https://tv.example.com/k/KEY"})()

	cases := []struct{ in, want string }{
		{"http://127.0.0.1:9978/proxy/play?id=a", "https://tv.example.com/k/KEY/proxy/play?id=a"},
		{"http://192.168.1.8:9978/file/a%20b.mp4", "https://tv.example.com/k/KEY/file/a%20b.mp4"},
		{"https://tv.example.com/k/KEY/image/x", "https://tv.example.com/k/KEY/image/x"},
		{"http://127.0.0.1:9978/api/v1/home", "https://tv.example.com/api/v1/home"},
		{"https://cdn.example/a.m3u8", "https://cdn.example/a.m3u8"},
	}
	for _, c := range cases {
		if got := PublicizeURL(c.in); got != c.want {
			t.Fatalf("PublicizeURL(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestMediaKeyPath(t *testing.T) {
	if key, rest, ok := SplitMediaKey("/k/abc/proxy/play"); !ok || key != "abc" || rest != "/proxy/play" {
		t.Fatalf("split: %q %q %v", key, rest, ok)
	}
	if _, _, ok := SplitMediaKey("/k/abc"); ok {
		t.Fatal("key without path must not split")
	}
	if got := WithMediaKey("http://192.168.1.8:9978/proxy/play?id=1", "abc"); got != "http://192.168.1.8:9978/k/abc/proxy/play?id=1" {
		t.Fatalf("with key: %q", got)
	}
	if got := WithMediaKey("http://127.0.0.1:9978/proxy/play?id=1", "abc"); got != "http://127.0.0.1:9978/proxy/play?id=1" {
		t.Fatalf("loopback must stay: %q", got)
	}
	if got := WithMediaKey("http://192.168.1.8:9978/api/v1/home", "abc"); got != "http://192.168.1.8:9978/api/v1/home" {
		t.Fatalf("api must stay: %q", got)
	}
}
