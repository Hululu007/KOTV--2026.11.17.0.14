package util

import "testing"

func TestMaskURLCredentials(t *testing.T) {
	got := MaskURLCredentials("http://user:secret@example.com/index.js.md5")
	if got != "http://user:***@example.com/index.js.md5" {
		t.Fatalf("got %q", got)
	}
	plain := "http://example.com/a"
	if MaskURLCredentials(plain) != plain {
		t.Fatalf("plain changed")
	}
}

func TestNodePackageBaseURL(t *testing.T) {
	got := NodePackageBaseURL("http://u:p@h/dir/index.js.md5")
	if got != "http://u:p@h/dir/" {
		t.Fatalf("md5 got %q", got)
	}
	got = NodePackageBaseURL("http://h/index.js")
	if got != "http://h/" {
		t.Fatalf("js got %q", got)
	}
}
