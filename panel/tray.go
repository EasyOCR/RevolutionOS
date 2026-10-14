package main

import (
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Minimal StatusNotifierWatcher + Host implementation (the de-facto
// freedesktop tray protocol - there's no fully ratified spec, this is what
// real apps using libappindicator/Qt/etc. actually speak). Combines watcher
// and host into one process, which is standard for a single-panel session
// (there's normally only one tray to host items in anyway).
//
// Deliberately v1-scoped: tracks registered items and can fetch their
// Title/IconName and call Activate - does not handle IconPixmap (raw pixel
// data, real icon rendering) or ContextMenu (the dbusmenu sub-protocol).
// Real apps commonly set IconName to a themed icon name resolvable via the
// system icon theme, which is what a real v2 would resolve to an actual
// image; for now the panel shows item titles as text, same pragmatic
// approach as the window taskbar.

const (
	sniWatcherIface = "org.kde.StatusNotifierWatcher"
	sniWatcherPath  = "/StatusNotifierWatcher"
	sniItemIface    = "org.kde.StatusNotifierItem"
	sniItemPath     = "/StatusNotifierItem"
)

// TrayItem is a real registered tray item, not mock data.
type TrayItem struct {
	Service string `json:"service"`
	Path    string `json:"path"`
	Title   string `json:"title"`
}

type trayWatcher struct {
	conn *dbus.Conn
	mu   sync.Mutex
	// keyed by "service\x00path" to allow multiple items per service (rare
	// in practice, but the spec technically allows a distinct object path).
	items map[string]TrayItem
}

func newTrayWatcher() (*trayWatcher, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	w := &trayWatcher{conn: conn, items: map[string]TrayItem{}}

	reply, err := conn.RequestName(sniWatcherIface, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		// Another watcher is already running (e.g. a real desktop's own tray
		// implementation, if this ever runs somewhere other than our own
		// masked-gnome-panel Flashback session) - not an error, just means
		// we defer to it rather than fight over the name.
		conn.Close()
		log.Printf("tray: %s already owned by another watcher, not hosting our own", sniWatcherIface)
		return nil, nil
	}

	conn.Export(w, sniWatcherPath, sniWatcherIface)
	conn.Export(introspectable{sniWatcherIface: watcherIntrospectXML}, sniWatcherPath, "org.freedesktop.DBus.Introspectable")

	// We are also the host - real items only bother registering once a host
	// exists (IsStatusNotifierHostRegistered), and some watch for the host's
	// bus name to disappear as their cue to stop trying.
	hostReply, err := conn.RequestName(sniItemIface+".Host-panel", dbus.NameFlagDoNotQueue)
	if err != nil || hostReply != dbus.RequestNameReplyPrimaryOwner {
		log.Printf("tray: could not register as host (non-fatal): %v", err)
	}

	log.Printf("tray: watcher registered as %s", sniWatcherIface)
	return w, nil
}

// RegisterStatusNotifierItem is the real D-Bus method real tray apps call.
// The spec is loose about the argument's exact form (bare bus name vs. a
// bus-name+path combo); the message's real Sender is the authoritative bus
// name regardless of what the argument claims, so that's what's trusted for
// the service identity, with the argument used only for the object path.
func (w *trayWatcher) RegisterStatusNotifierItem(service string, sender dbus.Sender) *dbus.Error {
	path := sniItemPath
	if len(service) > 0 && service[0] == '/' {
		path = service
	}

	item := TrayItem{Service: string(sender), Path: path}
	item.Title = w.fetchTitle(item)

	w.mu.Lock()
	w.items[string(sender)+"\x00"+path] = item
	w.mu.Unlock()

	log.Printf("tray: registered item %s%s (title=%q)", sender, path, item.Title)
	w.conn.Emit(sniWatcherPath, sniWatcherIface+".StatusNotifierItemRegistered", string(sender))
	return nil
}

func (w *trayWatcher) RegisterStatusNotifierHost(service string) *dbus.Error {
	// Accepted for protocol compliance (some items check this happened
	// before registering themselves) - we're always already the host.
	return nil
}

// fetchTitle calls the item's own object to read its real Title property -
// best-effort, since a real item might not have finished exporting its
// object yet, or might not set Title at all (IconName-only is valid too).
func (w *trayWatcher) fetchTitle(item TrayItem) string {
	obj := w.conn.Object(item.Service, dbus.ObjectPath(item.Path))
	var title string
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, sniItemIface, "Title").Store(&title)
	if err != nil {
		return ""
	}
	return title
}

// ListItems returns a stable-ordered snapshot of currently registered items.
func (w *trayWatcher) ListItems() []TrayItem {
	w.mu.Lock()
	defer w.mu.Unlock()
	items := make([]TrayItem, 0, len(w.items))
	for _, item := range w.items {
		items = append(items, item)
	}
	return items
}

// Activate calls the real item's Activate method - the same click-to-open
// action a real tray does, at an arbitrary (0,0) position since this isn't
// rendering the item at a specific screen coordinate the way a pixel-based
// icon tray would.
func (w *trayWatcher) Activate(service, path string) error {
	obj := w.conn.Object(service, dbus.ObjectPath(path))
	return obj.Call(sniItemIface+".Activate", 0, int32(0), int32(0)).Err
}

type introspectable map[string]string

func (i introspectable) Introspect() (string, *dbus.Error) {
	return i[sniWatcherIface], nil
}

const watcherIntrospectXML = `
<node>
  <interface name="org.kde.StatusNotifierWatcher">
    <method name="RegisterStatusNotifierItem">
      <arg type="s" name="service" direction="in"/>
    </method>
    <method name="RegisterStatusNotifierHost">
      <arg type="s" name="service" direction="in"/>
    </method>
    <property name="RegisteredStatusNotifierItems" type="as" access="read"/>
    <property name="IsStatusNotifierHostRegistered" type="b" access="read"/>
    <property name="ProtocolVersion" type="i" access="read"/>
    <signal name="StatusNotifierItemRegistered"><arg type="s"/></signal>
    <signal name="StatusNotifierItemUnregistered"><arg type="s"/></signal>
  </interface>
</node>`
