package main

import (
	"log"

	"github.com/gotk3/gotk3/gdk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const panelHeight = 32

// App holds the panel's non-UI state - tray watcher, screen geometry. Same
// role as the old Wails App struct, just without a JS-bridge context to
// carry around.
type App struct {
	screenWidth uint16
	tray        *trayWatcher
}

func NewApp() *App {
	return &App{}
}

func detectScreenWidth() uint16 {
	display, err := gdk.DisplayGetDefault()
	if err != nil {
		log.Printf("failed to detect screen width, falling back to 1920: %v", err)
		return 1920
	}
	monitor, err := display.GetPrimaryMonitor()
	if err != nil {
		log.Printf("failed to get primary monitor, falling back to 1920: %v", err)
		return 1920
	}
	_, _, width, _ := monitor.GetGeometry().GetRectangleInt()
	return uint16(width)
}

func (a *App) startup() {
	tray, err := newTrayWatcher()
	if err != nil {
		log.Printf("failed to start tray watcher: %v", err)
	}
	a.tray = tray
}

func (a *App) ListTrayItems() []TrayItem {
	if a.tray == nil {
		return nil
	}
	return a.tray.ListItems()
}

func (a *App) ActivateTrayItem(service, path string) error {
	if a.tray == nil {
		return nil
	}
	return a.tray.Activate(service, path)
}

// claimAsDock reserves strut space along the top of the screen via a raw
// EWMH property call - GTK's own SetTypeHint(gdk.WINDOW_TYPE_HINT_DOCK)
// (called in main.go before the window is realized) already handles
// _NET_WM_WINDOW_TYPE_DOCK through its normal API, but GTK3 has no
// equivalent high-level call for _NET_WM_STRUT_PARTIAL, so that part still
// has to reach past it the same way the old Wails version did. The one real
// simplification over that version: GDK hands back this window's actual X11
// XID directly (gdk_x11_window_get_xid), so there's no need for the old
// findOwnWindow _NET_CLIENT_LIST/PID-polling workaround - we already know
// exactly which window we are.
func (a *App) claimAsDock(xid xproto.Window) error {
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer c.Close()

	wmStrutPartial := internAtom(c, "_NET_WM_STRUT_PARTIAL")
	wmStrut := internAtom(c, "_NET_WM_STRUT")

	// strut order: left, right, top, bottom, then start/end pairs.
	width := uint32(a.screenWidth)
	strutPartial := []uint32{0, 0, panelHeight, 0, 0, 0, 0, 0, 0, width, 0, 0}
	setProp32(c, xid, wmStrutPartial, xproto.AtomCardinal, strutPartial)
	setProp32(c, xid, wmStrut, xproto.AtomCardinal, []uint32{0, 0, panelHeight, 0})

	log.Printf("claimed window 0x%x as dock, reserving %dpx at top (screen width %dpx)", xid, panelHeight, a.screenWidth)
	return nil
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func internAtom(c *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
	if err != nil {
		log.Fatalf("intern atom %s: %v", name, err)
	}
	return reply.Atom
}

func setProp32(c *xgb.Conn, win xproto.Window, propAtom xproto.Atom, typeAtom xproto.Atom, data []uint32) {
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		buf[i*4] = byte(v)
		buf[i*4+1] = byte(v >> 8)
		buf[i*4+2] = byte(v >> 16)
		buf[i*4+3] = byte(v >> 24)
	}
	err := xproto.ChangePropertyChecked(
		c, xproto.PropModeReplace, win, propAtom, typeAtom, 32,
		uint32(len(data)), buf,
	).Check()
	if err != nil {
		log.Printf("set property %d failed: %v", propAtom, err)
	}
}
