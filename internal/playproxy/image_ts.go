package playproxy

import (
	"bytes"
	"io"
	"strings"
)

// 部分 HLS 分片 Content-Type 标成 image/* 或带任意前缀，正文实际是「前缀 + MPEG-TS」。
// 在扫描窗内找连续 TS sync(0x47 @ 188)，offset>0 时剥前缀并标 video/MP2T，
// 避免 lavf 认成静帧图轨并立刻 EOF。不认某种图片壳、不定点偏移。

const (
	tsPacketBytes      = 188
	imagePrefixScanMax = 64 << 10
)

func shouldProbePrefixedTS(contentType string) bool {
	mime := strings.TrimSpace(strings.ToLower(contentType))
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if strings.HasPrefix(mime, "image/") {
		return true
	}
	// CDN 偶发标成 octet-stream / 空类型，仍按正文探测。
	return mime == "application/octet-stream" || mime == ""
}

// tsSyncAt 从 start 起找连续 TS sync(0x47 @ 188 字节间隔)；需至少两包可见，
// 若第三包也在窗内则一并校验。
func tsSyncAt(data []byte, start int) int {
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

// findPrefixedTransportStreamOffset 任意前缀后挂 TS 时返回 TS 起点；已是干净 TS 或找不到为 -1。
func findPrefixedTransportStreamOffset(data []byte) int {
	if len(data) == 0 {
		return -1
	}
	off := tsSyncAt(data, 0)
	if off <= 0 {
		return -1
	}
	return off
}

func strippedContentLength(contentLength int64, strippedPrefixBytes int) int64 {
	if contentLength <= 0 || strippedPrefixBytes <= 0 || int64(strippedPrefixBytes) >= contentLength {
		return -1
	}
	return contentLength - int64(strippedPrefixBytes)
}

// maybeStripPrefixedTS 在 image/*（及 octet-stream/空 CT）或「首字节非 TS sync」的响应上
// 探测任意前缀包 TS；命中则剥前缀并建议新 Content-Type / Content-Length。
// 未命中仍返回完整可读 body。
func maybeStripPrefixedTS(contentType string, contentLength int64, body io.Reader) (out io.Reader, newCT string, newCL int64, stripped bool) {
	if body == nil {
		return body, "", -1, false
	}
	probe := shouldProbePrefixedTS(contentType)
	buf := make([]byte, imagePrefixScanMax)
	n, err := io.ReadFull(body, buf)
	if n == 0 && err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return body, "", -1, false
	}
	prefix := buf[:n]
	// 非 image/octet 时：仅当开头已不是 TS sync 才继续找（避免误伤干净 TS/MP4）。
	if !probe {
		if len(prefix) == 0 || prefix[0] == 0x47 {
			return io.MultiReader(bytes.NewReader(prefix), body), "", -1, false
		}
	}
	off := findPrefixedTransportStreamOffset(prefix)
	if off < 0 {
		return io.MultiReader(bytes.NewReader(prefix), body), "", -1, false
	}
	rest := prefix[off:]
	return io.MultiReader(bytes.NewReader(rest), body), "video/MP2T", strippedContentLength(contentLength, off), true
}

// maybeStripImageWrappedTS 兼容旧名，语义同 [maybeStripPrefixedTS]。
func maybeStripImageWrappedTS(contentType string, contentLength int64, body io.Reader) (out io.Reader, newCT string, newCL int64, stripped bool) {
	return maybeStripPrefixedTS(contentType, contentLength, body)
}
