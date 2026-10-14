# precise-input

Optional infrastructure RevolutionOS exposes per-instance, for Liam to integrate into
RevolutionMacro later if he wants tighter, lower-latency control than the generic X11 paths it
uses today. **Not required for anything else in this repo to work** - the orchestrator and
RevolutionMacro itself continue using RevolutionMacro's existing XTest/XComposite backends
regardless of whether this is ever adopted. This exists because RevolutionOS, uniquely, controls
the full environment each instance runs in - it can offer something neither "just use X11" nor
"just use raw uinput" can alone.

## Why this exists

RevolutionMacro currently injects input via X11's `XTest` extension and captures via
`XComposite` (confirmed via its own `linuxdiag` tool - see `PROJECT.md`). Both go through the X
server's normal dispatch path, which is fine, and is *why* it was picked as this project's
isolation mechanism (XTest/XComposite are inherently scoped to whichever `$DISPLAY` the client
connects to). But it's not the lowest-latency path available - the X server's own event queue
sits between the injection call and the game actually seeing it.

Raw kernel-level `uinput` input has no such intermediary - but it's normally a *global* device,
visible to every process on the machine regardless of which orchestrator instance it "belongs"
to. That's exactly the isolation problem the old `bss-automation` DIY project's virtual gamepad
hit and never solved (see that project's own notes, referenced in `PROJECT.md`). Nobody could
previously get both precision and isolation at once on a single machine without VMs.

**RevolutionOS can, because it already creates each instance's isolated environment.** It can
hand each instance a uinput device that only *that* instance can ever see, by creating the device
outside the sandbox and bind-mounting only that one device node into a fresh, otherwise-empty
`/dev` inside each instance's `bwrap` sandbox - the same primitive flatpak itself uses. A
different instance's sandbox never has the device node at all, not even to see it exists
(confirmed: attempting to open another instance's device node from inside a sandbox fails with
`ENOENT`, not `EACCES` - it's not there, not "there but denied").

## What's proven vs. designed

**Proven, with real evidence** (not simulated - see PROJECT.md's session log for the actual test
sequence):
- A `bwrap` sandbox exposing only one instance's uinput device can open and read real events
  injected through it (confirmed: injected `KEY_A` press+release, received `code=30 value=1` then
  `code=30 value=0` inside the sandbox).
- A sandbox scoped to instance A genuinely cannot access instance B's device (`ENOENT`).
- This daemon (`daemon.py`) actually runs, creates a real device, and correctly executes `key`,
  `mouse_move`, and `mouse_button` commands over a real Unix socket, with proper error responses
  for malformed commands. Tested directly, not just written.

**Designed, not yet built:**
- The capture side (a low-latency frame-delivery mechanism paralleling this input daemon) - not
  started. Candidate approach: a small per-instance daemon using X11's `XShm` extension for faster
  shared-memory frame readback than a naive `XComposite` pixel-by-pixel copy, exposed over a
  similar local socket (or a shared-memory ring buffer directly, for lower overhead than copying
  frames through a socket at all).
- Orchestrator wiring: `scripts/orchestrator/start-instance.sh` doesn't launch this daemon yet.
  It would need to start it inside the same sandbox as the instance's X server, before or
  alongside Sober/RevoMacro.
- Auto-detection: no mechanism yet for RevolutionMacro to discover "I'm running under
  RevolutionOS, here's my instance's socket path" - probably an environment variable
  (`REVOLUTIONOS_INPUT_SOCKET=/run/revolutionos/instances/<name>/input.sock`) set by
  `start-instance.sh` before launching RevoMacro, which it could check for and prefer over its
  default backends if present.

## A real permissions gotcha worth knowing

The uinput device node ends up owned `root:input` regardless of which uid calls `UI_DEV_CREATE` -
confirmed on the dev machine, not assumed. Outside a sandbox this isn't visible day-to-day because
`systemd-logind` grants a dynamic per-session ACL to the active seat's user (the same mechanism
that lets an unprivileged desktop user use their own keyboard at all) - but that ACL does not
carry into a `bwrap` sandbox's fresh device namespace even when the bind-mounted node is the exact
same host inode. Inside the sandbox the node shows up owned by an unmapped uid (`65534`), and a
normal unprivileged read fails with `EACCES`.

**Fix used here:** since the process creating the device is the one that should also be granted
elevated setup privileges anyway (it's setting up an entire isolated instance - X server,
metacity, sandboxed environment), just explicitly `chmod`/chown the device node right after
creation, before anything tries to use it from inside the sandbox. `daemon.py` attempts this
itself and warns if it can't (needs to run with appropriate privileges - in production this would
be the privileged orchestrator setting permissions during instance setup, not each unprivileged
instance daemon chmod'ing its own device).

## Protocol

Newline-delimited JSON over a Unix domain socket. One daemon instance per orchestrator instance,
socket path scoped accordingly (e.g. `/run/revolutionos/instances/<name>/input.sock`).

```
{"type": "key", "code": "KEY_A", "action": "press"}
{"type": "key", "code": "KEY_A", "action": "release"}
{"type": "key", "code": "KEY_A", "action": "tap"}
{"type": "mouse_move", "dx": 10, "dy": -5}
{"type": "mouse_button", "code": "BTN_LEFT", "action": "press"}
{"type": "mouse_button", "code": "BTN_LEFT", "action": "release"}
```
Each command gets one response line: `{"ok": true}` or `{"ok": false, "error": "..."}`.

Deliberately simple/language-agnostic - a Go client (for RevolutionMacro itself) is a Unix socket
connection and `encoding/json`, nothing more.

## Usage

```
python3 daemon.py <instance-name> <socket-path>
python3 client_example.py <socket-path>   # reference client, not required
```
