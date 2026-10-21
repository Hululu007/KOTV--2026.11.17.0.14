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
