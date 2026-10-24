package parse

import (
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

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

func TestDropRangeHeaderKeepsTheRest(t *testing.T) {
	got := dropRangeHeader(map[string]string{
		"Range":          "bytes=0-",
		"Content-Length": "24540",
		"Host":           "cdn.example",
		"Cookie":         "a=b",
		"Connection":     "keep-alive",
	})
	if _, ok := got["Range"]; ok {
		t.Fatalf("Range must go: %v", got)
	}
	if got["Content-Length"] != "24540" || got["Host"] != "cdn.example" || got["Cookie"] != "a=b" || got["Connection"] != "keep-alive" {
		t.Fatalf("other keys stay: %v", got)
	}
	if dropRangeHeader(map[string]string{"Range": "bytes=0-"}) != nil {
		t.Fatal("only Range → nil")
	}
}

func TestJSONParseHeaderReplacesSiteHeader(t *testing.T) {
	r := model.Result{Header: model.FlexHeader{
		"Host":    "www.ncat3.app",
		"Referer": "https://www.ncat3.app/",
		"Cookie":  "site=1",
	}}
	p := &model.Parse{Type: model.FlexInt{Valid: true, Value: 1}}
	applyParsedHeaders(&r, p, map[string]string{"ua": "UA/1", "Range": "bytes=0-"}, false)
	if r.Header["User-Agent"] != "UA/1" {
		t.Fatalf("json ua: %v", r.Header)
	}
	if _, ok := r.Header["Host"]; ok {
		t.Fatalf("site host must not stay: %v", r.Header)
	}
	if _, ok := r.Header["Range"]; ok {
		t.Fatalf("range must go: %v", r.Header)
	}
	if _, ok := r.Header["Cookie"]; ok {
		t.Fatalf("site cookie must not stay: %v", r.Header)
	}

	r.Header = model.FlexHeader{"Host": "www.ncat3.app"}
	p.Ext = `{"header":{"Referer":"https://parse.example/"}}`
	applyParsedHeaders(&r, p, nil, false)
	if r.Header["Referer"] != "https://parse.example/" || len(r.Header) != 1 {
		t.Fatalf("empty json uses parse header: %v", r.Header)
	}

	r.Header = model.FlexHeader{"Host": "www.ncat3.app", "Range": "bytes=0-"}
	p.Ext = ""
	applyParsedHeaders(&r, p, nil, false)
	if r.Header["Host"] != "www.ncat3.app" || len(r.Header) != 1 {
		t.Fatalf("empty json keeps the header already on the result: %v", r.Header)
	}
}

func TestWebSniffHeaderReplacesSiteHeader(t *testing.T) {
	r := model.Result{Header: model.FlexHeader{"Host": "www.ncat3.app", "Cookie": "site=1"}}
	p := &model.Parse{Type: model.FlexInt{Valid: true, Value: 0}}
	applyParsedHeaders(&r, p, map[string]string{
		"User-Agent":     "Web/1",
		"Referer":        "https://player.example/",
		"Origin":         "https://player.example",
		"Range":          "bytes=0-",
		"Content-Length": "24540",
		"Content-Type":   "application/x-mpeg",
	}, false)
	if r.Header["User-Agent"] != "Web/1" || r.Header["Origin"] != "https://player.example" {
		t.Fatalf("web request headers: %v", r.Header)
	}
	if _, ok := r.Header["Host"]; ok {
		t.Fatalf("site host must not stay: %v", r.Header)
	}
	if _, ok := r.Header["Range"]; ok {
		t.Fatalf("range must go: %v", r.Header)
	}
	if r.Header["Content-Length"] != "24540" || r.Header["Content-Type"] != "application/x-mpeg" {
		t.Fatalf("request headers stay: %v", r.Header)
	}
}
