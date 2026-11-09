package service

import (
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestNodePushSpider(t *testing.T) {
	cases := []struct {
		api  string
		typ  int
		want bool
	}{
		{"/spider/push/4", 4, true},
		{"/spider/soaiquanpan/3", 3, false},
		{"csp_PushAgent", 3, false},
		{"http://example.com/api", 4, false},
		{"/spider/push/4", 3, false},
	}
	for _, c := range cases {
		site := model.Site{
			API:  c.api,
			Type: model.FlexInt{Valid: true, Value: c.typ},
		}
		if got := nodePushSpider(site); got != c.want {
			t.Fatalf("api=%s type=%d got=%v want=%v", c.api, c.typ, got, c.want)
		}
	}
}
