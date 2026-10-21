package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestUISocketFlushAndReply(t *testing.T) {
	s := New(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/ui", s.handleUI)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	s.events.EmitPostMsgTo("early", "")
	s.events.EmitPostMsgTo("hello", "c:abc")

	ui, _, err := websocket.DefaultDialer.Dial(base+"/ui?role=ui&clientId=abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	_ = ui.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, first, err := ui.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := ui.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "early" || string(second) != "hello" {
		t.Fatalf("flush got %q %q", first, second)
	}

	src, _, err := websocket.DefaultDialer.Dial(base+"/ui?role=src", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.WriteMessage(websocket.TextMessage, []byte(`{"msg":"from-jar","clientId":"abc"}`)); err != nil {
		t.Fatal(err)
	}
	_, pushed, err := ui.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(pushed) != "from-jar" {
		t.Fatalf("src frame %q", pushed)
	}

	reply := `{"id":"s1","action":"shown","values":{"platform":"android"}}`
	if err := ui.WriteMessage(websocket.TextMessage, []byte(reply)); err != nil {
		t.Fatal(err)
	}
	_ = src.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, got, err := src.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != reply {
		t.Fatalf("reply %q", got)
	}

	s.events.EmitPostMsgTo("toast", "")
	_, broadcast, err := ui.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(broadcast) != "toast" {
		t.Fatalf("broadcast %q", broadcast)
	}
}

func TestSrcFrameUsesPyCallScope(t *testing.T) {
	s := New(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/ui", s.handleUI)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	prev := pyCallScope
	prevRecent := recentPyScope
	t.Cleanup(func() {
		pyCallScope = prev
		recentPyScope = prevRecent
	})
	recentPyScope = func() string { return "" }

	dial := func(raw string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(base+raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	send := func(conn *websocket.Conn, text string) {
		t.Helper()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("remote", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		pyCallScope = func() (string, bool) { return "u:user", false }
		src := dial("/ui?role=src")
		send(src, `{"msg":"UI:remote-only"}`)
		assertUiRead(t, remote, "UI:remote-only")
		assertUiSilent(t, phone)
	})

	t.Run("envelope", func(t *testing.T) {
		remote := dial("/ui?role=ui&userId=user")
		other := dial("/ui?role=ui&userId=other")
		pyCallScope = func() (string, bool) { return "u:user", false }
		src := dial("/ui?role=src")
		send(src, `{"msg":"UI:from-envelope","userId":"other"}`)
		assertUiRead(t, other, "UI:from-envelope")
		assertUiSilent(t, remote)
	})

	t.Run("local", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		pyCallScope = func() (string, bool) { return "c:phone", false }
		src := dial("/ui?role=src")
		send(src, `{"msg":"UI:local-only"}`)
		assertUiRead(t, phone, "UI:local-only")
		assertUiSilent(t, remote)
	})

	t.Run("reuse", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		src := dial("/ui?role=src")
		pyCallScope = func() (string, bool) { return "u:user", false }
		send(src, `{"msg":"UI:remote-only"}`)
		assertUiRead(t, remote, "UI:remote-only")
		pyCallScope = func() (string, bool) { return "", false }
		send(src, `{"msg":"UI_CLOSE:remote-only"}`)
		assertUiRead(t, remote, "UI_CLOSE:remote-only")
		pyCallScope = func() (string, bool) { return "c:phone", false }
		send(src, `{"msg":"UI:local-only"}`)
		assertUiRead(t, phone, "UI:local-only")
		assertUiSilent(t, remote)
	})

	t.Run("drop", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		pyCallScope = func() (string, bool) { return "", false }
		src := dial("/ui?role=src")
		send(src, `{"msg":"UI:nobody"}`)
		assertUiSilent(t, phone)
		assertUiSilent(t, remote)
	})

	t.Run("conflict", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		src := dial("/ui?role=src")
		pyCallScope = func() (string, bool) { return "u:user", false }
		send(src, `{"msg":"UI:remote-only"}`)
		assertUiRead(t, remote, "UI:remote-only")
		pyCallScope = func() (string, bool) { return "", true }
		send(src, `{"msg":"UI:ambiguous"}`)
		assertUiSilent(t, remote)
		assertUiSilent(t, phone)
	})

	t.Run("after return", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		pyCallScope = func() (string, bool) { return "", false }
		recentPyScope = func() string { return "c:phone" }
		src := dial("/ui?role=src")
		send(src, `{"msg":"UI:late"}`)
		assertUiRead(t, phone, "UI:late")
		assertUiSilent(t, remote)
	})

	t.Run("close stays", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		src := dial("/ui?role=src")
		pyCallScope = func() (string, bool) { return "c:phone", false }
		recentPyScope = func() string { return "c:phone" }
		send(src, `{"msg":"UI:open"}`)
		assertUiRead(t, phone, "UI:open")
		pyCallScope = func() (string, bool) { return "", false }
		recentPyScope = func() string { return "u:user" }
		send(src, `{"msg":"UI_CLOSE:open"}`)
		assertUiRead(t, phone, "UI_CLOSE:open")
		send(src, `{"msg":"UI:next"}`)
		assertUiRead(t, remote, "UI:next")
		assertUiSilent(t, phone)
	})

	t.Run("toast", func(t *testing.T) {
		phone := dial("/ui?role=ui&clientId=phone")
		remote := dial("/ui?role=ui&userId=user")
		pyCallScope = func() (string, bool) { return "u:user", false }
		src := dial("/ui?role=src")
		send(src, `{"msg":"toast-all"}`)
		assertUiRead(t, phone, "toast-all")
		assertUiRead(t, remote, "toast-all")
	})
}

func assertUiRead(t *testing.T, conn *websocket.Conn, want string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func assertUiSilent(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, got, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("unexpected %q", got)
	}
}
