package spider

import "testing"

func TestJsPostMsgKeepsScope(t *testing.T) {
	var got []string
	SetPostMsgEmitter(func(msg, clientID string) {
		got = append(got, clientID+" "+msg)
	})
	t.Cleanup(func() { SetPostMsgEmitter(nil) })

	jsPostMsg("UI:1", "u:user")
	jsPostMsg("UI:2", "c:abc")
	jsPostMsg("   ", "u:user")

	if len(got) != 2 || got[0] != "u:user UI:1" || got[1] != "c:abc UI:2" {
		t.Fatalf("got %v", got)
	}
}

func TestJsExplicitScope(t *testing.T) {
	if got := jsExplicitScope("c:local", ""); got != "c:local" {
		t.Fatalf("empty passed: %q", got)
	}
	if got := jsExplicitScope("c:local", "  "); got != "c:local" {
		t.Fatalf("blank passed: %q", got)
	}
	if got := jsExplicitScope("", "u:user-1"); got != "u:user-1" {
		t.Fatalf("saved scope: %q", got)
	}
	if got := jsExplicitScope("c:local", "u:other"); got != "u:other" {
		t.Fatalf("override: %q", got)
	}
}
