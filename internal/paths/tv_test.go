package paths

import "testing"

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
