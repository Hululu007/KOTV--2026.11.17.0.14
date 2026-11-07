package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCatVodJSON_VideoSites(t *testing.T) {
	in := `{"video":{"sites":[{"key":"nodejs_wogg","api":"/spider/wogg/3","type":3}]},"color":[]}`
	out, ignored := NormalizeCatVodJSON(in)
	if ignored != 0 {
		t.Fatalf("ignored=%d", ignored)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	var sites []map[string]any
	if err := json.Unmarshal(root["sites"], &sites); err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0]["key"] != "nodejs_wogg" {
		t.Fatalf("sites=%v", sites)
	}
}

func TestNormalizeCatVodJSON_PreferVideoWhenBoth(t *testing.T) {
	in := `{
		"sites":[{"key":"top","api":"csp_X"}],
		"video":{"sites":[{"key":"nodejs_a","api":"/spider/a/3"}]},
		"read":{"sites":[{"key":"book1"}]},
		"comic":{"sites":[{"key":"c1"},{"key":"c2"}]},
		"music":{},
		"pan":{"sites":[]}
	}`
	out, ignored := NormalizeCatVodJSON(in)
	if ignored != 3 { // read:1 + comic:2 + pan:0
		t.Fatalf("ignored=%d want 3", ignored)
	}
	var root struct {
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Sites) != 1 || root.Sites[0]["key"] != "nodejs_a" {
		t.Fatalf("sites=%v", root.Sites)
	}
}

func TestNormalizeCatVodJSON_TopOnlyUntouched(t *testing.T) {
	in := `{"sites":[{"key":"a","api":"csp_A"}],"spider":"x.jar"}`
	out, ignored := NormalizeCatVodJSON(in)
	if ignored != 0 {
		t.Fatalf("ignored=%d", ignored)
	}
	if !strings.Contains(out, `"key":"a"`) {
		t.Fatalf("out=%s", out)
	}
}

func TestNormalizeCatVodJSON_EmptyTopUsesVideo(t *testing.T) {
	in := `{"sites":[],"video":{"sites":[{"key":"n1","api":"/spider/n/3"}]}}`
	out, _ := NormalizeCatVodJSON(in)
	var root struct {
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Sites) != 1 || root.Sites[0]["key"] != "n1" {
		t.Fatalf("sites=%v", root.Sites)
	}
}

func TestIsNodePackageURL(t *testing.T) {
	cases := []struct {
		u    string
		want bool
	}{
		{"http://u:p@host/path/index.js", true},
		{"http://u:p@host/path/index.js.md5", true},
		{"https://host/index.js.md5?x=1", true},
		{"http://host/config.json", false},
		{"http://host/foo.js", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsNodePackageURL(c.u); got != c.want {
			t.Fatalf("%q: got %v want %v", c.u, got, c.want)
		}
	}
}

func TestMaskURLCredentials(t *testing.T) {
	got := MaskURLCredentials("http://user:secret@example.com/index.js.md5")
	if strings.Contains(got, "secret") {
		t.Fatalf("password leaked: %s", got)
	}
	if !strings.Contains(got, "user:") || !strings.Contains(got, "***") {
		t.Fatalf("got %s", got)
	}
	plain := "http://example.com/index.js"
	if MaskURLCredentials(plain) != plain {
		t.Fatalf("plain changed")
	}
}

func TestNodePackageBaseURL(t *testing.T) {
	got := NodePackageBaseURL("http://u:p@h/dir/index.js.md5")
	if got != "http://u:p@h/dir/" {
		t.Fatalf("got %q", got)
	}
	got = NodePackageBaseURL("http://h/index.js")
	if got != "http://h/" {
		t.Fatalf("got %q", got)
	}
}
