package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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

func TestPythonUiLink(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip(err)
	}
	s := New(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/ui", s.handleUI)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")
	port := strings.TrimPrefix(ts.URL, "http://127.0.0.1:")

	ui, _, err := websocket.DefaultDialer.Dial(base+"/ui?role=ui&clientId=abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	_ = ui.SetReadDeadline(time.Now().Add(5 * time.Second))

	dir := t.TempDir()
	spider := filepath.Join(dir, "spider.py")
	if err := os.WriteFile(spider, []byte("class Spider:\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(repo, "internal", "spider", "pyrunner.py")); err == nil {
			break
		}
		parent := filepath.Dir(repo)
		if parent == repo {
			t.Fatal("pyrunner.py not found")
		}
		repo = parent
	}
	code := `
import sys, time
spider, cache, py_dir, port = sys.argv[1:]
sys.argv = ["pyrunner.py", spider, "k", "", "", cache]
sys.path.insert(0, py_dir)
import pyrunner
pyrunner._UI.send("from-py", int(port))
for _ in range(50):
    got = pyrunner._UI.take("sess")
    if got:
        sys.stdout.write(got)
        raise SystemExit(0)
    time.sleep(0.05)
raise SystemExit("no reply")
`
	cmd := exec.Command("python3", "-c", code, spider, dir, filepath.Join(repo, "internal", "spider"), port)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	_, msg, rerr := ui.ReadMessage()
	if rerr != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("ui read: %v python: %s", rerr, output.String())
	}
	if string(msg) != "from-py" {
		_ = cmd.Process.Kill()
		t.Fatalf("python frame %q python: %s", msg, output.String())
	}
	reply := `{"id":"sess","action":"positive"}`
	if err := ui.WriteMessage(websocket.TextMessage, []byte(reply)); err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("python: %v %s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("python timeout %s", output.String())
	}
	if strings.TrimSpace(output.String()) != reply {
		t.Fatalf("python reply %q", output.String())
	}
}
