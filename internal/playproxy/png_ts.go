package playproxy

import (
	"bytes"
	"io"
	"strings"
)

// PNG 容器包 TS：部分 HLS 分片 Content-Type 为 image/png，正文是 PNG 头 + IEND 后挂 MPEG-TS。
// 剥掉前缀再标 video/MP2T，避免 lavf 认成 png 静帧假片尾。

var (
	pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// IEND 类型四字节 + 空 IEND 块标准 CRC。
	pngIEND = []byte{0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}
)

const (
	tsPacketBytes   = 188
	pngPrefixScanMax = 64 << 10
)

func shouldProbePngPrefix(contentType string) bool {
	mime := strings.TrimSpace(strings.ToLower(contentType))
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if strings.HasPrefix(mime, "image/png") {
		return true
	}
	return strings.HasPrefix(mime, "image/")
}

func startsWithPNGSignature(data []byte) bool {
	if len(data) < len(pngSignature) {
		return false
	}
	return bytes.Equal(data[:len(pngSignature)], pngSignature)
}

func indexOfBytes(data, needle []byte) int {
	if len(needle) == 0 || len(data) < len(needle) {
		return -1
	}
	return bytes.Index(data, needle)
}

// findPngWrappedTransportStreamOffset 若 data 以 PNG 开头且 IEND 后可见连续 TS sync(0x47)，返回 TS 起点；否则 -1。
func findPngWrappedTransportStreamOffset(data []byte) int {
	if !startsWithPNGSignature(data) {
		return -1
	}
	iend := indexOfBytes(data, pngIEND)
	if iend < 0 {
		return -1
	}
	start := iend + len(pngIEND)
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

func strippedContentLength(contentLength int64, strippedPrefixBytes int) int64 {
	if contentLength <= 0 || strippedPrefixBytes <= 0 || int64(strippedPrefixBytes) >= contentLength {
		return -1
	}
	return contentLength - int64(strippedPrefixBytes)
}

// maybeStripPngWrappedTS 在 image/* 响应上探测 PNG 包 TS；命中则剥前缀并建议新 Content-Type / Content-Length。
// 未命中时仍返回可读的完整 body（已读前缀拼回）。
func maybeStripPngWrappedTS(contentType string, contentLength int64, body io.Reader) (out io.Reader, newCT string, newCL int64, stripped bool) {
	if body == nil || !shouldProbePngPrefix(contentType) {
		return body, "", -1, false
	}
	buf := make([]byte, pngPrefixScanMax)
	n, err := io.ReadFull(body, buf)
	if n == 0 && err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return body, "", -1, false
	}
	prefix := buf[:n]
	off := findPngWrappedTransportStreamOffset(prefix)
	if off < 0 {
		return io.MultiReader(bytes.NewReader(prefix), body), "", -1, false
	}
	rest := prefix[off:]
	return io.MultiReader(bytes.NewReader(rest), body), "video/MP2T", strippedContentLength(contentLength, off), true
}
