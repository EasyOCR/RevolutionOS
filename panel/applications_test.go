package main

import (
	"strings"
	"testing"
)

// Real test against this machine's actual /usr/share/applications - not
// mocked. Confirms the .desktop parser produces sane real data, independent
// of any Wails/X11 runtime (this is pure file I/O).
func TestListApplications(t *testing.T) {
	apps := (&App{}).ListApplications()
	if len(apps) == 0 {
		t.Fatal("expected at least one real application, got none - check /usr/share/applications is readable")
	}
	for _, app := range apps {
		if app.Name == "" {
			t.Errorf("app with empty Name: %+v", app)
		}
		if app.Exec == "" {
			t.Errorf("app %q has empty Exec", app.Name)
		}
	}
	t.Logf("found %d real applications, e.g. first 5:", len(apps))
	for i, app := range apps {
		if i >= 5 {
			break
		}
		t.Logf("  %q -> exec=%q categories=%q", app.Name, app.Exec, app.Categories)
	}
}

func TestCleanExecStripsFieldCodes(t *testing.T) {
	cases := map[string]string{
		"firefox %u":         "firefox",
		"code --new-window %F": "code --new-window",
		"vlc %U":              "vlc",
		"plain-cmd":           "plain-cmd",
	}
	for input, want := range cases {
		got := strings.TrimSpace(fieldCodeRe.ReplaceAllString(input, ""))
		if got != want {
			t.Errorf("cleaning %q: got %q, want %q", input, got, want)
		}
	}
}
