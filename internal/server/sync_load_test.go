package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobo/KOTV/internal/settings"
)

func TestSyncLoadPlan(t *testing.T) {
	const cur = "http://current"
	cases := []struct {
		name      string
		typ       string
		current   string
		hasConfig bool
		config    string
		configs   string
		wantURL   string
		wantName  string
		wantLoad  bool
		wantSkip  bool
	}{
		{name: "history no config", typ: "history", current: cur, hasConfig: false, wantLoad: false, wantSkip: false},
		{name: "history empty url", typ: "history", current: cur, hasConfig: true, config: `{"type":0,"url":""}`, wantSkip: true},
		{name: "history bad json", typ: "history", current: cur, hasConfig: true, config: `{`, wantSkip: true},
		{name: "history different", typ: "history", current: cur, hasConfig: true, config: `{"url":"file:///tmp/a.json","name":"全部"}`, wantURL: "file:///tmp/a.json", wantName: "全部", wantLoad: true},
		{name: "history same", typ: "history", current: cur, hasConfig: true, config: `{"url":"http://current","name":"n"}`, wantURL: "http://current", wantName: "n"},
		{name: "keep when current set", typ: "keep", current: cur, configs: `[{"url":"file:///tmp/a.json","name":"k"}]`},
		{name: "keep when current empty", typ: "keep", current: "", configs: `[{"url":"file:///tmp/a.json","name":"k"}]`, wantURL: "file:///tmp/a.json", wantName: "k", wantLoad: true},
		{name: "keep empty configs", typ: "keep", current: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotName, load, skip := syncLoadPlan(c.typ, c.current, c.hasConfig, c.config, c.configs)
			if gotURL != c.wantURL || gotName != c.wantName || load != c.wantLoad || skip != c.wantSkip {
				t.Fatalf("got url=%q name=%q load=%v skip=%v", gotURL, gotName, load, skip)
			}
		})
	}
}

func TestSyncHistoryLoadsBeforeImport(t *testing.T) {
	prev := settings.Get(settings.VOD)
	t.Cleanup(func() { settings.Set(settings.VOD, prev) })
	settings.Set(settings.VOD, "http://current")

	release := make(chan struct{})
	loaded := make(chan struct{})
	imported := make(chan struct{})
	var mu sync.Mutex
	var order []string
	s := New(nil)
	s.SetSyncHandler(&SyncHandler{
		LoadConfig: func(url, name string) error {
			if url != "file:///tmp/a.json" || name != "全部" {
				t.Errorf("load %q %q", url, name)
			}
			mu.Lock()
			order = append(order, "load")
			mu.Unlock()
			close(loaded)
			<-release
			return nil
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			mu.Lock()
			order = append(order, "import")
			mu.Unlock()
			close(imported)
			return nil
		},
	})

	form := url.Values{}
	form.Set("config", `{"type":0,"url":"file:///tmp/a.json","name":"全部"}`)
	form.Set("targets", "[]")
	form.Set("force", "false")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/action?do=sync&mode=1&type=history", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		s.handleAction(rr, req)
		done <- rr
	}()

	select {
	case rr := <-done:
		if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != "OK" {
			t.Fatalf("status %d body %q", rr.Code, rr.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response waited for load")
	}
	select {
	case <-loaded:
	case <-time.After(2 * time.Second):
		t.Fatal("load not started")
	}
	select {
	case <-imported:
		t.Fatal("imported before load finished")
	default:
	}
	close(release)
	select {
	case <-imported:
	case <-time.After(2 * time.Second):
		t.Fatal("import after load")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "load" || order[1] != "import" {
		t.Fatalf("order %v", order)
	}
}

func TestSyncHistorySameURLImportsWithoutLoad(t *testing.T) {
	prev := settings.Get(settings.VOD)
	t.Cleanup(func() { settings.Set(settings.VOD, prev) })
	settings.Set(settings.VOD, "file:///tmp/a.json")

	var loaded, imported bool
	s := New(nil)
	s.SetSyncHandler(&SyncHandler{
		LoadConfig: func(url, name string) error {
			loaded = true
			return nil
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			imported = true
			if targetsJSON != `[{"key":"a"}]` {
				t.Fatalf("targets %s", targetsJSON)
			}
			return nil
		},
	})
	form := url.Values{}
	form.Set("config", `{"url":"file:///tmp/a.json","name":"n"}`)
	form.Set("targets", `[{"key":"a"}]`)
	req := httptest.NewRequest(http.MethodPost, "/action?do=sync&mode=1&type=history", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleAction(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if loaded || !imported {
		t.Fatalf("loaded=%v imported=%v", loaded, imported)
	}
}

func TestSyncHistoryEmptyURLSkipsImport(t *testing.T) {
	prev := settings.Get(settings.VOD)
	t.Cleanup(func() { settings.Set(settings.VOD, prev) })
	settings.Set(settings.VOD, "http://current")

	called := false
	s := New(nil)
	s.SetSyncHandler(&SyncHandler{
		LoadConfig: func(url, name string) error {
			called = true
			return nil
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			called = true
			return nil
		},
	})
	form := url.Values{}
	form.Set("config", `{"type":0,"url":""}`)
	form.Set("targets", `[{"key":"a"}]`)
	req := httptest.NewRequest(http.MethodPost, "/action?do=sync&mode=1&type=history", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleAction(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if called {
		t.Fatal("empty url should skip load and import")
	}
}

func TestSyncHistoryTargetsOnly(t *testing.T) {
	prev := settings.Get(settings.VOD)
	t.Cleanup(func() { settings.Set(settings.VOD, prev) })
	settings.Set(settings.VOD, "http://current")

	var loaded, imported bool
	s := New(nil)
	s.SetSyncHandler(&SyncHandler{
		LoadConfig: func(url, name string) error {
			loaded = true
			return nil
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			imported = true
			return nil
		},
	})
	form := url.Values{}
	form.Set("targets", `[{"key":"a"}]`)
	req := httptest.NewRequest(http.MethodPost, "/action?do=sync&mode=1&type=history", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleAction(rr, req)
	if rr.Code != http.StatusOK || loaded || !imported {
		t.Fatalf("status %d loaded=%v imported=%v", rr.Code, loaded, imported)
	}
}

func TestSyncLoadFailureSkipsImport(t *testing.T) {
	prev := settings.Get(settings.VOD)
	t.Cleanup(func() { settings.Set(settings.VOD, prev) })
	settings.Set(settings.VOD, "http://current")

	imported := make(chan struct{}, 1)
	s := New(nil)
	s.SetSyncHandler(&SyncHandler{
		LoadConfig: func(url, name string) error {
			return errSyncLoad
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			imported <- struct{}{}
			return nil
		},
	})
	form := url.Values{}
	form.Set("config", `{"url":"file:///tmp/a.json"}`)
	form.Set("targets", `[{"key":"a"}]`)
	req := httptest.NewRequest(http.MethodPost, "/action?do=sync&mode=1&type=history", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleAction(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	select {
	case <-imported:
		t.Fatal("imported after load error")
	case <-time.After(200 * time.Millisecond):
	}
}

var errSyncLoad = errString("load failed")

type errString string

func (e errString) Error() string { return string(e) }
