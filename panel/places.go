package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PlaceEntry is a real filesystem location - not placeholder data.
type PlaceEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// standard XDG user-dirs keys, in the order a Places menu conventionally
// shows them. "Home" isn't one of these - it's always $HOME itself.
var xdgUserDirKeys = []struct {
	key  string
	name string
}{
	{"XDG_DESKTOP_DIR", "Desktop"},
	{"XDG_DOCUMENTS_DIR", "Documents"},
	{"XDG_DOWNLOAD_DIR", "Downloads"},
	{"XDG_MUSIC_DIR", "Music"},
	{"XDG_PICTURES_DIR", "Pictures"},
	{"XDG_VIDEOS_DIR", "Videos"},
}

// ListPlaces returns real filesystem locations for the Places menu: the
// user's actual home directory, plus whichever standard XDG user dirs
// (~/.config/user-dirs.dirs, the same file xdg-user-dirs-update maintains)
// actually exist on disk. A fresh account (no user-dirs.dirs yet, e.g. this
// dev machine's throwaway `revuser` test account) falls back to the
// conventional $HOME/<Name> paths - still only listed if they really exist,
// not assumed.
func (a *App) ListPlaces() []PlaceEntry {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	places := []PlaceEntry{{Name: "Home", Path: home}}
	configured := parseUserDirs(filepath.Join(home, ".config", "user-dirs.dirs"))

	for _, dir := range xdgUserDirKeys {
		path, ok := configured[dir.key]
		if !ok {
			path = filepath.Join(home, dir.name)
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			places = append(places, PlaceEntry{Name: dir.name, Path: path})
		}
	}
	return places
}

// parseUserDirs reads the handful of XDG_*_DIR="..." lines xdg-user-dirs
// writes - a tiny shell-variable-style format, not full shell parsing (the
// real file is machine-generated and always this simple; $HOME expansion is
// the only thing actually used in practice).
func parseUserDirs(path string) map[string]string {
	result := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()

	home, _ := os.UserHomeDir()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "XDG_") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		value = strings.ReplaceAll(value, "$HOME", home)
		result[key] = value
	}
	return result
}

// OpenPlace opens a real location in the user's default file manager.
func (a *App) OpenPlace(path string) error {
	return exec.Command("xdg-open", path).Start()
}
