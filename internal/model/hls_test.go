package model

import "testing"

func TestURLIsHls(t *testing.T) {
	yes := []string{
		"https://cdn/a.f300.m3u8",
		"https://cdn/a.f300.m3u8/",
		"https://cdn/play.php",
		"https://cdn/m3u8",
		"https://cdn/v?file=m3u8",
		"data:application/vnd.apple.mpegurl,abc",
	}
	no := []string{
		"https://cdn/fengshows?id=2.m3u8",
		"https://cdn/a.mp4",
		"https://cdn/a.m3u",
		"rtsp://cdn/a.m3u8",
		"https://cdn/live.m3u8?type=mpd",
		"data:application/dash+xml,abc",
	}
	for _, raw := range yes {
		if !URLIsHls(raw) {
			t.Fatalf("want hls: %s", raw)
		}
	}
	for _, raw := range no {
		if URLIsHls(raw) {
			t.Fatalf("want not hls: %s", raw)
		}
	}
}

func TestNormalizeManifest(t *testing.T) {
	if got := NormalizeManifest("hls"); got != "application/x-mpegURL" {
		t.Fatalf("hls=%q", got)
	}
	if got := NormalizeManifest("HLS"); got != "application/x-mpegURL" {
		t.Fatalf("HLS=%q", got)
	}
	if got := NormalizeManifest("m3u8"); got != "m3u8" {
		t.Fatalf("m3u8 word=%q", got)
	}
	if got := NormalizeManifest("mpd"); got != "application/dash+xml" {
		t.Fatalf("mpd=%q", got)
	}
	if got := NormalizeManifest("application/x-mpegURL"); got != "application/x-mpegurl" {
		t.Fatalf("mime=%q", got)
	}
}
