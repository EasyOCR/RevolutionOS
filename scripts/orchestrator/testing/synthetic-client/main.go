// Minimal synthetic X11 client for stress-testing orchestrator instance isolation
// (see ../../test-synthetic-isolation.sh). Opens a plain window titled with
// whatever's passed on argv, sets both WM_NAME and _NET_WM_NAME (window managers/
// wmctrl vary in which they read), and blocks until killed. Deliberately reuses
// the same xgb dependency panel/ already has rather than pulling in a new one
// (xdotool/xterm/xclock/xeyes are all absent from this host - no system package
// needed this way).
package main

import (
	"log"
	"os"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <window-title>", os.Args[0])
	}
	title := os.Args[1]

	c, err := xgb.NewConn()
	if err != nil {
		log.Fatalf("connect to %s: %v", os.Getenv("DISPLAY"), err)
	}
	defer c.Close()

	screen := xproto.Setup(c).DefaultScreen(c)
	win, err := xproto.NewWindowId(c)
	if err != nil {
		log.Fatalf("alloc window id: %v", err)
	}

	err = xproto.CreateWindowChecked(
		c, screen.RootDepth, win, screen.Root,
		0, 0, 200, 100, 1,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwBackPixel, []uint32{screen.WhitePixel},
	).Check()
	if err != nil {
		log.Fatalf("create window: %v", err)
	}

	setProp(c, win, "WM_NAME", "STRING", []byte(title))
	netWmName := internAtom(c, "_NET_WM_NAME")
	utf8String := internAtom(c, "UTF8_STRING")
	xproto.ChangePropertyChecked(c, xproto.PropModeReplace, win, netWmName, utf8String, 8, uint32(len(title)), []byte(title)).Check()

	if err := xproto.MapWindowChecked(c, win).Check(); err != nil {
		log.Fatalf("map window: %v", err)
	}

	log.Printf("synthetic client %q mapped on %s (window 0x%x), pid %d", title, os.Getenv("DISPLAY"), win, os.Getpid())

	// Block until killed - the test script manages this process's lifetime.
	for {
		time.Sleep(time.Hour)
	}
}

func internAtom(c *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
	if err != nil {
		log.Fatalf("intern atom %s: %v", name, err)
	}
	return reply.Atom
}

func setProp(c *xgb.Conn, win xproto.Window, propName, typeName string, data []byte) {
	prop := internAtom(c, propName)
	typ := internAtom(c, typeName)
	err := xproto.ChangePropertyChecked(c, xproto.PropModeReplace, win, prop, typ, 8, uint32(len(data)), data).Check()
	if err != nil {
		log.Printf("set property %s failed: %v", propName, err)
	}
}
