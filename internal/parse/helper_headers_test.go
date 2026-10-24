package parse

import "testing"

func TestPickPlayHeadersKeepsPlayKeysOnly(t *testing.T) {
	in := map[string]string{
		"ua":                "UA/1",
		"Cookie":            "a=b",
		"Authorization":     "Bearer tok",
		"Token":             "site-token",
		"X-Token":           "x",
		"referrer":          "https://ref.example/",
		"Origin":            "https://o.example",
		"Range":             "bytes=0-",
		"Host":              "cdn.example",
		"Connection":        "keep-alive",
		"Content-Length":    "1",
		"Transfer-Encoding": "chunked",
		"Accept-Encoding":   "gzip",
		" ":                 "skip-empty-key",
		"Empty":             "  ",
	}
	got := pickPlayHeaders(in)
	if got["User-Agent"] != "UA/1" || got["Cookie"] != "a=b" || got["Referer"] != "https://ref.example/" {
		t.Fatalf("canonical play headers: %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("only User-Agent, Referer, Cookie: %v", got)
	}
	if pickPlayHeaders(nil) != nil {
		t.Fatal("nil in → nil out")
	}
	if pickPlayHeaders(map[string]string{"Range": "bytes=0-", "Host": "cdn.example"}) != nil {
		t.Fatal("no play keys → nil out")
	}
}
