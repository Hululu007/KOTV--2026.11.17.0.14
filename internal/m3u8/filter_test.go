package m3u8

import (
	"strings"
	"testing"
)

func TestNoEndlistUnchanged(t *testing.T) {
	in := "" +
		"#EXTM3U\n" +
		"#EXTINF:4.0,\nhttps://cdn.example/a/1.ts\n" +
		"#EXT-X-DISCONTINUITY\n" +
		"#EXTINF:1.0,\nhttps://ad.example/x/ad.ts\n"
	if got := Process(in); got != in {
		t.Fatalf("no ENDLIST must stay original\n got:\n%s", got)
	}
}

func TestLastDiscontinuityBlockKept(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := 1; i <= 8; i++ {
		b.WriteString("#EXTINF:4.0,\nhttps://cdn.example/a/" + itoa(i) + ".ts\n")
	}
	b.WriteString("#EXT-X-DISCONTINUITY\n#EXTINF:1.0,\nhttps://cdn.example/a/tail.ts\n#EXT-X-ENDLIST\n")
	in := b.String()
	got := Process(in)
	if got != in {
		t.Fatalf("short last block must stay\n got:\n%s", got)
	}
}

// 仅主机不同、无 DISCONTINUITY：证据不足，不删（避免双 CDN 正片被当成广告）。
func TestHostOnlyDifferenceKept(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n")
	for i := 1; i <= 8; i++ {
		b.WriteString("#EXTINF:4.0,\nhttps://cdn.example/a/" + itoa(i) + ".ts\n")
	}
	b.WriteString("#EXTINF:4.0,\nhttps://ad.example/x/ad1.ts\n")
	b.WriteString("#EXTINF:4.0,\nhttps://ad.example/x/ad2.ts\n")
	b.WriteString("#EXT-X-ENDLIST\n")
	in := b.String()
	if got := Process(in); got != in {
		t.Fatalf("host-only difference without discontinuity must stay\n got:\n%s", got)
	}
}

func TestEqualHostsNotCut(t *testing.T) {
	in := "#EXTM3U\n" +
		"#EXTINF:4.0,\nhttps://a.example/1.ts\n" +
		"#EXTINF:4.0,\nhttps://a.example/2.ts\n" +
		"#EXTINF:4.0,\nhttps://b.example/3.ts\n" +
		"#EXTINF:4.0,\nhttps://b.example/4.ts\n" +
		"#EXT-X-ENDLIST\n"
	if got := Process(in); got != in {
		t.Fatalf("no majority, keep original\n%s", got)
	}
}

// 逐片 DISCONTINUITY + 双主机：众数块长为 1，不连续规则不删。
func TestPerSegmentDiscMultiHostKept(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:50\n")
	for i := 1; i <= 4; i++ {
		b.WriteString("#EXTINF:3.0,\n#EXT-X-DISCONTINUITY\n")
		b.WriteString("http://cdn-a.example/play/ts/seg" + itoa(i) + "\n")
	}
	for i := 1; i <= 12; i++ {
		b.WriteString("#EXTINF:3.0,\n#EXT-X-DISCONTINUITY\n")
		b.WriteString("https://cdn-b.example/file/" + itoa(i) + "/clip.png\n")
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	in := b.String()
	got := Process(in)
	if got != in {
		t.Fatalf("per-segment discontinuity multi-host must stay intact\n got:\n%s", got)
	}
	if !strings.Contains(got, "cdn-a.example") || !strings.Contains(got, "cdn-b.example") {
		t.Fatalf("both hosts must stay:\n%s", got)
	}
}

func TestSameHostHashNamesKept(t *testing.T) {
	in := "#EXTM3U\n" +
		"#EXTINF:4.0,\nhttps://cdn.example/v/a75c0511.ts\n" +
		"#EXTINF:4.0,\nhttps://cdn.example/v/11eed2e8.ts\n" +
		"#EXTINF:4.0,\nhttps://cdn.example/v/3ad8620b.ts\n" +
		"#EXTINF:4.0,\nhttps://cdn.example/v/9f00aa11.ts\n" +
		"#EXT-X-ENDLIST\n"
	if got := Process(in); got != in {
		t.Fatalf("one host must not be cut by name:\n%s", got)
	}
}

func TestTooManyMinorityBlocksKept(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for n := 0; n < 5; n++ {
		writeSegs(&b, "https://cdn.example/a/"+itoa(n), 1, 8)
		b.WriteString("#EXT-X-DISCONTINUITY\n")
	}
	for n := 0; n < 4; n++ {
		b.WriteString("#EXTINF:1.0,\nhttps://cdn.example/a/ad" + itoa(n) + ".ts\n#EXT-X-DISCONTINUITY\n")
	}
	writeSegs(&b, "https://cdn.example/a/z", 1, 8)
	b.WriteString("#EXT-X-ENDLIST\n")
	in := b.String()
	if got := Process(in); got != in {
		t.Fatalf("too many minority blocks must stay\n%s", got)
	}
}

func TestOneMinorityBlockRemoved(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n")
	writeSegs(&b, "https://cdn.example/a/", 1, 8)
	b.WriteString("#EXT-X-DISCONTINUITY\n#EXTINF:4.0,\nhttps://cdn.example/a/ad.ts\n#EXT-X-DISCONTINUITY\n")
	writeSegs(&b, "https://cdn.example/a/z", 1, 8)
	b.WriteString("#EXT-X-ENDLIST\n")
	got := Process(b.String())
	if strings.Contains(got, "/ad.ts") {
		t.Fatalf("ad segment should go:\n%s", got)
	}
	if strings.Contains(got, "#EXT-X-DISCONTINUITY") {
		t.Fatalf("orphan discontinuity should go:\n%s", got)
	}
	if !strings.Contains(got, "#EXT-X-VERSION:3") || !strings.Contains(got, "#EXT-X-TARGETDURATION:10") {
		t.Fatalf("playlist tags must stay:\n%s", got)
	}
	if !strings.Contains(got, "https://cdn.example/a/1.ts") || !strings.Contains(got, "https://cdn.example/a/z8.ts") {
		t.Fatalf("both content blocks must stay:\n%s", got)
	}
	if !strings.Contains(got, "#EXT-X-ENDLIST") {
		t.Fatalf("endlist must stay:\n%s", got)
	}
}

func writeSegs(b *strings.Builder, prefix string, from, n int) {
	for i := from; i < from+n; i++ {
		b.WriteString("#EXTINF:4.0,\n")
		b.WriteString(prefix)
		b.WriteString(itoa(i))
		b.WriteString(".ts\n")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [12]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
