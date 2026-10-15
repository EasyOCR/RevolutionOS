package main

import (
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// WindowEntry is a real open window, read from the live X11 session via
// _NET_CLIENT_LIST - not mock data.
type WindowEntry struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}

// ListWindows reads the window manager's real _NET_CLIENT_LIST for the
// taskbar. Skips DOCK-type windows (the panel itself sets this - see
// claimAsDock in app.go - so this naturally excludes the panel from its own
// taskbar without needing to hardcode a title/PID match against itself) and
// windows with no title (skips override-redirect/utility windows a real
// taskbar wouldn't show either).
func (a *App) ListWindows() []WindowEntry {
	c, err := xgb.NewConn()
	if err != nil {
		return nil
	}
	defer c.Close()

	root := xproto.Setup(c).DefaultScreen(c).Root
	clientList := internAtom(c, "_NET_CLIENT_LIST")
	netWmName := internAtom(c, "_NET_WM_NAME")
	utf8String := internAtom(c, "UTF8_STRING")
	wmWindowType := internAtom(c, "_NET_WM_WINDOW_TYPE")
	wmWindowTypeDock := internAtom(c, "_NET_WM_WINDOW_TYPE_DOCK")

	reply, err := xproto.GetProperty(c, false, root, clientList, xproto.AtomWindow, 0, 1024).Reply()
	if err != nil || reply == nil {
		return nil
	}

	var windows []WindowEntry
	for i := 0; i+4 <= len(reply.Value); i += 4 {
		win := xproto.Window(le32(reply.Value[i : i+4]))

		if windowIsType(c, win, wmWindowType, wmWindowTypeDock) {
			continue
		}
		title := windowTitle(c, win, netWmName, utf8String)
		if title == "" {
			continue
		}
		windows = append(windows, WindowEntry{ID: uint32(win), Title: title})
	}
	return windows
}

// ActivateWindow asks the window manager to raise and focus a window via the
// standard EWMH _NET_ACTIVE_WINDOW client message (the same mechanism any
// real taskbar/pager uses) rather than trying to move focus directly, which
// EWMH-aware window managers expect and metacity honors.
func (a *App) ActivateWindow(id uint32) error {
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer c.Close()

	root := xproto.Setup(c).DefaultScreen(c).Root
	activeWindow := internAtom(c, "_NET_ACTIVE_WINDOW")
	win := xproto.Window(id)

	// source indication 2 = "pager" (i.e. a taskbar/panel), per EWMH spec.
	data := xproto.ClientMessageDataUnionData32New([]uint32{2, 0, 0, 0, 0})
	event := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   activeWindow,
		Data:   data,
	}
	eventMask := uint32(xproto.EventMaskSubstructureNotify | xproto.EventMaskSubstructureRedirect)
	return xproto.SendEventChecked(c, false, root, eventMask, string(event.Bytes())).Check()
}

func windowIsType(c *xgb.Conn, win xproto.Window, wmWindowType, want xproto.Atom) bool {
	reply, err := xproto.GetProperty(c, false, win, wmWindowType, xproto.AtomAtom, 0, 8).Reply()
	if err != nil || reply == nil {
		return false
	}
	for i := 0; i+4 <= len(reply.Value); i += 4 {
		if xproto.Atom(le32(reply.Value[i:i+4])) == want {
			return true
		}
	}
	return false
}

func windowTitle(c *xgb.Conn, win xproto.Window, netWmName, utf8String xproto.Atom) string {
	reply, err := xproto.GetProperty(c, false, win, netWmName, utf8String, 0, 256).Reply()
	if err != nil || reply == nil || len(reply.Value) == 0 {
		return ""
	}
	return string(reply.Value)
}
