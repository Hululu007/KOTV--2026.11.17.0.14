package player

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalArgvDarwinApps(t *testing.T) {
	url := "http://127.0.0.1/proxy/play?x=1"
	vlcBin := "/Applications/VLC.app/Contents/MacOS/VLC"
	argv, dir, err := externalArgv("darwin", "vlc", url, "", vlcBin, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "" {
		t.Fatalf("dir=%q", dir)
	}
	want := []string{"open", "-a", "/Applications/VLC.app", url}
	if strings.Join(argv, "\n") != strings.Join(want, "\n") {
		t.Fatalf("vlc argv=%q", argv)
	}

	argv, _, err = externalArgv("darwin", "vlc", url, "", "/usr/local/bin/vlc", "/Users/me/Applications/VLC.app", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if argv[2] != "/Users/me/Applications/VLC.app" {
		t.Fatalf("vlc app=%q", argv)
	}

	if _, _, err = externalArgv("darwin", "vlc", url, "", "/usr/local/bin/vlc", "", "", ""); err == nil {
		t.Fatal("darwin vlc without app should fail")
	}

	argv, _, err = externalArgv("darwin", "iina", url, "", "", "", "/Users/me/Applications/IINA.app", "")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"open", "-a", "/Users/me/Applications/IINA.app", url}
	if strings.Join(argv, "\n") != strings.Join(want, "\n") {
		t.Fatalf("iina argv=%q", argv)
	}
	if _, _, err = externalArgv("darwin", "iina", url, "", "", "", "", ""); err == nil {
		t.Fatal("missing IINA should fail")
	}
}

func TestExternalArgvRejectsIINAOffMac(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		_, _, err := externalArgv(goos, "iina", "http://a", "", "", "", "/Applications/IINA.app", "")
		if err == nil || !strings.Contains(err.Error(), "macOS") {
			t.Fatalf("%s iina err=%v", goos, err)
		}
	}
}

func TestExternalArgvExecutable(t *testing.T) {
	url := "http://127.0.0.1/a.mp4"
	for _, goos := range []string{"windows", "linux"} {
		bin := "/opt/vlc"
		if goos == "windows" {
			bin = `C:/Program Files/VideoLAN/VLC/vlc.exe`
		}
		argv, dir, err := externalArgv(goos, "vlc", url, "", bin, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if dir != "" || len(argv) != 2 || argv[0] != bin || argv[1] != url {
			t.Fatalf("%s vlc argv=%q dir=%q", goos, argv, dir)
		}
	}

	mpv := "/usr/bin/mpv"
	argv, dir, err := externalArgv("linux", "mpv", url, mpv, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Dir(mpv) {
		t.Fatalf("dir=%q", dir)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--force-window=yes") || strings.Contains(joined, "--input-ipc-server=") {
		t.Fatalf("mpv argv=%q", argv)
	}
	if argv[0] != mpv || argv[len(argv)-1] != url {
		t.Fatalf("mpv argv=%q", argv)
	}

	argv, _, err = externalArgv("linux", "mpv", url, mpv, "", "", "", "/tmp/kotv.sock")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argv, " "), "--input-ipc-server=/tmp/kotv.sock") {
		t.Fatalf("ipc argv=%q", argv)
	}
	if _, _, err = externalArgv("windows", "mpv", url, "", "", "", "", ""); err == nil {
		t.Fatal("missing mpv should fail")
	}
}
