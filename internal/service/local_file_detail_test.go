package service

import (
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestLocalFileDetailKeepsHashInPath(t *testing.T) {
	s := &SiteService{}
	id := filepath.Join(t.TempDir(), "show#1.$ep.mp4")
	vod := model.Vod{VodID: model.FlexString(id), Site: &model.Site{}}
	detail, err := s.DetailContent(LocalFileKey, vod)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.VodFlags) != 1 || len(detail.VodFlags[0].Episodes) != 1 {
		t.Fatalf("flags=%+v", detail.VodFlags)
	}
	got := detail.VodFlags[0].Episodes[0].URL
	if got != id {
		t.Fatalf("episode url=%q want %q", got, id)
	}
}
