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
