package spider

import (
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
)

func TestRewritePlayJSON(t *testing.T) {
	localproxy.SetPort(9978)
	p := &nodePkg{id: "abc123def456", port: 12345}
	in := `{
		"parse":0,
		"url":"js2p://_WEB_/spider/ffm3u8/3/proxy/hls/x/.m3u8",
		"url2":"http://127.0.0.1:12345/spider/a/3/proxy/ts/y/.ts",
		"url3":"http://localhost:12345/other"
	}`
	out := p.RewritePlayJSON(in)
	engine := "http://127.0.0.1:9978/node/abc123def456"
	if strings.Contains(out, "js2p://") {
		t.Fatalf("js2p not rewritten: %s", out)
	}
	if strings.Contains(out, "127.0.0.1:12345") || strings.Contains(out, "localhost:12345") {
		t.Fatalf("local port not rewritten: %s", out)
	}
	if !strings.Contains(out, engine+"/spider/ffm3u8/3/proxy/hls/x/.m3u8") {
		t.Fatalf("missing rewritten js2p path: %s", out)
	}
	if !strings.Contains(out, engine+"/spider/a/3/proxy/ts/y/.ts") {
		t.Fatalf("missing rewritten local url: %s", out)
	}
}

func TestRewriteNodeURL(t *testing.T) {
	localproxy.SetPort(9978)
	p := &nodePkg{id: "pkg1", port: 9999}
	got := p.RewriteNodeURL("http://127.0.0.1:9999/danmu/abc")
	want := "http://127.0.0.1:9978/node/pkg1/danmu/abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRewriteNodeURLRemotePublicize(t *testing.T) {
	localproxy.SetPort(9978)
	defer hostclient.Bind(hostclient.Request{
		ClientID:   "node-pub",
		PublicBase: "https://tv.example.com/k/KEY",
	})()
	p := &nodePkg{id: "pkg1", port: 9999}
	got := p.RewriteNodeURL("http://127.0.0.1:9999/danmu/abc")
	want := "https://tv.example.com/k/KEY/node/pkg1/danmu/abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	out := p.RewritePlayJSON(`{"url":"js2p://_WEB_/x.m3u8"}`)
	if !strings.Contains(out, want[:strings.Index(want, "/danmu")]) {
		t.Fatalf("play json not publicized: %s", out)
	}
}

func TestHandleNodeMsgToast(t *testing.T) {
	var got []string
	SetPostMsgEmitter(func(msg, clientID string) {
		got = append(got, msg)
	})
	t.Cleanup(func() { SetPostMsgEmitter(nil) })

	resp := HandleNodeMsg([]byte(`{"action":"toast","opt":{"message":"你好"}}`))
	if m, ok := resp.(map[string]any); !ok || m["ok"] != true {
		t.Fatalf("resp=%v", resp)
	}
	if len(got) != 1 || got[0] != "你好" {
		t.Fatalf("got %v", got)
	}
}

func TestHandleNodeMsgGetPlayInfo(t *testing.T) {
	SetCurrentPlayInfo("牧神记", "第01集", "线路1")
	t.Cleanup(func() { SetCurrentPlayInfo("", "", "") })
	resp := HandleNodeMsg([]byte(`{"action":"getPlayInfo"}`))
	m, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("resp=%T", resp)
	}
	if m["title"] != "牧神记" || m["episodeName"] != "第01集" || m["flag"] != "线路1" {
		t.Fatalf("resp=%v", m)
	}
}

func TestHandleNodeMsgUnknown(t *testing.T) {
	if HandleNodeMsg([]byte(`{"action":"nope"}`)) != nil {
		t.Fatal("want nil")
	}
}
