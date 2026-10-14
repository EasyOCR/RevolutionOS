package main

import "os/exec"

// System menu actions - real, not stubbed. Confirmed real commands by
// inspecting this host directly rather than assuming generic systemd calls:
// gnome-session-quit is the session-integrated tool (respects the actual
// GNOME session state, unlike calling loginctl/systemctl directly which
// bypasses the desktop session entirely), xdg-screensaver is the portable
// lock mechanism (no gnome-screensaver-command on this system).
//
// Deliberately no Go tests in this file, unlike applications.go/places.go/
// windows.go - there is no safe way to test "does this actually log out /
// reboot / power off" without doing exactly that to whatever machine runs
// the test. These are only meant to ever be exercised inside a disposable
// VM, never on a real host - see PROJECT.md.

func (a *App) LockScreen() error {
	return exec.Command("xdg-screensaver", "lock").Start()
}

func (a *App) LogOut() error {
	return exec.Command("gnome-session-quit", "--logout", "--no-prompt").Start()
}

func (a *App) Restart() error {
	return exec.Command("gnome-session-quit", "--reboot", "--no-prompt").Start()
}

func (a *App) ShutDown() error {
	return exec.Command("gnome-session-quit", "--power-off", "--no-prompt").Start()
}
