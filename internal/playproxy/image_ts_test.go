package playproxy

import (
	"bytes"
	"io"
	"testing"
)

func TestShouldProbeImageWrap(t *testing.T) {
	if !shouldProbeImageWrap("image/png") {
		t.Fatal("image/png")
	}
	if !shouldProbeImageWrap("image/png; charset=binary") {
		t.Fatal("image/png with param")
	}
	if !shouldProbeImageWrap("image/jpeg") {
		t.Fatal("image/jpeg")
	}
	if !shouldProbeImageWrap("application/octet-stream") {
		t.Fatal("octet-stream")
	}
	if shouldProbeImageWrap("video/mp2t") {
		t.Fatal("video/mp2t")
	}
}

func TestFindPngWrappedTransportStreamOffset(t *testing.T) {
	if findPngWrappedTransportStreamOffset(nil) >= 0 {
		t.Fatal("nil")
	}
	if findPngWrappedTransportStreamOffset(pngSignature) >= 0 {
		t.Fatal("signature only")
	}

	var pure bytes.Buffer
	pure.Write(pngSignature)
	pure.Write(pngIEND)
	if findPngWrappedTransportStreamOffset(pure.Bytes()) >= 0 {
		t.Fatal("pure png without ts")
	}

	var wrapped bytes.Buffer
	wrapped.Write(pngSignature)
	wrapped.Write([]byte{0, 0, 0, 0})
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

func TestFindJpegWrappedTransportStreamOffset(t *testing.T) {
	if findJpegWrappedTransportStreamOffset(nil) >= 0 {
		t.Fatal("nil")
	}
	// 最小 JPEG：SOI + EOI，无 TS。
	pure := append(append([]byte{}, jpegSOI...), jpegEOI...)
	if findJpegWrappedTransportStreamOffset(pure) >= 0 {
		t.Fatal("pure jpeg")
	}

	var wrapped bytes.Buffer
	wrapped.Write(jpegSOI)
	wrapped.Write([]byte{0xFF, 0xE0, 0x00, 0x10}) // APP0 占位
	wrapped.Write(jpegEOI)
	tsOff := wrapped.Len()
	pkt := make([]byte, tsPacketBytes)
	pkt[0] = 0x47
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	got := findJpegWrappedTransportStreamOffset(wrapped.Bytes())
	if got != tsOff {
		t.Fatalf("offset=%d want=%d", got, tsOff)
	}
	// Content-Type 标 png、正文却是 JPEG 包 TS：总入口也能剥。
	if findImageWrappedTransportStreamOffset(wrapped.Bytes()) != tsOff {
		t.Fatal("image wrap jpeg")
	}
}

func TestMaybeStripImageWrappedTS(t *testing.T) {
	var wrapped bytes.Buffer
	wrapped.Write(jpegSOI)
	wrapped.Write(jpegEOI)
	tsOff := wrapped.Len()
	pkt := make([]byte, tsPacketBytes)
	pkt[0] = 0x47
	for i := 1; i < len(pkt); i++ {
		pkt[i] = byte(i)
	}
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	wrapped.Write(pkt)
	fullLen := int64(wrapped.Len())

	// CDN 常把 JPEG 包 TS 标成 image/png。
	out, ct, cl, ok := maybeStripImageWrappedTS("image/png", fullLen, bytes.NewReader(wrapped.Bytes()))
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

	pure := append(append([]byte{}, jpegSOI...), jpegEOI...)
	out2, _, _, ok2 := maybeStripImageWrappedTS("image/jpeg", int64(len(pure)), bytes.NewReader(pure))
	if ok2 {
		t.Fatal("pure jpeg should not strip")
	}
	got2, _ := io.ReadAll(out2)
	if !bytes.Equal(got2, pure) {
		t.Fatalf("pure body mismatch")
	}

	raw := []byte{0x47, 1, 2, 3}
	out3, _, _, ok3 := maybeStripImageWrappedTS("video/mp2t", int64(len(raw)), bytes.NewReader(raw))
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
