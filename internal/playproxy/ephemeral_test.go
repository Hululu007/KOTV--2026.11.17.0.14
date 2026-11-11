package playproxy

import (
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
)

func TestIsEphemeralURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		u    string
		want bool
	}{
		{"http://127.0.0.1:9978/proxy/play?id=1", true},
		{"http://127.0.0.1:9978/proxy/cached_m3u8?id=1", true},
		{"http://127.0.0.1:9978/proxy/rolling_m3u8?id=1", true},
		{"http://127.0.0.1:9978/proxy/hls/x", true},
		{"http://127.0.0.1:9978/proxy/bt/x", true},
		{"http://127.0.0.1:9978/proxy?do=quark", false},
		{"https://cdn.example/a.m3u8", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsEphemeralURL(c.u); got != c.want {
			t.Fatalf("IsEphemeralURL(%q)=%v want %v", c.u, got, c.want)
		}
	}
}

func TestRegisterSkipsEphemeral(t *testing.T) {
	t.Parallel()
	in := "http://127.0.0.1:9978/proxy/hls/abc"
	if got := Register(in, nil, false); got != in {
		t.Fatalf("Register ephemeral should return same url, got %q", got)
	}
}

func TestPublicizeHostDoesNotRegisterLoopback(t *testing.T) {
	defer hostclient.Bind(hostclient.Request{ClientID: "host-only", PublicBase: "https://tv.example.com:8443"})()

	raw := "http://127.0.0.1:5266/fishplay/go/quark/x"
	got := PublicizeHost(raw)
	if got != raw {
		t.Fatalf("PublicizeHost must not rewrite non-engine path, got %q", got)
	}
	if strings.Contains(got, "/proxy/play?") {
		t.Fatalf("PublicizeHost must not register: %q", got)
	}

	proxied := PublicizeURL(raw)
	if !strings.Contains(proxied, "https://tv.example.com:8443/proxy/play?id=") {
		t.Fatalf("PublicizeURL should still register loopback other port, got %q", proxied)
	}
}
