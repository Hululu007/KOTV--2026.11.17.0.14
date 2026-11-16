package clientsession

import "testing"

func TestNoteLiveSkipsBaseline(t *testing.T) {
	s := &Session{}
	s.SetLiveBaseline(map[string]struct{}{"http://host/live": {}})
	s.NoteLive("http://host/live")
	s.NoteLive("  http://remote/live  ")
	s.NoteLive("")
	got := s.AddedLives()
	if _, ok := got["http://host/live"]; ok {
		t.Fatal("baseline url was recorded as added")
	}
	if _, ok := got["http://remote/live"]; !ok {
		t.Fatal("new url was not recorded")
	}
	if len(got) != 1 {
		t.Fatalf("added = %v", got)
	}
}
