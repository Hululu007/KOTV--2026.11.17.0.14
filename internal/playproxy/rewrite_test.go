package playproxy

import (
	"strings"
	"testing"
)

func TestRewritePlaylistInjectOffKeepsCDNAbsolute(t *testing.T) {
	t.Parallel()
	in := "#EXTM3U\n#EXTINF:1,\nhttps://cdn.example/seg.png\n#EXT-X-KEY:METHOD=AES-128,URI=\"https://cdn.example/key\"\n"
	out := rewritePlaylist(in, "https://cdn.example/dir/v.m3u8", "abc", false)
	if !strings.Contains(out, "https://cdn.example/seg.png") {
		t.Fatalf("cdn segment should stay absolute: %q", out)
	}
	if !strings.Contains(out, `URI="https://cdn.example/key"`) {
		t.Fatalf("cdn key should stay absolute: %q", out)
	}
	if strings.Contains(out, "/proxy/play?") {
		t.Fatalf("inject=false should not proxy children: %q", out)
	}
}

func TestRewritePlaylistInjectOffAbsolutizesRelative(t *testing.T) {
	t.Parallel()
	in := "#EXTM3U\n#EXTINF:1,\nseg.ts\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n"
	out := rewritePlaylist(in, "https://cdn.example/dir/v.m3u8", "abc", false)
	if !strings.Contains(out, "https://cdn.example/dir/seg.ts") {
		t.Fatalf("relative segment should become absolute: %q", out)
	}
	if !strings.Contains(out, `URI="https://cdn.example/dir/key"`) {
		t.Fatalf("relative key should become absolute: %q", out)
	}
	if strings.Contains(out, "/proxy/play?") {
		t.Fatalf("inject=false should not proxy children: %q", out)
	}
}

func TestRewritePlaylistInjectOnProxiesChildren(t *testing.T) {
	t.Parallel()
	in := "#EXTM3U\n#EXTINF:1,\nhttps://cdn.example/seg.ts\n"
	out := rewritePlaylist(in, "https://cdn.example/dir/v.m3u8", "abc", true)
	if strings.Contains(out, "https://cdn.example/seg.ts\n") {
		t.Fatalf("inject=true should proxy children: %q", out)
	}
	if !strings.Contains(out, "/proxy/play?") || !strings.Contains(out, "u=") {
		t.Fatalf("expected proxied child: %q", out)
	}
}
