package playproxy

import (
	"bytes"
	"io"
	"testing"
)

func TestShouldProbePrefixedTS(t *testing.T) {
	if !shouldProbePrefixedTS("image/png") {
		t.Fatal("image/png")
	}
	if !shouldProbePrefixedTS("image/png; charset=binary") {
		t.Fatal("image/png with param")
	}
	if !shouldProbePrefixedTS("image/jpeg") {
		t.Fatal("image/jpeg")
	}
	if !shouldProbePrefixedTS("application/octet-stream") {
		t.Fatal("octet-stream")
	}
	if !shouldProbePrefixedTS("") {
		t.Fatal("empty")
	}
	if shouldProbePrefixedTS("video/mp2t") {
		t.Fatal("video/mp2t")
	}
}

func tsPackets(n int) []byte {
	var b bytes.Buffer
	pkt := make([]byte, tsPacketBytes)
	pkt[0] = 0x47
	for i := 1; i < len(pkt); i++ {
		pkt[i] = byte(i)
	}
	for i := 0; i < n; i++ {
		b.Write(pkt)
	}
	return b.Bytes()
}

func TestFindPrefixedTransportStreamOffset(t *testing.T) {
	if findPrefixedTransportStreamOffset(nil) >= 0 {
		t.Fatal("nil")
	}
	if findPrefixedTransportStreamOffset([]byte{0x89, 0x50}) >= 0 {
		t.Fatal("short junk")
	}
	pure := tsPackets(3)
	if findPrefixedTransportStreamOffset(pure) >= 0 {
		t.Fatal("clean ts at 0 must not strip")
	}

	prefix := []byte{0x00, 0x01, 0x02, 0xFF, 0xD8, 0xFF, 0xD9, 0xDE, 0xAD}
	wrapped := append(append([]byte{}, prefix...), tsPackets(3)...)
	got := findPrefixedTransportStreamOffset(wrapped)
	if got != len(prefix) {
		t.Fatalf("offset=%d want=%d", got, len(prefix))
	}
}

func TestMaybeStripPrefixedTS(t *testing.T) {
	prefix := []byte{0xFF, 0xD8, 0xFF, 0xD9, 0x11, 0x22}
	body := append(append([]byte{}, prefix...), tsPackets(3)...)
	fullLen := int64(len(body))

	out, ct, cl, ok := maybeStripPrefixedTS("image/png", fullLen, bytes.NewReader(body))
	if !ok {
		t.Fatal("expected strip")
	}
	if ct != "video/MP2T" {
		t.Fatalf("ct=%q", ct)
	}
	if cl != fullLen-int64(len(prefix)) {
		t.Fatalf("cl=%d want=%d", cl, fullLen-int64(len(prefix)))
	}
	got, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	want := body[len(prefix):]
	if !bytes.Equal(got, want) {
		t.Fatalf("body len=%d want=%d", len(got), len(want))
	}

	// 旧名入口。
	outAlias, _, _, okAlias := maybeStripImageWrappedTS("image/jpeg", fullLen, bytes.NewReader(body))
	if !okAlias {
		t.Fatal("alias")
	}
	gotAlias, _ := io.ReadAll(outAlias)
	if !bytes.Equal(gotAlias, want) {
		t.Fatal("alias body")
	}

	pureImg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	out2, _, _, ok2 := maybeStripPrefixedTS("image/jpeg", int64(len(pureImg)), bytes.NewReader(pureImg))
	if ok2 {
		t.Fatal("pure image should not strip")
	}
	got2, _ := io.ReadAll(out2)
	if !bytes.Equal(got2, pureImg) {
		t.Fatalf("pure body mismatch")
	}

	raw := tsPackets(2)
	out3, _, _, ok3 := maybeStripPrefixedTS("video/mp2t", int64(len(raw)), bytes.NewReader(raw))
	if ok3 {
		t.Fatal("clean video/mp2t")
	}
	got3, _ := io.ReadAll(out3)
	if !bytes.Equal(got3, raw) {
		t.Fatalf("passthrough mismatch")
	}

	// 非 image CT，但首字节非 0x47 且窗内有 TS → 仍剥。
	junk := append([]byte{0x01, 0x02, 0x03}, tsPackets(3)...)
	out4, _, _, ok4 := maybeStripPrefixedTS("application/octet-stream", int64(len(junk)), bytes.NewReader(junk))
	if !ok4 {
		t.Fatal("octet junk+ts")
	}
	got4, _ := io.ReadAll(out4)
	if !bytes.Equal(got4, junk[3:]) {
		t.Fatalf("octet strip body")
	}
}

func TestStrippedContentLength(t *testing.T) {
	if strippedContentLength(100, 10) != 90 {
		t.Fatal("100-10")
	}
	if strippedContentLength(10, 10) != -1 {
		t.Fatal("equal")
	}
	if strippedContentLength(-1, 5) != -1 {
		t.Fatal("unknown")
	}
}
