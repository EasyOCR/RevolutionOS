# precise-input: Design Document

| | |
|---|---|
| **Status** | Draft v1 (2026-10-14) |
| **Scope** | Per-instance, isolated, low-latency input injection for RevolutionOS instances |
| **Supersedes** | The "designed, not yet built" notes in `README.md` (README stays as the quick-start/evidence log) |
| **Consumers** | RevolutionMacro (Liam, optional adoption), the RevolutionOS orchestrator (`scripts/orchestrator/`) |

Legend used throughout: **[PROVEN]** = demonstrated with real evidence (see `README.md` / `PROJECT.md`),
**[BUILT]** = exists in `daemon.py`, **[DESIGN]** = proposed here, **[UNKNOWN]** = needs a spike before we commit.

---

## 1. Summary

RevolutionOS runs several Sober + RevolutionMacro pairs on one machine, each in its own X display,
`$HOME`, `$XDG_RUNTIME_DIR`, xauth cookie and network namespace. Input today goes through X11 XTest,
which is correctly scoped to each `$DISPLAY` but is neither the lowest-latency nor the most
deterministic path.

`precise-input` provides a second, optional path: a **per-instance virtual kernel input device
(uinput)** driven by a small daemon over a Unix socket, where **the device is visible only to the
instance it belongs to**. Raw uinput is normally global; the point of this design is getting
precision *and* isolation together, which only works because RevolutionOS already owns each
instance's environment.

This document defines the goals, threat model, architecture, wire protocol, lifecycle,
orchestrator integration, failure handling, test plan and rollout. It also records the open
questions that could invalidate parts of the design, most importantly §4.3 (whether Flatpak's own
`/dev` handling defeats the isolation proof) and §4.4 (whether anything inside an Xvfb/Xephyr
instance actually *consumes* the device).

## 2. Goals and non-goals

### Goals
1. **Isolation (hard requirement).** Instance A's input device must not be openable, listable or
   writable by instance B, and must not leak into the host desktop session.
2. **Precision.** Deterministic event timing and grouping: events that must arrive together arrive
   in a single `SYN_REPORT`; timed sequences are scheduled by the daemon, not by client round-trips.
3. **Low, bounded latency.** Target budget in §9; measured, not asserted.
4. **Safe by default.** A crashed or disconnected client must never leave keys/buttons stuck down.
5. **Optional and non-invasive.** Nothing else in the repo may depend on this. If the daemon is not
   running, RevolutionMacro keeps using XTest/XComposite exactly as today.
6. **Language-agnostic protocol.** A Go client is a socket plus `encoding/json`.

### Non-goals (this document)
- Frame capture (XShm / shared-memory ring). Sketched in §14 only; it gets its own design doc.
- Defending against a hostile process running as the same host user with root-equivalent powers.
  The boundary defended is *instance vs instance* and *instance vs host desktop*, matching the
  existing orchestrator threat model (§3).
- Anti-detection or evading any game's terms of service. This is an input-plumbing layer.
- Replacing XTest. It stays the default and the fallback.

## 3. Threat model and isolation requirements

| Actor | Can do | Must not be able to |
|---|---|---|
| Process in instance A's sandbox | Open A's device, connect to A's socket | See, open, or write B's device or socket; read host keyboard/mouse; inject into the host session |
| Process in instance B | (same, for B) | (same, for A) |
| Host desktop user session | Run the orchestrator | Receive injected input from any instance on the host seat |
| Compromised/buggy RevoMacro in A | Send arbitrary commands on A's socket | Exceed the daemon's allowlist, rate limit, or hold input down past its lease (§8) |

Out of scope: a root process on the host; kernel bugs; a hostile process sharing instance A's
sandbox and uid (it *is* A as far as this layer can tell).

**Existing precedent to stay consistent with:** the orchestrator isolates X11 by per-instance
display + cookie, `$HOME`, `$XDG_RUNTIME_DIR`, and a network namespace (`start-instance.sh`,
`lib.sh`). Unix-domain sockets that live on the filesystem are *not* network-namespaced, so the
input socket crosses netns boundaries fine, and must therefore be protected by filesystem
permissions/bind-mounts, not by netns.

## 4. Background, and the three hard problems

### 4.1 What is already proven [PROVEN]
- A `bwrap` sandbox given only instance A's uinput node can open it and read injected events
  (`KEY_A` press/release → `code=30 value=1/0`).
- A sandbox scoped to A cannot open B's node: `ENOENT` (absent), not `EACCES` (denied).
- `daemon.py` creates a device and executes `key`, `mouse_move`, `mouse_button` over a Unix socket
  with error responses for malformed input.
- Gotcha: the node is `root:input` regardless of creator uid, and the `systemd-logind` seat ACL does
  not carry into a bwrap `/dev`; inside it the owner shows as unmapped `65534`. Fix: the creator
  `chmod`s the node before anything enters the sandbox.

### 4.2 Problem 1: the proof used a hand-built `bwrap`, but instances launch through `flatpak run`
`start-instance.sh` launches Sober and RevoMacro with `flatpak run` (inside the instance netns).
Flatpak builds its own `/dev` for the app. Sober additionally *requires* the
`--device=input` override (`create-instance.sh` applies it per-`$HOME`). Flatpak's `input` device
permission exposes the host's input device nodes to the app wholesale, not one chosen node.

**Consequence [UNKNOWN, must be verified first]:** inside a Flatpak-launched Sober with
`--device=input`, instance B's uinput node may be visible (and, with permissive modes, openable)
to instance A. If so, the `ENOENT` guarantee from §4.1 does **not** hold on the real launch path,
and per-instance isolation of the device would be convention only.

Mitigation options (decide after Spike S1, §13):

| Option | Idea | Pros | Cons |
|---|---|---|---|
| **M1: DAC by node mode/owner** | Each instance's node gets a mode/owner no other instance's sandbox identity can satisfy (own uid, or per-instance group) | No change to how apps launch | Instances all run as the *same host user* today, so uid/gid can't discriminate without introducing per-instance users/groups; Flatpak's user-namespace uid mapping may collapse identities (cf. the `65534` finding) |
| **M2: Launch Flatpak inside an outer bwrap with a curated `/dev/input`** | Outer sandbox exposes only the instance's node; Flatpak's `--device=input` then binds only what exists | Preserves the "node is absent" guarantee | Nested user namespaces / setuid-bwrap interactions with Flatpak are fragile; must be tested, may be impossible on some hosts |
| **M3: Do not run Flatpak's apps against the device; give only the *daemon* the device, and deliver input to the app via a path that is already per-instance** | Daemon owns the uinput node in a private mount namespace; apps see no uinput at all; injection reaches the app via the instance's X server (see §4.4) | Strongest isolation, simplest story | Gives up the "raw kernel input, no X server in between" latency win; it becomes "XTest fed by a better scheduler" |
| **M4: Run each instance as its own host user** | Real uid separation, DAC works | Strong, boring, auditable | Large orchestrator change; affects `$HOME`, Flatpak per-user state, GPU access, the netns sudo dance |

**Recommendation (provisional):** try M2, fall back to M1 via per-instance groups, and treat M3 as
the guaranteed-safe floor. Do not ship the "isolated" claim in docs until S1 passes on the real
launch path.

### 4.3 Problem 2: who creates the device
Opening `/dev/uinput` and `UI_DEV_CREATE` needs privilege that the unprivileged instance does not
have, and the created node's ownership is out of the daemon's hands. So device creation and
permissioning belong to the **privileged orchestrator step** (the same place that already does
`ip netns`/`sudo`), not to a per-instance process chmod'ing its own device (the README already says
this is the production shape).

### 4.4 Problem 3: who consumes the events [UNKNOWN]
Xvfb and Xephyr do not read `/dev/input`; they have no udev hotplug, so a uinput device does
**not** move the cursor or type into an X client in those backends. Events only matter if the
*consumer* reads evdev itself. Candidates:

- **Sober** (requires `--device=input`, so it plausibly reads evdev for gamepad/keyboard/mouse).
  Whether it consumes *keyboard and mouse* from evdev, or only gamepads, is not established.
- **A real Xorg instance** with the libinput/evdev driver and a per-instance `InputDevice`/udev
  filter (would replace Xvfb/Xephyr; large change; interacts with the DRM-master findings in
  `PROJECT.md`).
- **A bridge**: a daemon that reads the instance's evdev device and replays into the instance's X
  server via XTest. Isolated, but it reintroduces the X queue, which is the thing we wanted to
  avoid.

Spike S2 (§13) answers this by instrumenting what Sober actually does with the node. The result
selects the architecture for the data path:

- Sober reads evdev kbd/mouse → **Path A** (direct, the goal of this project).
- Sober reads gamepad only → keyboard/mouse still go via XTest; uinput is a **gamepad/virtual
  controller** product (the `bss-automation` use case). Scope narrows, still valuable.
- Neither → this project reduces to the M3/bridge path, and the main claim of the README
  ("lower latency than XTest") needs re-evaluating with a measurement, not an argument.

> Because of 4.2 and 4.4, **nothing in §5–§12 should be implemented beyond what already exists
> until S1 and S2 are done.** The protocol/daemon hardening (§6–§8) is safe to do first because it
> is useful on every path.

## 5. Architecture [DESIGN]

```
 host (privileged orchestrator: start-instance.sh via sudo)
 ├─ creates uinput device(s) for instance <name>   ──►  /dev/input/eventN (host)
 ├─ sets owner/mode, records node path + sysname
 ├─ starts per-instance daemon in the instance's mount ns
 │     daemon: owns the fd to its device, listens on input.sock
 └─ launches apps (Sober, RevoMacro) with
        REVOLUTIONOS_INPUT_SOCKET=<sock>   (+ node exposed per §4.2 option)

 instance <name> sandbox
 ├─ RevoMacro ──(Unix socket, NDJSON)──► daemon ──► uinput ──► kernel evdev ──► consumer (Sober / X bridge)
 └─ falls back to XTest if env var unset, socket missing, or hello/caps fails
```

Components:

1. **Provisioner** (privileged; new function(s) in `lib.sh`, called from `start-instance.sh` /
   `stop-instance.sh`): creates the device, applies permissions, creates the socket directory,
   starts and supervises the daemon, tears everything down.
2. **Daemon** (`daemon.py` evolving; possibly reimplemented, see §11): one process per instance,
   one device set per instance, one socket per instance.
3. **Client** (any language; `client_example.py` is the Python reference): speaks the protocol,
   handles reconnects, never assumes the daemon exists.
4. **Discovery**: the environment variable `REVOLUTIONOS_INPUT_SOCKET`, set only for that
   instance's processes. Absence means "not under precise-input".

### 5.1 Paths and naming
- Socket dir: `$XDG_RUNTIME_DIR`-style **per-instance** dir, mode `0700`. Use the existing
  `instance_runtime_dir()` (already per-instance, already `0700`) rather than inventing
  `/run/revolutionos/instances/<name>/` as the README example does; it is already
  cleaned on stop and already bind-visible to that instance. Socket: `<runtime>/input.sock`.
  (The README's `/run/...` path needs a root-owned dir; reconcile when the provisioner is
  written, and prefer the runtime dir unless S1 shows it is not visible from the sandbox.)
- Device name: `revolutionos-<instance>-input` (existing), plus one suffix per device (§5.2).
- Identification: the daemon must not locate its node by scanning `evdev.list_devices()` for a
  matching *name* (current `device_path()`); two instances with the same name, a rename, or a
  stale device from a crashed daemon would confuse it. Use the device handle `UInput` already holds
  (`ui.device.path` in python-evdev, or `UI_GET_SYSNAME`) and write the resolved path to a state
  file for the provisioner.

### 5.2 Devices per instance [DESIGN]
Separate devices per function, because kernel/userspace classify a device by its capabilities and a
combined kbd+mouse+pad device confuses consumers (libinput, SDL, games):

| Device | Capabilities | Notes |
|---|---|---|
| `…-kbd` | `EV_KEY` (allowlisted keys), `EV_MSC`/`MSC_SCAN` optional, `EV_LED` no | key events |
| `…-mouse` | `EV_REL` (`REL_X/Y/WHEEL/HWHEEL`), `BTN_LEFT/RIGHT/MIDDLE/SIDE/EXTRA` | relative motion |
| `…-abs` (optional) | `EV_ABS` `ABS_X/Y` with a fixed range = instance geometry, `BTN_TOUCH`/`BTN_LEFT`, `INPUT_PROP_DIRECT` | exact positioning without accumulating relative error; a pointer must be one or the other, so this is a separate opt-in device |
| `…-pad` (optional) | `ABS_X/Y/RX/RY/Z/RZ`, `BTN_SOUTH…`, hat | the `bss-automation` virtual-gamepad case |

Phase 1 keeps the current single combined device to match what was proven; split in Phase 2 once
S2 shows what the consumer needs.

## 6. Protocol v1 [DESIGN]

Transport: `AF_UNIX`, `SOCK_STREAM`, newline-delimited JSON (UTF-8). It stays human-debuggable
(`socat`) and trivial in Go. A binary framing is *not* introduced now; JSON parse cost is
microseconds and is not the dominant latency term (§9). Revisit only if measurement says otherwise.

### 6.1 Compatibility
Today's commands (`key`, `mouse_move`, `mouse_button` with `{"ok":…}` replies) remain valid and are
treated as **protocol v0**. v1 is negotiated: a client that sends `hello` gets v1 semantics; a
client that does not gets v0 behaviour unchanged.

### 6.2 Handshake
```
→ {"op":"hello","version":1,"client":"revolutionmacro/x.y","lease_ms":2000}
← {"ok":true,"version":1,"instance":"acct1","devices":["kbd","mouse"],
   "caps":{"keys":[...],"buttons":[...],"rel":["X","Y","WHEEL","HWHEEL"],"abs":null},
   "limits":{"max_events_per_sec":2000,"max_batch":64,"max_schedule_ahead_ms":5000},
   "clock":"CLOCK_MONOTONIC"}
```
`caps` lets the client choose XTest fallback *before* it needs input. `lease_ms` is §8's
dead-man timer.

### 6.3 Commands
All commands accept an optional `"id"` (any JSON scalar) echoed in the reply, so clients can
pipeline. Names accept the evdev constant name (`"KEY_A"`) only; **numeric codes are rejected** so
the allowlist is the single source of truth.

| `op` | Fields | Effect |
|---|---|---|
| `key` | `code`, `action`: `press`\|`release`\|`tap`, optional `hold_ms` for `tap` | one `SYN_REPORT` per press/release; `tap` with `hold_ms` schedules the release in the daemon |
| `mouse_move` | `dx`, `dy` (int, clamped to ±`max_rel`) | one `SYN_REPORT` carrying both axes |
| `scroll` | `dy`, `dx` (detents) | `REL_WHEEL`/`REL_HWHEEL` |
| `mouse_button` | `code`, `action`: `press`\|`release`\|`click` | **strict**: any other action is an error (today anything not `"press"` silently becomes a release) |
| `abs_move` | `x`, `y` | needs the `abs` device |
| `batch` | `events`: ordered list of the above, optional `atomic:true` | all events written, **one** `SYN_REPORT` (e.g. Shift+Click, diagonal move, chord) |
| `schedule` | `at_ns` (daemon monotonic) or `in_ms`, plus a `batch`/command | executed by the daemon's timer thread; see §7 |
| `release_all` | – | release every key/button currently held |
| `state` | – | currently-held keys/buttons, queue depth, counters |
| `bye` | – | clean close; releases held inputs |

Replies: `{"ok":true,"id":…}` or `{"ok":false,"id":…,"error":"<code>","detail":"…"}` with stable
`error` codes (`bad_json`, `unknown_op`, `unknown_code`, `not_allowed`, `rate_limited`,
`bad_arg`, `device_gone`, `too_far_ahead`). **Reply modes:** `"ack":"none"` on a command (or
per connection via `hello`) suppresses the reply for fire-and-forget streams; errors are still
sent. This removes the per-command round trip, which the current client pays on every call.

### 6.4 Example
```
{"op":"batch","atomic":true,"events":[
  {"op":"key","code":"KEY_LEFTSHIFT","action":"press"},
  {"op":"mouse_button","code":"BTN_LEFT","action":"press"}]}
{"op":"schedule","in_ms":40,"cmd":{"op":"release_all"}}
```

## 7. Precision and timing [DESIGN]

What "precise" concretely means here, and what the daemon owns:

1. **Atomic grouping.** Events in a `batch` share one `SYN_REPORT`. Consumers see them as one
   input frame. (Today every press/release and every move is its own frame, so a "diagonal move"
   is already one frame, but chords are impossible.)
2. **Daemon-side timing.** Sequences like "press, hold 35 ms, release" or "move 10 px every 8 ms
   for 200 ms" are scheduled in the daemon with a monotonic-clock timer thread, not by the client
   sleeping between round trips (which adds scheduler jitter plus a socket round trip per step).
   Use `time.monotonic_ns()` and absolute deadlines (`clock_nanosleep`-style: sleep until
   `deadline - spin_margin`, then spin the last ~100–200 µs). Python's GIL makes this
   best-effort; see §11 for when to port.
3. **Sub-frame ordering.** The scheduler is a single priority queue keyed by deadline, with a
   stable tiebreaker (enqueue order). Writes to the device happen on one thread; sockets only enqueue.
4. **Honest limits.** The kernel stamps events at write time, but the *consumer* still polls at its
   own rate (Sober's input loop, display frame cadence). Daemon precision bounds our jitter; it
   cannot beat the consumer's sampling. Docs must say so.
5. **Relative-motion drift.** Relative moves lose precision through any pointer acceleration the
   consumer applies. Offer `abs_move` (§5.2) for exact targets, and document that `mouse_move` is
   subject to the consumer's pointer profile.

## 8. Safety and robustness [DESIGN]

Current daemon issues found on review of `daemon.py` (all verified by reading the code; each is
addressed by the item beside it):

| Issue in current code | Risk | Fix |
|---|---|---|
| `socketserver.UnixStreamServer` is single-threaded | A second client blocks until the first disconnects; one stuck client wedges input | Threaded server, or an `asyncio`/selector loop with the single device-writer thread from §7 |
| No tracking of held keys | Client crash/`kill -9` after `press` leaves the key logically down in the kernel until something releases it | Track pressed set per connection; on disconnect, lease expiry or `bye`, emit releases |
| `getattr(e, cmd["code"])` | Accepts any `ecodes` attribute (`"EV_KEY"`, `"SYN_REPORT"`, …); kernel gets nonsense or a non-key value | Strict allowlist dict `name → code` built from `KEY_CAPS`/`BTN_CAPS` |
| `value = 1 if action == "press" else 0` for buttons | Typo (`"pres"`) silently releases a button | Validate `action` ∈ {press, release, click} |
| `int(cmd["dx"])` unbounded | Huge deltas, non-numeric (`"dx":"1e9"`), `bool`/float coercion | Type-check, clamp to ±`max_rel`, reject non-ints |
| `socket` created then `chmod 0660` | Window between `bind()` and `chmod()` where the socket has umask-derived perms | Set `umask(0o077)` around bind, or bind inside a `0700` directory (§5.1) and rely on the directory |
| No peer check | Anything that can reach the socket path can inject | `SO_PEERCRED`: require expected uid (and ideally the instance's launch pid-tree/cgroup if available) |
| No rate or size limits | A runaway client floods the device; huge JSON lines exhaust memory | Per-connection token bucket, max line length (e.g. 64 KiB), `max_batch` |
| `os.remove(socket_path)` on start | Stomps a live daemon's socket if started twice | Take a `flock` on a lockfile; refuse to start if held |
| `device_path()` scans by name | Wrong device with duplicate names | Use the handle (§5.1) |
| Device chmod only warns on failure | Silent later `EACCES` in sandbox | Make it a startup error when running in provisioned mode (§10) |

Additional design rules:
- **Dead-man lease.** Client declares `lease_ms` in `hello` and must send *any* command (a `ping`
  is fine) within it while anything is held; on expiry the daemon releases everything and reports
  `lease_expired`. Default off for the first connection to preserve v0 behaviour, on for v1.
- **Kill switch.** The orchestrator can `SIGUSR1` the daemon (or send `release_all` over a
  privileged control socket, §10) to flush held inputs before stopping an instance.
- **Allowlist is policy.** The key allowlist deliberately excludes system keys
  (`KEY_POWER`, `KEY_SLEEP`, `KEY_VOLUME*`, `KEY_SYSRQ`, VT-switch combos) so a bad client cannot
  trigger host actions *if* the device ever became visible to the host seat (see 4.2).
- **Seat isolation.** The device must be tagged so `systemd-logind` / udev does **not** assign it
  to the host seat (`ID_SEAT=` something other than `seat0`, or `ENV{LIBINPUT_IGNORE_DEVICE}="1"`
  / `ID_INPUT=` cleared via a udev rule matching `revolutionos-*-input`). Otherwise the host
  desktop's libinput will happily treat our virtual keyboard as a real one and type into the
  user's session. **[UNKNOWN until tested]**: whether the dev machine's session currently does this;
  the evidence in `README.md` is of the sandbox side only.

## 9. Latency and performance

### 9.1 Budget (targets, to be validated by §12 L1–L3)
| Stage | Target (p50 / p99) | Notes |
|---|---|---|
| client write → daemon read | < 100 µs / < 500 µs | local Unix socket |
| parse + validate + enqueue | < 100 µs / < 300 µs | JSON in Python; port if exceeded |
| scheduled deadline error | < 500 µs / < 2 ms | Python timer thread with spin margin |
| `write()+SYN` to uinput | < 50 µs / < 200 µs | kernel path |
| kernel → consumer wakeup | consumer-dependent | measured, not controlled |
| **Daemon-controlled total** | **< 1 ms / < 3 ms** | |

### 9.2 What we will not claim
We will not publish a "faster than XTest" number until L3 compares both paths on the *same
consumer* (§12). The README's "lower latency" is a hypothesis about the X server queue, not a
measurement.

### 9.3 Capacity
N instances ⇒ N daemons ⇒ N×(1–4) uinput devices. The kernel has a finite input-device /
`/dev/input/eventN` namespace (minor numbers) and each device adds an `input_dev` worth of memory
and an event handler; the limit is high, but long-lived churn with leaked devices (crashed
daemons) is the realistic risk. §10 mandates cleanup and a startup reaper.

## 10. Orchestrator integration [DESIGN]

### 10.1 Start (`start-instance.sh`)
New steps, between X server readiness and launching Sober (matches the README's "before or
alongside Sober/RevoMacro"):

1. `provision_input "$NAME"` (privileged):
   - reap leftovers: any `revolutionos-$NAME-*` input device whose daemon pid file is dead
     (devices vanish when the creating fd closes, so this mainly covers a *hung* daemon).
   - start daemon, wait for its ready line / socket, read back resolved node path(s).
   - apply owner/mode per the chosen mitigation (§4.2); fix seat tagging (§8).
2. Export `REVOLUTIONOS_INPUT_SOCKET` in the `netns_exec_as_user … env …` line for **RevoMacro
   only** (Sober doesn't need it; fewer processes holding the path is less exposure).
3. Add the daemon's PID to `PIDS` so the existing `cleanup_on_failure` trap and `pids` file cover
   it.
4. If provisioning fails: **log and continue without** `REVOLUTIONOS_INPUT_SOCKET`. This feature
   is optional by contract (goal 5); an instance must still start with XTest.

### 10.2 Stop (`stop-instance.sh`)
Before killing the X server: send `release_all`, then `SIGTERM` the daemon; wait for the socket and
the device to disappear; kill on timeout. Verify the node is gone (`ls`), since a leaked node is
exactly the cross-instance hazard §4.2 worries about.

### 10.3 Privilege model
Device creation needs `/dev/uinput` access. Options, in order of preference:
1. **One tiny privileged helper** (`revolutionos-uinput-create`) invoked via the same `sudo`
   pattern the orchestrator already uses for `ip netns`; it creates the device, sets permissions,
   and hands the fd/daemon to an unprivileged process. Least privilege, matches existing practice.
2. A udev rule granting a dedicated group `rw` on `/dev/uinput` (removes sudo, widens who on the
   host can create *global* input devices, so it weakens the story).
3. Run the whole daemon as root (what the code effectively assumes today); simplest, largest blast
   radius. Not recommended beyond the prototype.

### 10.4 Image / packaging
`image/` builds a Bazzite-based image. Ship: `python3-evdev` (or a static Go/C daemon, §11), the
udev rule, `uinput` kernel module autoload (`modules-load.d`), and the helper. Keep it out of the
image until S1/S2 pass so we don't ship an unvalidated isolation claim.

### 10.5 Virtualization
Many users will run RevolutionOS in a Proxmox VM with GPU passthrough. `uinput` and mount
namespaces are guest-kernel features and should be transparent, but this has **not** been tested
in a VM (PROJECT.md, 2026-10-11). Required validation: S1, S2 and L1 repeated inside a guest.

## 11. Implementation language and structure

Phase 1–2 stay in Python (`evdev`): the existing code and the proof are Python, iteration is fast.
Port the daemon to a small Go or C program if L2 shows the Python timer thread missing its p99
targets (GIL pauses, GC), or if shipping a Python+evdev dependency in the image is unwelcome.
Go is attractive because the primary client (RevolutionMacro) is Go and can share the protocol
types. The protocol (§6) is the stable contract; the daemon is replaceable behind it.

Proposed layout:
```
precise-input/
  README.md          # quick-start + evidence log (exists)
  DESIGN.md          # this file
  PROTOCOL.md        # normative wire spec extracted from §6 once v1 stabilises
  daemon.py          # evolves per §6-§8
  client_example.py  # reference client; gains hello/batch/schedule helpers
  tests/             # §12 (pytest; hardware-free where possible)
  tools/latency.py   # §12 L-series harness
```

## 12. Test plan

### Unit (no hardware; mock `UInput`)
- Protocol: malformed JSON, oversize line, unknown `op`/`code`, numeric codes rejected, bad `action`,
  non-int and out-of-range `dx/dy`, `id` echo, `ack:none`.
- State: press then disconnect ⇒ release emitted; `release_all`; lease expiry; `tap` with `hold_ms`.
- Scheduling: ordering with equal deadlines; deadline error bounded under load (mock clock).
- Batch: all events precede exactly one `SYN_REPORT` (assert on the mock's write log).

### Integration (needs `/dev/uinput`, root; CI-skip if absent)
- I1: create device, read it back via `evdev.InputDevice`, verify event sequence and `SYN` grouping.
- I2: two daemons, two sockets; commands to one produce events only on its own device.
- I3: kill -9 a client holding `KEY_A`; assert a release is observed on the device.
- I4: second concurrent client is served while the first is idle (regression for the single-threaded
  server).
- I5: socket permissions are never wider than intended at any moment (stat in a tight loop during bind).

### Isolation (the claims we want to be able to repeat)
- **S1** (gate for the whole project): on the **real** launch path (`flatpak run` with the
  `--device=input` override, instance netns, per-instance `$HOME`), from inside instance A's
  Sober sandbox attempt to `open()`/`stat()`/list B's node and socket; record `ENOENT` vs
  `EACCES` vs success. Repeat for each mitigation in §4.2. Pass = A cannot read or write B's device.
- **S3**: host-session leak test: with the daemon running, assert the host desktop receives no key
  events from injected input (`libinput debug-events` on the host seat while injecting).
- **S4**: negative control: from A, connect to B's socket path ⇒ fails.

### Consumer
- **S2** (gate for the data path): `strace -f -e trace=openat,read,ioctl` (or `bpftrace`, which the
  project already used for Sober) on Sober/RevoMacro to see whether they `open()` and `read()` the
  per-instance node, and what they do with kbd/mouse/gamepad events. Then inject and observe an
  in-game effect with a known game action.

### Latency
- **L1**: client→daemon→`write()` timing (daemon-side timestamps, µs resolution).
- **L2**: scheduled-event deadline error distribution under idle and CPU-loaded conditions.
- **L3**: end-to-end, same consumer, XTest vs uinput, using a sink that timestamps arrival
  (the repo's `testing/synthetic-client` is the natural place to extend if the consumer can be an X
  client; otherwise a purpose-built evdev reader), ≥10 000 samples, report p50/p95/p99.

### Soak
- 8 h, N instances, periodic client kills; assert zero stuck keys, zero leaked devices
  (`/proc/bus/input/devices` before/after), stable daemon RSS.

## 13. Rollout plan

| Phase | Deliverable | Exit criterion |
|---|---|---|
| **0: Spikes** | S1 (flatpak `/dev` visibility), S2 (what Sober reads), seat-leak check | Written results appended to `README.md` / `PROJECT.md`; go/no-go and mitigation choice recorded |
| **1: Harden daemon** | §8 fixes, strict allowlist, threaded server, held-key tracking, unit + I-series tests | I1–I5 pass; behaviour for v0 clients unchanged |
| **2: Protocol v1** | `hello`/caps, `batch`, `schedule`, `scroll`, `ack`, lease; `PROTOCOL.md` | Reference client exercises every op; L1–L2 within §9 budget |
| **3: Orchestrator wiring** | §10 provisioner, env var, stop-time cleanup, failure-is-nonfatal | Two instances start/stop 50× with zero leaked devices; S1/S3/S4 pass on the real path |
| **4: Split devices / absolute / pad** | §5.2 devices as S2 dictates | Feature-specific tests |
| **5: Packaging + VM validation** | Image changes (§10.4), Proxmox guest runs of S1/S2/L1 | Passes inside a VM |
| **6: Hand-off** | Liam integrates a client (Go), A/B measurement L3, decision to make it default or not | Measured benefit or explicit "not worth it" |

Phases 1–2 are valuable regardless of the S1/S2 outcome and can proceed in parallel with Phase 0.

## 14. Capture side (out of scope, placeholder)
README's candidate: a per-instance `XShm` frame reader exposed via shared memory. Dependencies
to resolve first: the DRM-master / concurrent-instance rendering problem documented in
`PROJECT.md` (2026-10-11) determines whether more than one instance reliably presents frames at
all; capture design should not start before that is settled. Keep it a separate document.

## 15. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Flatpak `--device=input` exposes all input nodes (§4.2) | Medium–High | Breaks the central isolation claim | S1 first; M2/M1/M3 ladder; don't claim isolation until proven |
| Nothing in the instance consumes kbd/mouse from evdev (§4.4) | Medium | Project reduces to gamepad/bridge | S2 first; scope accordingly |
| Virtual keyboard leaks into the host seat | Medium | Injected keys type into the user's real session | udev tagging + S3 |
| Python timer jitter misses budget | Medium | Imprecise scheduling | Spin margin; port to Go/C behind same protocol |
| Leaked/stuck devices or keys after crashes | Medium | Stuck input, leaked nodes | Held-key tracking, lease, reaper, soak test |
| Privileged helper widens attack surface | Low–Med | Local privilege escalation | Minimal helper, fixed argv, no shell, audited |
| Behaviour differs inside a VM | Low–Med | Late surprise | Phase 5 gate |
| Over-promising ("faster than XTest") | Medium | Credibility | §9.2: no claim without L3 data |

## 16. Open questions
1. S1 outcome: which of M1–M4 works on the real Flatpak launch path?
2. S2 outcome: what does Sober read from evdev (keys, mouse, pad)? Does it use the node
   independent of window focus, as the README assumes?
3. Is an absolute-pointer device needed by RevolutionMacro's click targeting, or is relative motion
   sufficient with the game's pointer profile?
4. Does RevolutionMacro want protocol v1's scheduling, or will it do timing itself? (Ask Liam
   before building `schedule`; it is the most expensive feature to get right.)
5. Where should the socket live (instance runtime dir vs `/run/revolutionos/…`) given what the
   sandbox can see?
6. Is a per-instance host user (M4) acceptable for the broader orchestrator, given it would also
   simplify other isolation (GPU, DRM, files)?

## Appendix A: current implementation vs this design

| Area | `daemon.py` today | Design |
|---|---|---|
| Server | `UnixStreamServer`, one client at a time | concurrent clients, single device-writer thread |
| Commands | `key`, `mouse_move`, `mouse_button` | + `scroll`, `abs_move`, `batch`, `schedule`, `release_all`, `state`, `hello`, `bye` |
| Validation | `getattr(ecodes, …)` | strict allowlist, typed/clamped args |
| Held-input safety | none | per-connection tracking, lease, release on disconnect |
| Replies | always one line | optional `ack:none`, stable error codes, `id` echo |
| Device | one combined kbd+mouse | split devices (Phase 4) |
| Permissions | best-effort `chmod` warning | provisioner-owned, hard failure in provisioned mode |
| Discovery | none | `REVOLUTIONOS_INPUT_SOCKET` |
| Peer auth | file mode only | `SO_PEERCRED` + dir mode |

## Appendix B: glossary
- **uinput**: kernel facility for userspace to create virtual input devices (`/dev/uinput`).
- **evdev**: the kernel's per-device input event interface (`/dev/input/eventN`).
- **SYN_REPORT**: marker that ends one atomic group of events.
- **XTest**: X11 extension for synthetic input, scoped to one X server.
- **DAC**: discretionary access control (file owner/mode).
