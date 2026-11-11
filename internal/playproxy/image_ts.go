package playproxy

import (
	"bytes"
	"io"
	"strings"
)

// 部分 HLS 分片 Content-Type 标成 image/*，正文却是「图片头 + MPEG-TS」。
// 常见：PNG（IEND 后接 TS）、JPEG（SOI…EOI 后接 TS）。剥前缀再标 video/MP2T，
// 避免 lavf 认成静帧图轨并立刻 EOF。

var (
	pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// IEND 类型四字节 + 空 IEND 块标准 CRC。
	pngIEND     = []byte{0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}
	jpegSOI     = []byte{0xFF, 0xD8}
	jpegEOI     = []byte{0xFF, 0xD9}
)

const (
	tsPacketBytes    = 188
	imagePrefixScanMax = 64 << 10
)

func shouldProbeImageWrap(contentType string) bool {
	mime := strings.TrimSpace(strings.ToLower(contentType))
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if strings.HasPrefix(mime, "image/") {
		return true
	}
	// CDN 偶发标成 octet-stream，仍按魔数探测。
	return mime == "application/octet-stream" || mime == ""
}

func startsWithPNGSignature(data []byte) bool {
	if len(data) < len(pngSignature) {
		return false
	}
	return bytes.Equal(data[:len(pngSignature)], pngSignature)
}

func startsWithJPEGSignature(data []byte) bool {
	return len(data) >= 2 && data[0] == jpegSOI[0] && data[1] == jpegSOI[1]
}

func indexOfBytes(data, needle []byte) int {
	if len(needle) == 0 || len(data) < len(needle) {
		return -1
	}
	return bytes.Index(data, needle)
}

// tsSyncAfter 从 start 起找连续 TS sync(0x47 @ 188 字节间隔)；需至少两包可见。
func tsSyncAfter(data []byte, start int) int {
	for offset := start; offset+tsPacketBytes < len(data); offset++ {
		if data[offset] != 0x47 || data[offset+tsPacketBytes] != 0x47 {
			continue
		}
		if offset+tsPacketBytes*2 < len(data) && data[offset+tsPacketBytes*2] != 0x47 {
			continue
		}
		return offset
	}
	return -1
}

// findImageWrappedTransportStreamOffset PNG/JPEG 头后挂 TS 时返回 TS 起点，否则 -1。
func findImageWrappedTransportStreamOffset(data []byte) int {
	if off := findPngWrappedTransportStreamOffset(data); off >= 0 {
		return off
	}
	return findJpegWrappedTransportStreamOffset(data)
}

func findPngWrappedTransportStreamOffset(data []byte) int {
	if !startsWithPNGSignature(data) {
		return -1
	}
	iend := indexOfBytes(data, pngIEND)
	if iend < 0 {
		return -1
	}
	return tsSyncAfter(data, iend+len(pngIEND))
}

func findJpegWrappedTransportStreamOffset(data []byte) int {
	if !startsWithJPEGSignature(data) {
		return -1
	}
	eoi := indexOfBytes(data, jpegEOI)
	if eoi < 0 {
		return -1
	}
	return tsSyncAfter(data, eoi+len(jpegEOI))
}

func strippedContentLength(contentLength int64, strippedPrefixBytes int) int64 {
	if contentLength <= 0 || strippedPrefixBytes <= 0 || int64(strippedPrefixBytes) >= contentLength {
		return -1
	}
	return contentLength - int64(strippedPrefixBytes)
}

// maybeStripImageWrappedTS 在 image/*（及 octet-stream）响应上探测图片头包 TS；
// 命中则剥前缀并建议新 Content-Type / Content-Length。未命中仍返回完整可读 body。
func maybeStripImageWrappedTS(contentType string, contentLength int64, body io.Reader) (out io.Reader, newCT string, newCL int64, stripped bool) {
	if body == nil || !shouldProbeImageWrap(contentType) {
		return body, "", -1, false
	}
	buf := make([]byte, imagePrefixScanMax)
	n, err := io.ReadFull(body, buf)
	if n == 0 && err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return body, "", -1, false
	}
	prefix := buf[:n]
	off := findImageWrappedTransportStreamOffset(prefix)
	if off < 0 {
		return io.MultiReader(bytes.NewReader(prefix), body), "", -1, false
	}
	rest := prefix[off:]
	return io.MultiReader(bytes.NewReader(rest), body), "video/MP2T", strippedContentLength(contentLength, off), true
}
