package paths

import (
	"strings"
	"testing"
)

func TestTVDirNameUsername(t *testing.T) {
	cases := []struct {
		in, wantDir, wantSeg string
	}{
		{"", "", ""},
		{"c:abc", "", ""},
		{"alice", "u_alice", "s_u_alice"},
		{"u:alice", "u_alice", "s_u_alice"},
		{"u_alice", "u_alice", "s_u_alice"},
		{"s_u_alice", "u_alice", "s_u_alice"},
		{"a/b:c", "u_a_b_c", "s_u_a_b_c"},
	}
	for _, c := range cases {
		if got := TVDirName(c.in); got != c.wantDir {
			t.Fatalf("TVDirName(%q)=%q want %q", c.in, got, c.wantDir)
		}
		if got := TVUserSeg(c.in); got != c.wantSeg {
			t.Fatalf("TVUserSeg(%q)=%q want %q", c.in, got, c.wantSeg)
		}
	}
}

func TestIsLegacyTVDir(t *testing.T) {
	if !IsLegacyTVDir("s_c_deadbeef") {
		t.Fatal("s_c_")
	}
	if !IsLegacyTVDir("s_u_0123456789abcdef0123456789abcdef") {
		t.Fatal("s_u_uuid")
	}
	if IsLegacyTVDir("s_u_alice") {
		t.Fatal("keep username dir")
	}
	if IsLegacyTVDir(".cookie") {
		t.Fatal("keep root file-like name")
	}
}

func TestSanitizeTVUsernameRuneCap(t *testing.T) {
	// 65 个汉字 → 64
	var b strings.Builder
	for i := 0; i < 65; i++ {
		b.WriteRune('中')
	}
	got := TVUsername(b.String())
	if n := len([]rune(got)); n != 64 {
		t.Fatalf("rune len=%d want 64 (%q)", n, got)
	}
}
