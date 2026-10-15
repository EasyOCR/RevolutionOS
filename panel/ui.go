package main

import (
	"log"
	"time"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

func buildUI(app *App) *gtk.Box {
	root, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	root.SetBorderWidth(4)

	root.PackStart(buildApplicationsMenu(app), false, false, 0)
	root.PackStart(buildPlacesMenu(app), false, false, 0)

	windowsBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	root.PackStart(windowsBox, true, true, 0)
	refreshWindows(app, windowsBox)
	glibInterval(2, func() bool {
		refreshWindows(app, windowsBox)
		return true
	})

	trayBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	root.PackStart(trayBox, false, false, 0)
	refreshTray(app, trayBox)
	glibInterval(2, func() bool {
		refreshTray(app, trayBox)
		return true
	})

	clock, _ := gtk.LabelNew(time.Now().Format("3:04 PM"))
	root.PackStart(clock, false, false, 4)
	glibInterval(1, func() bool {
		clock.SetText(time.Now().Format("3:04 PM"))
		return true
	})

	root.PackStart(buildSystemMenu(app), false, false, 0)

	return root
}

// popupBelow shows a freshly-built menu anchored under the button that
// triggered it. Plain gtk.Button + a manual PopupAtWidget call here, rather
// than gtk.MenuButton + SetPopup - MenuButton manages its own popup lifecycle
// internally and fighting that from a "clicked" handler that rebuilds the
// menu each time proved unreliable (clicks weren't reaching the menu at
// all). A plain button's "clicked" signal has no such ambiguity.
//
// The button's highlight is driven by an explicit "menu-active" style class,
// not CSS :focus - GTK moves keyboard focus into the menu itself once it's
// shown (so arrow keys navigate the menu items), which means the originating
// button's :focus state doesn't persist for as long as its menu stays open.
// This class is added right before the popup appears and removed via the
// menu's own "hide" signal, so the top-bar button stays highlighted for the
// menu's actual visible lifetime.
func popupBelow(btn *gtk.Button, menu *gtk.Menu) {
	ctx, err := btn.GetStyleContext()
	if err != nil {
		log.Printf("failed to get style context for menu button: %v", err)
	} else {
		ctx.AddClass("menu-active")
		menu.Connect("hide", func() {
			ctx.RemoveClass("menu-active")
		})
	}

	menu.ShowAll()
	menu.PopupAtWidget(btn, gdk.GDK_GRAVITY_SOUTH_WEST, gdk.GDK_GRAVITY_NORTH_WEST, nil)
}

func buildApplicationsMenu(app *App) *gtk.Button {
	btn, _ := gtk.ButtonNewWithLabel("Applications")

	btn.Connect("clicked", func() {
		menu, _ := gtk.MenuNew()
		apps := app.ListApplications()
		if len(apps) == 0 {
			item, _ := gtk.MenuItemNewWithLabel("No applications found")
			item.SetSensitive(false)
			menu.Append(item)
		}
		for _, a := range apps {
			a := a
			item, _ := gtk.MenuItemNewWithLabel(a.Name)
			item.Connect("activate", func() {
				if err := app.LaunchApplication(a.Exec); err != nil {
					log.Printf("launch %q failed: %v", a.Name, err)
				}
			})
			menu.Append(item)
		}
		popupBelow(btn, menu)
	})

	return btn
}

func buildPlacesMenu(app *App) *gtk.Button {
	btn, _ := gtk.ButtonNewWithLabel("Places")

	btn.Connect("clicked", func() {
		menu, _ := gtk.MenuNew()
		places := app.ListPlaces()
		if len(places) == 0 {
			item, _ := gtk.MenuItemNewWithLabel("No places found")
			item.SetSensitive(false)
			menu.Append(item)
		}
		for _, p := range places {
			p := p
			item, _ := gtk.MenuItemNewWithLabel(p.Name)
			item.Connect("activate", func() {
				if err := app.OpenPlace(p.Path); err != nil {
					log.Printf("open place %q failed: %v", p.Name, err)
				}
			})
			menu.Append(item)
		}
		popupBelow(btn, menu)
	})

	return btn
}

func refreshWindows(app *App, box *gtk.Box) {
	clearChildren(box)
	for _, w := range app.ListWindows() {
		w := w
		label := w.Title
		if len(label) > 32 {
			label = label[:32] + "…"
		}
		btn, _ := gtk.ButtonNewWithLabel(label)
		btn.SetTooltipText(w.Title)
		btn.Connect("clicked", func() {
			if err := app.ActivateWindow(w.ID); err != nil {
				log.Printf("activate window %d failed: %v", w.ID, err)
			}
		})
		box.PackStart(btn, false, false, 0)
	}
	box.ShowAll()
}

func refreshTray(app *App, box *gtk.Box) {
	clearChildren(box)
	for _, item := range app.ListTrayItems() {
		item := item
		label := item.Title
		if label == "" {
			label = item.Service
		}
		btn, _ := gtk.ButtonNewWithLabel(label)
		btn.Connect("clicked", func() {
			if err := app.ActivateTrayItem(item.Service, item.Path); err != nil {
				log.Printf("activate tray item %s failed: %v", item.Service, err)
			}
		})
		box.PackStart(btn, false, false, 0)
	}
	box.ShowAll()
}

func clearChildren(box *gtk.Box) {
	children := box.GetChildren()
	children.Foreach(func(item interface{}) {
		if w, ok := item.(gtk.IWidget); ok {
			box.Remove(w)
		}
	})
}

func buildSystemMenu(app *App) *gtk.Button {
	btn, _ := gtk.ButtonNewWithLabel("System")

	menu, _ := gtk.MenuNew()

	lock, _ := gtk.MenuItemNewWithLabel("Lock Screen")
	lock.Connect("activate", func() {
		if err := app.LockScreen(); err != nil {
			log.Printf("lock screen failed: %v", err)
		}
	})
	menu.Append(lock)

	logout, _ := gtk.MenuItemNewWithLabel("Log Out")
	logout.Connect("activate", func() {
		if err := app.LogOut(); err != nil {
			log.Printf("log out failed: %v", err)
		}
	})
	menu.Append(logout)

	restart, _ := gtk.MenuItemNewWithLabel("Restart")
	restart.Connect("activate", func() {
		confirmThen("Restart now?", func() {
			if err := app.Restart(); err != nil {
				log.Printf("restart failed: %v", err)
			}
		})
	})
	menu.Append(restart)

	shutdown, _ := gtk.MenuItemNewWithLabel("Shut Down")
	shutdown.Connect("activate", func() {
		confirmThen("Shut down now?", func() {
			if err := app.ShutDown(); err != nil {
				log.Printf("shut down failed: %v", err)
			}
		})
	})
	menu.Append(shutdown)

	btn.Connect("clicked", func() {
		popupBelow(btn, menu)
	})

	return btn
}

// confirmThen matches the old React UI's confirm() speed bump in front of
// Restart/ShutDown - these end the session they run in, so a stray click
// shouldn't be able to do that for real with no second step. Only ever meant
// to be exercised inside a disposable VM, never a real host - see system.go.
func confirmThen(question string, action func()) {
	dialog := gtk.MessageDialogNew(nil, gtk.DIALOG_MODAL, gtk.MESSAGE_WARNING, gtk.BUTTONS_OK_CANCEL, "%s", question)
	defer dialog.Destroy()
	if dialog.Run() == gtk.RESPONSE_OK {
		action()
	}
}
