package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// RULED OUT (2026-09-30): bumped to 600 to test whether the panel's tiny
// height was tripping a WebKitGTK compositing path. Result: at 600px the
// window/background render exactly as they did at 32px (correct size,
// position, background colour), but WebKit's content still never
// composites - identical symptom, just a bigger visible rectangle. Not a
// geometry issue. See PROJECT.md.
const panelHeight = 32

// App struct
type App struct {
	ctx         context.Context
	screenWidth uint16
	tray        *trayWatcher
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// detectScreenWidth opens a throwaway X11 connection to read the default
// screen's real width. An earlier version hardcoded this to 5120 (this dev
// machine's own display), which would leave the panel comically oversized -
// or the strut's horizontal extent wrong - on any other screen, e.g. a VM's
// default 1024x768 or 1920x1080. Needed before the Wails window itself is
// created (main.go), so this can't just reuse claimAsDock's later connection.
func detectScreenWidth() uint16 {
	c, err := xgb.NewConn()
	if err != nil {
		log.Printf("failed to detect screen width, falling back to 1920: %v", err)
		return 1920
	}
	defer c.Close()
	return xproto.Setup(c).DefaultScreen(c).WidthInPixels
}

// startup pins the Wails window to the top of the screen and claims it as a
// dock/panel via raw EWMH property calls - Wails itself has no concept of
// dock windows, so this reaches past it to the underlying X11 window.
// Proven working standalone first (bare xgb window, verified _NET_WORKAREA
// actually shifts and reverts) before wiring into a real Wails window here.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowSetPosition(ctx, 0, 0)

	go func() {
		if err := a.claimAsDock(); err != nil {
			log.Printf("failed to claim panel window as dock: %v", err)
		}
	}()

	tray, err := newTrayWatcher()
	if err != nil {
		log.Printf("failed to start tray watcher: %v", err)
	}
	a.tray = tray
}

// ListTrayItems returns real registered StatusNotifierItems, or an empty
// list if the tray watcher isn't running (e.g. another watcher already owns
// the bus name - see newTrayWatcher).
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

func (a *App) claimAsDock() error {
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer c.Close()

	root := xproto.Setup(c).DefaultScreen(c).Root
	pid := uint32(os.Getpid())

	win, err := findOwnWindow(c, root, pid)
	if err != nil {
		return err
	}

	wmWindowType := internAtom(c, "_NET_WM_WINDOW_TYPE")
	wmWindowTypeDock := internAtom(c, "_NET_WM_WINDOW_TYPE_DOCK")
	wmStrutPartial := internAtom(c, "_NET_WM_STRUT_PARTIAL")
	wmStrut := internAtom(c, "_NET_WM_STRUT")

	setProp32(c, win, wmWindowType, xproto.AtomAtom, []uint32{uint32(wmWindowTypeDock)})

	// strut order: left, right, top, bottom, then start/end pairs.
	width := uint32(a.screenWidth)
	strutPartial := []uint32{0, 0, panelHeight, 0, 0, 0, 0, 0, 0, width, 0, 0}
	setProp32(c, win, wmStrutPartial, xproto.AtomCardinal, strutPartial)
	setProp32(c, win, wmStrut, xproto.AtomCardinal, []uint32{0, 0, panelHeight, 0})

	// TRIED AND RULED OUT (2026-09-28): a WindowHide()+WindowShow() cycle
	// here, on the theory that EWMH wants _NET_WM_WINDOW_TYPE set before
	// mapping and claimAsDock (running from OnStartup) sets it after Wails
	// already mapped the window. Real evidence against: a second
	// WEBKIT_DEBUG=all capture with the hide/show in place shows WebKit
	// re-running its ENTIRE load (fresh WebPage::constructor, fresh scheme
	// handler registration, fresh navigationID) and reaching
	// DidFirstVisuallyNonEmptyLayout + DidFirstMeaningfulPaint again - same
	// internal-success-but-invisible outcome as without the fix. Whatever's
	// wrong, it survives a full reload triggered post-dock-claim, so this
	// is not a window-mapping-order issue. Don't re-try this without new
	// evidence pointing back at it specifically.

	log.Printf("claimed window 0x%x as dock, reserving %dpx at top (screen width %dpx)", win, panelHeight, a.screenWidth)
	return nil
}

// findOwnWindow polls _NET_CLIENT_LIST for a window whose _NET_WM_PID
// matches our own process - retried briefly since OnStartup can fire before
// the window manager has finished registering the newly-mapped window.
func findOwnWindow(c *xgb.Conn, root xproto.Window, pid uint32) (xproto.Window, error) {
	clientList := internAtom(c, "_NET_CLIENT_LIST")
	wmPid := internAtom(c, "_NET_WM_PID")

	for attempt := 0; attempt < 20; attempt++ {
		reply, err := xproto.GetProperty(c, false, root, clientList, xproto.AtomWindow, 0, 1024).Reply()
		if err == nil && reply != nil {
			windows := reply.Value
			for i := 0; i+4 <= len(windows); i += 4 {
				wid := xproto.Window(le32(windows[i : i+4]))
				pidReply, err := xproto.GetProperty(c, false, wid, wmPid, xproto.AtomCardinal, 0, 1).Reply()
				if err == nil && pidReply != nil && len(pidReply.Value) >= 4 {
					if le32(pidReply.Value[0:4]) == pid {
						return wid, nil
					}
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return 0, os.ErrNotExist
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
