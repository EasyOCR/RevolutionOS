package main

import (
	"log"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/jezek/xgb/xproto"
)

const panelCSS = `
window, box {
	background-color: #ffffff;
}
#panel-root {
	border-bottom: 1px solid #dee2e6;
}
button {
	background: transparent;
	background-image: none;
	border: none;
	box-shadow: none;
	border-radius: 8px;
	color: #495057;
}
button:focus, button.menu-active {
	background-color: #edeff2;
}
button:hover {
	background-color: #f1f3f5;
}
`

func main() {
	gtk.Init(nil)

	app := NewApp()
	app.screenWidth = detectScreenWidth()

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal(err)
	}
	win.SetTitle("panel")
	win.SetDecorated(false)
	win.SetResizable(false)
	win.SetDefaultSize(int(app.screenWidth), panelHeight)
	win.SetTypeHint(gdk.WINDOW_TYPE_HINT_DOCK)
	win.Move(0, 0)
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	applyCSS(win)

	root := buildUI(app)
	root.SetName("panel-root")
	win.Add(root)

	win.Connect("realize", func() {
		gdkWin, err := win.GetWindow()
		if err != nil {
			log.Printf("failed to get GDK window for dock claim: %v", err)
			return
		}
		xid := xproto.Window(gdkWin.GetXID())
		if err := app.claimAsDock(xid); err != nil {
			log.Printf("failed to claim panel window as dock: %v", err)
		}
	})

	win.ShowAll()
	app.startup()

	gtk.Main()
}

func applyCSS(win *gtk.Window) {
	provider, err := gtk.CssProviderNew()
	if err != nil {
		log.Printf("failed to create CSS provider: %v", err)
		return
	}
	if err := provider.LoadFromData(panelCSS); err != nil {
		log.Printf("failed to load panel CSS: %v", err)
		return
	}
	screen := win.GetScreen()
	gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

// glibIdle is a small helper for UI callbacks that need to run update logic
// on a timer - every refresh loop in ui.go uses this same pattern.
func glibInterval(seconds uint, f func() bool) {
	glib.TimeoutAdd(seconds*1000, f)
}
