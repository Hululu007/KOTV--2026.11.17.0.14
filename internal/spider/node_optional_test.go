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
