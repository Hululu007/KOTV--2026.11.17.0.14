package model

import "testing"

func TestClassifyPlayPrefix(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		ct   string
		want string
	}{
		{"m3u8", []byte("#EXTM3U\n#EXTINF:1,\nseg.ts\n"), "", FormatHLS},
		{"bom", append([]byte{0xEF, 0xBB, 0xBF}, []byte("#EXTM3U\n")...), "", FormatHLS},
		{"ws", []byte("\n\t#EXTM3U\n"), "", FormatHLS},
		{"ct_hls", []byte("#EXT-X-VERSION:3\n"), "application/vnd.apple.mpegurl", FormatHLS},
		{"ct_hls_empty_body", nil, "application/x-mpegURL", FormatHLS},
		{"flv", []byte("FLV\x01\x01"), "", ""},
		{"flv_ct_ignored", []byte("FLV\x01"), "application/x-mpegURL", ""},
		{"ftyp", []byte("\x00\x00\x00\x18ftypisom"), "", ""},
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, "image/png", ""},
		{"dash_xml", []byte(`<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011">`), "", FormatDASH},
		{"dash_ct", []byte("<Period>"), "application/dash+xml", FormatDASH},
		{"empty", nil, "", ""},
	}
	for _, tc := range cases {
		if got := ClassifyPlayPrefix(tc.head, tc.ct); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestFormatIsHlsDash(t *testing.T) {
	if !FormatIsHls("application/x-mpegURL") || !FormatIsHls("APPLICATION/VND.APPLE.MPEGURL") {
		t.Fatal("hls mime")
	}
	if !FormatIsHls("hls") || !FormatIsHls("m3u8") {
		t.Fatal("hls literal")
	}
	if FormatIsHls("video/mp4") {
		t.Fatal("not hls")
	}
	if !FormatIsDash("application/dash+xml") || !FormatIsDash("mpd") {
		t.Fatal("dash")
	}
}
