package database

import (
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/model"
)

func TestSiteDisabledSurvivesResync(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "site.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const cfgID = int64(7)
	sites := []model.Site{{Key: "a", Name: "A"}, {Key: "b", Name: "B"}}
	if err := db.SyncSites(cfgID, sites); err != nil {
		t.Fatal(err)
	}
	sites[0].Disabled = true
	if err := db.UpdateSiteFlags(sites[0]); err != nil {
		t.Fatal(err)
	}
	b := model.Site{Key: "b", Disabled: true}
	if err := db.UpdateSiteFlagsByKey(cfgID, b); err != nil {
		t.Fatal(err)
	}

	again := []model.Site{{Key: "a", Name: "A"}, {Key: "b", Name: "B"}, {Key: "c", Name: "C"}}
	if err := db.SyncSites(cfgID, again); err != nil {
		t.Fatal(err)
	}
	if !again[0].Disabled || !again[1].Disabled || again[2].Disabled {
		t.Fatalf("disabled after resync = %v %v %v, want true true false",
			again[0].Disabled, again[1].Disabled, again[2].Disabled)
	}
}
