package clientsession

import "testing"

func TestNoteRepoSkipsBaseline(t *testing.T) {
	s := &Session{}
	s.SetRepoBaseline(map[string]struct{}{"http://host/vod": {}})
	s.NoteRepo("http://host/vod")
	s.NoteRepo("  http://remote/depot  ")
	s.NoteRepo("")
	got := s.AddedRepos()
	if _, ok := got["http://host/vod"]; ok {
		t.Fatal("baseline should not be added")
	}
	if _, ok := got["http://remote/depot"]; !ok {
		t.Fatalf("want remote depot, got %v", got)
	}
}

func TestNoteLiveSkipsBaseline(t *testing.T) {
	s := &Session{}
	s.SetLiveBaseline(map[string]struct{}{"http://host/live": {}})
	s.NoteLive("http://host/live")
	s.NoteLive("  http://remote/live  ")
	s.NoteLive("")
	got := s.AddedLives()
	if _, ok := got["http://host/live"]; ok {
		t.Fatal("baseline should not be added")
	}
	if _, ok := got["http://remote/live"]; !ok {
		t.Fatalf("want remote live, got %v", got)
	}
}

func TestURLsAddedBySessionKeepsHost(t *testing.T) {
	added := map[string]struct{}{
		"http://a": {},
		"http://b": {},
	}
	got := URLsAddedBySession(added, "http://a", map[string]struct{}{"http://b": {}})
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
	got = URLsAddedBySession(added, "http://host", nil)
	if len(got) != 2 {
		t.Fatalf("want 2, got %v", got)
	}
}
