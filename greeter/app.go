package main

import (
	"context"
	"log"
)

// flashbackSessionCmd launches GNOME Flashback/Metacity - the only session
// RevolutionOS actually ships (see build.sh's gnome-flashback install and
// PROJECT.md's "Resolved: X11 (Flashback) is the actual target" section).
// Unlike GDM, which showed a session picker that could silently default to
// the wrong entry (see PROJECT.md's 2026-09-27 GDM investigation), this
// greeter has no picker at all - there's only one session, so it's simply
// what StartSession launches on a successful login.
//
// Wrapped in `startx` - confirmed by a real VM login (2026-09-27) that
// authenticated fully (PAM/gnome-keyring both succeeded) and even got as far
// as starting gnome-session and gnome-flashback.service, which then died
// with "cannot open display: ". GDM used to spawn its own Xorg for the user
// session automatically; greetd doesn't do that for us, and this is a
// completely separate concern from the greeter's own display (which
// correctly runs under cage/Wayland - see system_files' greetd config.toml -
// since nothing here needs GDM's X11-hosting behavior once the greeter
// itself is done). Reuses vt1: greetd's own VT (config.toml's `vt = 1`) is
// free again once the greeter process this replaces has exited.
var flashbackSessionCmd = []string{"startx", "/usr/bin/gnome-session", "--session=gnome-flashback-metacity", "--", "vt1"}

// flashbackSessionEnv sets GDK_BACKEND=x11 explicitly - the orchestrator hit
// a real bug this same session where a stray WAYLAND_DISPLAY leaking into
// GTK3's backend selection silently broke X11-only components (see
// start-instance.sh's fix, same root cause class). Flashback/Metacity is
// X11-only, so this forecloses that whole bug class here too rather than
// hoping greetd's environment never has Wayland leftovers.
var flashbackSessionEnv = []string{
	"GDK_BACKEND=x11",
	"XDG_SESSION_TYPE=x11",
	"XDG_SESSION_DESKTOP=gnome-flashback-metacity",
	"DESKTOP_SESSION=gnome-flashback-metacity",
}

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Login drives one full greetd auth exchange: create_session, answer
// whatever auth prompts come back with the given password (this product only
// supports plain PAM password auth - no need to generalize to arbitrary
// conversations), and start the Flashback session on success. Returns a
// user-facing message on failure, or "" on success.
//
// Dials a fresh connection per attempt rather than reusing one across
// retries - simpler than tracking greetd's session-cancellation semantics,
// and a failed attempt shouldn't leave any state for the next one to trip on.
func (a *App) Login(username, password string) string {
	g, err := dialGreetd()
	if err != nil {
		log.Printf("greetd dial failed: %v", err)
		return "Could not reach the login service."
	}
	defer g.Close()

	resp, err := g.createSession(username)
	if err != nil {
		log.Printf("create_session failed: %v", err)
		return "Could not reach the login service."
	}

	for resp["type"] == "auth_message" {
		resp, err = g.postAuthMessageResponse(password)
		if err != nil {
			log.Printf("post_auth_message_response failed: %v", err)
			return "Could not reach the login service."
		}
	}

	switch resp["type"] {
	case "success":
		startResp, err := g.startSession(flashbackSessionCmd, flashbackSessionEnv)
		if err != nil {
			log.Printf("start_session failed: %v", err)
			return "Login succeeded but the session failed to start."
		}
		if startResp["type"] == "error" {
			log.Printf("start_session error: %v", startResp["description"])
			return "Login succeeded but the session failed to start."
		}
		return ""
	case "error":
		log.Printf("auth error: %v", resp["description"])
		return "Incorrect username or password."
	default:
		log.Printf("unexpected greetd response type: %v", resp["type"])
		return "Unexpected response from the login service."
	}
}
