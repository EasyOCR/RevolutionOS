package main

import (
	"os"
	"testing"
)

// Real test against this machine's actual home directory - not mocked.
func TestListPlaces(t *testing.T) {
	places := (&App{}).ListPlaces()
	if len(places) == 0 {
		t.Fatal("expected at least Home, got no places at all")
	}
	home, _ := os.UserHomeDir()
	if places[0].Name != "Home" || places[0].Path != home {
		t.Errorf("expected first place to be Home=%q, got %+v", home, places[0])
	}
	for _, p := range places {
		if info, err := os.Stat(p.Path); err != nil || !info.IsDir() {
			t.Errorf("place %q -> %q does not actually exist as a directory", p.Name, p.Path)
		}
	}
	t.Logf("found %d real places:", len(places))
	for _, p := range places {
		t.Logf("  %q -> %q", p.Name, p.Path)
	}
}
