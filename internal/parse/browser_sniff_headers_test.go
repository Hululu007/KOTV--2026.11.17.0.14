package parse

import "testing"

func TestSniffHeaderBookResponseFirstKeepsRequestHeaders(t *testing.T) {
	b := newSniffHeaderBook()
	if _, ok := b.headersOrWait("req-1", "https://cdn.example/a.m3u8"); ok {
		t.Fatal("response must wait for the request headers")
	}
	pending := b.noteRequest("req-1", map[string]string{"Referer": "https://player.example/"})
	if pending != "https://cdn.example/a.m3u8" {
		t.Fatalf("pending url: %q", pending)
	}
}

func TestSniffHeaderBookRequestFirst(t *testing.T) {
	b := newSniffHeaderBook()
	if pending := b.noteRequest("req-1", map[string]string{"Referer": "https://player.example/"}); pending != "" {
		t.Fatalf("no pending url: %q", pending)
	}
	h, ok := b.headersOrWait("req-1", "https://cdn.example/a.m3u8")
	if !ok || h["Referer"] != "https://player.example/" || len(h) != 1 {
		t.Fatalf("request headers: ok=%v %v", ok, h)
	}
}
