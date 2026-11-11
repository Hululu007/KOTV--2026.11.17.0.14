package playproxy

import (
	"bytes"
	"io"
	"testing"
)

func TestShouldProbePngPrefix(t *testing.T) {
	if !shouldProbePngPrefix("image/png") {
		t.Fatal("image/png")
	}
	if !shouldProbePngPrefix("image/png; charset=binary") {
		t.Fatal("image/png with param")
	}
	if !shouldProbePngPrefix("image/jpeg") {
		t.Fatal("image/jpeg")
	}
	if shouldProbePngPrefix("video/mp2t") {
		t.Fatal("video/mp2t")
	}
	if shouldProbePngPrefix("") {
		t.Fatal("empty")
	}
}

func TestFindPngWrappedTransportStreamOffset(t *testing.T) {
	if findPngWrappedTransportStreamOffset(nil) >= 0 {
		t.Fatal("nil")
	}
	if findPngWrappedTransportStreamOffset(pngSignature) >= 0 {
		t.Fatal("signature only")
	}

	// 最小合法空 PNG：签名 + IHDR 省略，直接拼 IEND 类型+CRC（探测只认签名与 IEND 标记）。
	var pure bytes.Buffer
	pure.Write(pngSignature)
	pure.Write(pngIEND)
	if findPngWrappedTransportStreamOffset(pure.Bytes()) >= 0 {
		t.Fatal("pure png without ts")
	}

	var wrapped bytes.Buffer
	wrapped.Write(pngSignature)
	wrapped.Write([]byte{0, 0, 0, 0}) // 占位
	wrapped.Write(pngIEND)
	tsOff := wrapped.Len()
	pkt := bytes.Repeat([]byte{0x47}, tsPacketBytes)
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	got := findPngWrappedTransportStreamOffset(wrapped.Bytes())
	if got != tsOff {
		t.Fatalf("offset=%d want=%d", got, tsOff)
	}
}

func TestMaybeStripPngWrappedTS(t *testing.T) {
	var wrapped bytes.Buffer
	wrapped.Write(pngSignature)
	wrapped.Write(pngIEND)
	tsOff := wrapped.Len()
	pkt := make([]byte, tsPacketBytes)
	pkt[0] = 0x47
	for i := 1; i < len(pkt); i++ {
		pkt[i] = byte(i)
	}
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	tail := []byte("TAIL")
	wrapped.Write(tail)
	fullLen := int64(wrapped.Len())

	out, ct, cl, ok := maybeStripPngWrappedTS("image/png", fullLen, bytes.NewReader(wrapped.Bytes()))
	if !ok {
		t.Fatal("expected strip")
	}
	if ct != "video/MP2T" {
		t.Fatalf("ct=%q", ct)
	}
	if cl != fullLen-int64(tsOff) {
		t.Fatalf("cl=%d want=%d", cl, fullLen-int64(tsOff))
	}
	got, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	want := wrapped.Bytes()[tsOff:]
	if !bytes.Equal(got, want) {
		t.Fatalf("body len=%d want=%d", len(got), len(want))
	}

	// 纯 PNG：不剥，正文完整。
	pure := append(append([]byte{}, pngSignature...), pngIEND...)
	out2, _, _, ok2 := maybeStripPngWrappedTS("image/png", int64(len(pure)), bytes.NewReader(pure))
	if ok2 {
		t.Fatal("pure png should not strip")
	}
	got2, _ := io.ReadAll(out2)
	if !bytes.Equal(got2, pure) {
		t.Fatalf("pure body mismatch")
	}

	// 非 image：不探测。
	raw := []byte{0x47, 1, 2, 3}
	out3, _, _, ok3 := maybeStripPngWrappedTS("video/mp2t", int64(len(raw)), bytes.NewReader(raw))
	if ok3 {
		t.Fatal("non-image")
	}
	got3, _ := io.ReadAll(out3)
	if !bytes.Equal(got3, raw) {
		t.Fatalf("passthrough mismatch")
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
