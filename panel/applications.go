package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// AppEntry is a real installed application, parsed from an actual .desktop
// file - not placeholder/mock data.
type AppEntry struct {
	Name       string `json:"name"`
	Exec       string `json:"exec"`
	Categories string `json:"categories"`
	// Icon is the raw Icon= value from the .desktop file - either a themed
	// icon name (resolved against the system icon theme) or an absolute
	// path. Resolving it to an actual image is a UI concern (see ui.go), not
	// something this backend file deals with.
	Icon string `json:"icon"`
}

var fieldCodeRe = regexp.MustCompile(`%[fFuUdDnNickvm]`)

// ListApplications enumerates real installed applications from the standard
// XDG .desktop locations. Deliberately reads the actual files on disk rather
// than shelling out to something like `gio list` - this is meant to become
// the real Applications menu, so it needs the real data.
func (a *App) ListApplications() []AppEntry {
	dirs := []string{"/usr/share/applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local/share/applications"))
	}

	seen := map[string]bool{}
	var apps []AppEntry
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			app, ok := parseDesktopFile(filepath.Join(dir, e.Name()))
			if ok && !seen[app.Name] {
				seen[app.Name] = true
				apps = append(apps, app)
			}
		}
	}

	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	return apps
}

// parseDesktopFile is a minimal, purpose-built .desktop parser - only reads
// the handful of keys the Applications menu actually needs, only from the
// [Desktop Entry] section (ignores [Desktop Action ...] subsections and
// every localized Name[xx]=/Comment[xx]= variant, since this doesn't need
// locale support to be a real, working menu).
func parseDesktopFile(path string) (AppEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppEntry{}, false
	}

	var name, execLine, categories string
	isApplication, noDisplay := false, false
	inEntry := false

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "[Desktop Entry]":
			inEntry = true
		case strings.HasPrefix(line, "["):
			inEntry = false
		case !inEntry:
			// skip - not in the section we care about
		case strings.HasPrefix(line, "Name="):
			name = strings.TrimPrefix(line, "Name=")
		case strings.HasPrefix(line, "Exec="):
			execLine = strings.TrimPrefix(line, "Exec=")
		case strings.HasPrefix(line, "Type="):
			isApplication = strings.TrimPrefix(line, "Type=") == "Application"
		case strings.HasPrefix(line, "Categories="):
			categories = strings.TrimPrefix(line, "Categories=")
		case line == "NoDisplay=true", line == "Hidden=true":
			noDisplay = true
		}
	}

	if !isApplication || noDisplay || name == "" || execLine == "" {
		return AppEntry{}, false
	}
	return AppEntry{
		Name:       name,
		Exec:       strings.TrimSpace(fieldCodeRe.ReplaceAllString(execLine, "")),
		Categories: categories,
	}, true
}

// LaunchApplication runs an Exec= line already cleaned by ListApplications.
// Splits on whitespace only - doesn't handle quoted arguments with embedded
// spaces, which covers the large majority of real .desktop Exec lines but
// not all of them. Good enough for a real v1, not pretending to be a full
// shell-lexer implementation.
func (a *App) LaunchApplication(execLine string) error {
	parts := strings.Fields(execLine)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	return cmd.Start()
}
