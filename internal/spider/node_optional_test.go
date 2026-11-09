package spider

import (
	"errors"
	"fmt"
	"testing"
)

func TestNodeRouteMissing(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("network down"), false},
		{fmt.Errorf("POST /spider/push/4/home http 404: {\"statusCode\":404}"), true},
		{fmt.Errorf("POST /spider/x/3/search http 404: not found"), true},
		{fmt.Errorf("POST /spider/x/3/home http 500: boom"), false},
	}
	for _, c := range cases {
		if got := nodeRouteMissing(c.err); got != c.want {
			t.Fatalf("nodeRouteMissing(%v)=%v want %v", c.err, got, c.want)
		}
	}
}

func TestPickHomeVideoRaw(t *testing.T) {
	miss := fmt.Errorf("POST /spider/x/3/homeVod http 404: missing")
	cases := []struct {
		name     string
		vodRaw   string
		vodErr   error
		videoRaw string
		videoErr error
		want     string
	}{
		{"vod ok", `{"list":[1]}`, nil, "", nil, `{"list":[1]}`},
		{"vod empty body", "  ", nil, `{"list":[2]}`, nil, "{}"},
		{"vod missing use video", "", miss, `{"list":[2]}`, nil, `{"list":[2]}`},
		{"both missing", "", miss, "", miss, "{}"},
		{"vod other err soft empty", "", errors.New("boom"), `{"list":[2]}`, nil, "{}"},
	}
	for _, c := range cases {
		got, err := pickHomeVideoRaw(c.vodRaw, c.vodErr, c.videoRaw, c.videoErr)
		if err != nil {
			t.Fatalf("%s: unexpected err %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
