package runtime

import (
	"runtime"
	"testing"
)

func TestPlatformNames(t *testing.T) {
	os, arch := OS(), Arch()
	if os == "darwin" || arch == "amd64" || arch == "386" {
		t.Fatalf("names must match the client: os=%q arch=%q", os, arch)
	}
	if runtime.GOOS == "darwin" && os != "macos" {
		t.Fatalf("darwin -> %q", os)
	}
	if runtime.GOARCH == "amd64" && arch != "x64" {
		t.Fatalf("amd64 -> %q", arch)
	}
	if Platform() != os+"-"+arch {
		t.Fatalf("Platform()=%q", Platform())
	}
}
