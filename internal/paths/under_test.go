package paths

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathUnderRootSameAndChild(t *testing.T) {
	root := filepath.Clean("/data/media")
	child := filepath.Join(root, "Movies", "a.mp4")
	if !PathUnderRoot(root, root) {
		t.Fatal("root should contain itself")
	}
	if !PathUnderRoot(child, root) {
		t.Fatalf("%q should be under %q", child, root)
	}
	if PathUnderRoot(filepath.Clean("/data"), root) {
		t.Fatal("parent must not count as under root")
	}
	if PathUnderRoot(filepath.Clean("/other"), root) {
		t.Fatal("sibling tree must not count")
	}
}

func TestPathUnderRootWindowsDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	root := `C:\`
	child := `C:\Users\demo\Downloads`
	if !PathUnderRoot(root, root) {
		t.Fatal("drive root")
	}
	if !PathUnderRoot(child, root) {
		t.Fatalf("%q under %q", child, root)
	}
	if PathUnderRoot(`D:\x`, root) {
		t.Fatal("other drive")
	}
}
