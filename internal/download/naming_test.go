package download

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/settings"
)

func TestSanitizeNameKeepsPlayStringSafe(t *testing.T) {
	cases := map[string]string{
		"第#1集":          "第_1集",
		"A$B＄C":         "A_B_C",
		`a/b\c:d*e?f"g`: "a_b_c_d_e_f_g",
		"  ":            "未命名",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvePathOwnerSubdir(t *testing.T) {
	dir := t.TempDir()
	settings.Set(settings.DownloadDir, dir)
	defer settings.Set(settings.DownloadDir, "")
	local := ResolvePath("", "片名", "线路", "第1集", "mp4")
	if !strings.HasPrefix(local, dir+string(filepath.Separator)) || strings.Contains(local, "未命名") {
		t.Fatalf("local download path: %s", local)
	}
	remote := ResolvePath("alice", "片名", "线路", "第1集", "mp4")
	if !strings.HasPrefix(remote, filepath.Join(dir, "alice")+string(filepath.Separator)) {
		t.Fatalf("remote download path: %s", remote)
	}
}

func TestViewerSees(t *testing.T) {
	mine, other, local := &Task{Owner: "u1"}, &Task{Owner: "u2"}, &Task{}
	user := Viewer{Owner: "u1"}
	if !user.sees(mine) || user.sees(other) || user.sees(local) {
		t.Fatal("remote user only sees own tasks")
	}
	admin := Viewer{Owner: "u2", All: true}
	if !admin.sees(mine) || !admin.sees(other) || !admin.sees(local) {
		t.Fatal("admin sees all tasks")
	}
	host := Viewer{}
	if host.sees(mine) != false || !host.sees(local) {
		t.Fatal("viewer without All only sees owner-less tasks")
	}
}
