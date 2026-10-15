package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const xvfbBin = "/home/linuxbrew/.linuxbrew/Cellar/xorg-server/21.1.24/bin/Xvfb"

// TestListWindows is a full, self-contained real test: builds the synthetic
// test client fresh, starts a real headless Xvfb + metacity on a throwaway
// display, maps a real window on it, and confirms ListWindows/ActivateWindow
// see and can act on it for real - not mocked, not just "the connection
// doesn't error." Uses the two real fixes found while stress-testing
// orchestrator isolation (Xvfb needs -xkbdir, metacity needs
// GDK_BACKEND=x11 - see PROJECT.md) - without them this test would fail the
// same way the orchestrator itself did before those fixes.
//
// Skips (doesn't fail) if this dev machine's specific Xvfb build isn't
// present, e.g. in a CI environment without Homebrew - the earlier
// lighter-weight version of this test (just checking the xgb connection
// path didn't error against whatever $DISPLAY happened to be ambient) is
// gone now that this does the real thing instead.
func TestListWindows(t *testing.T) {
	if _, err := os.Stat(xvfbBin); err != nil {
		t.Skipf("this dev machine's Xvfb (%s) not present - skipping", xvfbBin)
	}

	// synthetic-client is its own Go module (separate go.mod), so this needs
	// to build from within its own directory, not be referenced as a package
	// path from panel's module.
	clientBin := filepath.Join(t.TempDir(), "synthetic-client")
	build := exec.Command("go", "build", "-o", clientBin, ".")
	build.Dir = "../scripts/orchestrator/testing/synthetic-client"
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building synthetic-client: %v\n%s", err, out)
	}

	disp := ":90"
	socket := "/tmp/.X11-unix/X90"
	// Xvfb doesn't get a chance to unlink its own socket file when killed
	// with SIGKILL (t.Cleanup can't wait around for a graceful exit), so a
	// stale socket from a previous run can make the *next* run's socket-exists
	// check pass against a dead server - confirmed the hard way (this test
	// passed in isolation, then failed when run again immediately after as
	// part of the full suite). Removing it explicitly in cleanup, not just
	// killing the process, so runs are independent regardless of ordering.
	os.Remove(socket)

	xvfb := exec.Command(xvfbBin, disp, "-screen", "0", "640x480x24", "-xkbdir", "/usr/share/X11/xkb")
	if err := xvfb.Start(); err != nil {
		t.Fatalf("starting Xvfb: %v", err)
	}
	t.Cleanup(func() {
		xvfb.Process.Kill()
		xvfb.Wait()
		os.Remove(socket)
	})
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("Xvfb never created %s", socket)
	}

	metacity := exec.Command("metacity", "--replace")
	metacity.Env = append(os.Environ(), "DISPLAY="+disp, "GDK_BACKEND=x11")
	if err := metacity.Start(); err != nil {
		t.Fatalf("starting metacity: %v", err)
	}
	t.Cleanup(func() { metacity.Process.Kill(); metacity.Wait() })
	time.Sleep(500 * time.Millisecond)

	client := exec.Command(clientBin, "TEST-WINDOW-TITLE")
	client.Env = append(os.Environ(), "DISPLAY="+disp)
	if err := client.Start(); err != nil {
		t.Fatalf("starting synthetic client: %v", err)
	}
	t.Cleanup(func() { client.Process.Kill(); client.Wait() })
	time.Sleep(500 * time.Millisecond)

	t.Setenv("DISPLAY", disp)

	windows := (&App{}).ListWindows()
	t.Logf("found %d real window(s):", len(windows))
	for _, w := range windows {
		t.Logf("  id=0x%x title=%q", w.ID, w.Title)
	}

	var found *WindowEntry
	for i := range windows {
		if windows[i].Title == "TEST-WINDOW-TITLE" {
			found = &windows[i]
		}
	}
	if found == nil {
		t.Fatalf("expected to find TEST-WINDOW-TITLE among real windows, got %+v", windows)
	}

	if err := (&App{}).ActivateWindow(found.ID); err != nil {
		t.Errorf("ActivateWindow on a real window failed: %v", err)
	}
}
