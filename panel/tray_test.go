package main

import (
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// TestTrayWatcher is a full, real test: launches a genuinely isolated D-Bus
// session bus (dbus-launch), not the host's real one - this dev machine's
// real session bus already has gnome-shell holding
// org.kde.StatusNotifierWatcher, so testing against it would only exercise
// the "someone else already owns this" fallback path, never the real
// registration/activation logic. Then runs a synthetic fake tray item
// against our real trayWatcher and confirms it's tracked and activatable -
// not mocked.
func TestTrayWatcher(t *testing.T) {
	addr, pid := launchIsolatedBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	t.Cleanup(func() { exec.Command("kill", "-9", pid).Run() })

	watcher, err := newTrayWatcher()
	if err != nil {
		t.Fatalf("newTrayWatcher: %v", err)
	}
	if watcher == nil {
		t.Fatal("expected a real watcher on a fresh isolated bus, got nil (someone else already owns the name?)")
	}

	activated := make(chan struct{}, 1)
	fakeItemConn := connectFakeTrayItem(t, addr, activated)

	if err := watcher.RegisterStatusNotifierItem("", dbus.Sender(fakeItemConn.Names()[0])); err != nil {
		t.Fatalf("RegisterStatusNotifierItem: %v", err)
	}

	items := watcher.ListItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 registered item, got %d: %+v", len(items), items)
	}
	if items[0].Title != "Fake Tray Item" {
		t.Errorf("expected real title fetched from the fake item, got %q", items[0].Title)
	}
	t.Logf("watcher correctly tracked real item: %+v", items[0])

	if err := watcher.Activate(items[0].Service, items[0].Path); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	select {
	case <-activated:
		t.Log("fake item's real Activate method was actually called")
	case <-time.After(2 * time.Second):
		t.Fatal("Activate did not reach the fake item within 2s")
	}
}

func launchIsolatedBus(t *testing.T) (address, pid string) {
	t.Helper()
	out, err := exec.Command("dbus-launch", "--sh-syntax").Output()
	if err != nil {
		t.Fatalf("dbus-launch: %v", err)
	}
	addrRe := regexp.MustCompile(`DBUS_SESSION_BUS_ADDRESS='([^']+)'`)
	pidRe := regexp.MustCompile(`DBUS_SESSION_BUS_PID=(\d+)`)
	addrMatch := addrRe.FindSubmatch(out)
	pidMatch := pidRe.FindSubmatch(out)
	if addrMatch == nil || pidMatch == nil {
		t.Fatalf("could not parse dbus-launch output: %s", out)
	}
	return string(addrMatch[1]), string(pidMatch[1])
}

// connectFakeTrayItem simulates a real app that wants a tray icon: connects
// to the bus, exports a real /StatusNotifierItem object with a real Title
// property (via the real org.freedesktop.DBus.Properties machinery, not
// hand-rolled) and a real Activate method.
func connectFakeTrayItem(t *testing.T, addr string, activated chan<- struct{}) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatalf("fake item connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	props := prop.New(conn, sniItemPath, map[string]map[string]*prop.Prop{
		sniItemIface: {
			"Title": {Value: "Fake Tray Item", Writable: false, Emit: prop.EmitFalse},
		},
	})
	if props == nil {
		t.Fatal("prop.New returned nil")
	}

	handler := &fakeItemHandler{activated: activated}
	if err := conn.Export(handler, sniItemPath, sniItemIface); err != nil {
		t.Fatalf("export fake item: %v", err)
	}
	return conn
}

type fakeItemHandler struct {
	activated chan<- struct{}
}

func (h *fakeItemHandler) Activate(x, y int32) *dbus.Error {
	select {
	case h.activated <- struct{}{}:
	default:
	}
	return nil
}
