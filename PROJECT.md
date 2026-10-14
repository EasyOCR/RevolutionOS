# RevoOS (working name)

A full, usable, redistributable Linux desktop OS built specifically around running
[Revolution Macro](https://github.com/nosyliam/revolution) (a Bee Swarm Simulator automation tool
for Roblox) — including native support for running multiple concurrent macro sessions
(multiple accounts) on one machine, which the stock app can't do (it supports configuring
multiple account presets, but only runs one at a time). Committed to Liam (RevoMacro's developer)
as a real deliverable, not a personal hack.

## Status (2026-09-24)

**Done:**
- Base decided: `ghcr.io/ublue-os/bazzite-nvidia-open:stable`, X11/GNOME Flashback+Metacity (not
  Wayland/cage), no VMs/vGPU for isolation. All confirmed via real testing, not assumption.
- Orchestrator (`scripts/orchestrator/` + `ui/`): bash scripts for per-instance isolated X11
  display + fake `$HOME` (flatpak data isolation) + metacity; Wails v2/React/Mantine GUI wrapping
  them, styled to match RevoMacro's real UI (confirmed via extracting its embedded CSS - 2500+
  Mantine token references, blue primary, dark default). Builds clean, renders correctly.
- OS image (`image/`): Containerfile (multi-stage - builds the orchestrator from source, installs
  Sober + RevoMacro + gnome-flashback), CI workflow, ISO config - all matching real current
  ublue-os/image-template conventions. Two real bugs caught and fixed: orchestrator wasn't
  actually wired into the image at all, and the CI workflow was in a path GitHub Actions never
  reads.
- Panel/shell (`panel/`): proved a real Wails-rendered window can claim GNOME Flashback's panel
  role - dock type + strut screen-space reservation via raw EWMH X11 calls (Wails has no native
  support for this) - verified with real `_NET_WORKAREA` measurements, not simulated.

**Not done - remaining for a finished product:**
- Orchestrator: **update (2026-09-25)** - the per-instance X server + window manager mechanism
  itself is now confirmed working end-to-end with synthetic clients (two real bugs found and
  fixed, see "Synthetic isolation stress test" below) and window/display-level isolation between
  instances is confirmed, not assumed. Still never tested with 2+ real concurrent account logins
  (explicitly deferred) - that's the only remaining gap here, not the mechanism itself anymore.
  Never measured real per-instance GPU/VRAM cost, so the actual concurrency ceiling is unknown.
- Panel: **update (2026-09-25) - Applications menu, Places menu, window list/taskbar, System menu,
  and a real system tray are all now built**, see "Real panel content" and "System menu + tray"
  below. Restyled to match a real RevolutionMacro screenshot (light theme, gray active-state pill,
  blue reserved for action buttons) rather than the earlier CSS-token-only guess. Still not
  attempted: a Places-style visual redesign pass beyond color matching (icons, exact spacing) -
  waiting on Liam's actual Wails source for that, per the original plan.
- Image: **update (2026-09-25) - locally built and boot-tested for the first time**, see "Local
  image build + boot test" below. Still no CI run ever triggered (local build only, deliberately -
  see boundaries), no ISO generated (qcow2 only so far), only tested in a QEMU/libvirt VM, not real
  hardware. RevolutionMacro's flatpak bundle still isn't redistributable (needs a real repo from
  Liam). GDM default-session-selection **implemented (2026-09-25), not yet end-to-end tested** -
  see "GDM default session" below. Cosign signing referenced in CI but no keypair actually
  generated/stored yet.
- Branding/polish: explicitly last priority, not started - name is still "working name," no icons/
  wallpaper/first-run experience.
- Redistribution readiness: only tested on this one dev machine/GPU. Needs validation across
  different NVIDIA generations/driver versions, plus real end-user-facing docs (distinct from this
  file, which is our internal working doc).
- **Sign-in/identity - not started, real requirement (2026-09-25):** the finished product should
  not use traditional local Linux accounts at all - users sign into the desktop with Discord or
  Patreon (whichever ties to their existing RevolutionMacro account/subscription), not a
  username+password created at install time. This is a bigger feature than it sounds: GDM's
  greeter would need to be replaced or extended to do an OAuth flow instead of a password prompt
  (possibly another Wails-rendered surface, matching the panel's approach - worth considering once
  the panel itself is further along), then map the result to a real local session/user under the
  hood since PAM/systemd-logind ultimately still need *something* local to start a session for.
  Not designed yet, just captured here so it doesn't get lost. Today's test VM uses a throwaway
  local account (`revuser`) created purely to unblock validating the panel mechanism - that's
  scaffolding, not a preview of the real sign-in design.

### Local image build + boot test (2026-09-25)

First time this has actually happened, not just scaffolded. Built `localhost/revolutionos:latest`
locally (rootful podman, `image/Containerfile`), generated a qcow2 via `bootc-image-builder
--type qcow2 --rootfs btrfs --local`, booted it in a real QEMU/libvirt VM, and watched it via
`virt-viewer`. Real bugs this caught that would never have surfaced without an actual build:

- `build.sh` had two `/ctx/`-prefix bugs (`cp -avf /orchestrator-src/.` and the RevolutionMacro
  bundle check) that silently no-op'd instead of erroring - the ctx bind-mount puts everything
  under `/ctx/`, and only the `system_files` line had the prefix right. Fixed both.
- The base image tag was wrong: `bazzite-nvidia-open:stable` (no `-gnome-`) is Bazzite's KDE
  default, not the GNOME variant - confirmed by comparing image digests against this dev machine's
  own `rpm-ostree status`, which runs `bazzite-gnome-nvidia-open` and is the exact environment
  Flashback has been reliable on. Switched to the correct tag.
- `panel/`'s width was hardcoded to `5120` (this dev machine's own display) - would have shipped a
  comically oversized/mis-measured panel on any other resolution. Now detects the real screen width
  at startup via a throwaway X11 connection before the Wails window is created.
- `bootc-image-builder` needs `--rootfs` set explicitly - the base image (confirmed on both our
  build and the raw upstream image) ships no `/usr/lib/bootc/install/*.toml` filesystem-default
  metadata at all. Picked `btrfs` based on real evidence (this dev machine's own `/var` partition),
  not a guess.
- **The image ships no user account at all** - no kickstart, no cloud-init, nothing in `build.sh`
  creates one, so a fresh boot hits GDM with nothing to log into. Confirmed via `/etc/passwd`
  inspection and a real `virsh screenshot`. Worked around for this test by shutting the VM down,
  connecting its disk via `qemu-nbd`, chrooting into the active ostree deployment
  (`/ostree/deploy/default/deploy/<checksum>.1` - resolved via the `/ostree/boot.0/...` symlink,
  *not* assumed from directory listing order, since two deployment serials existed) and creating
  one directly. This is a real gap for the shipped product, superseded by the sign-in requirement
  above - not something to solve with a baked-in default account.
- Panel mechanism itself (masking `gnome-panel.service`, autostarting `revolutionos-panel.service`
  via a session drop-in) built and deployed into the image correctly, confirmed via
  `systemctl --global mask` output during the build. **Not yet confirmed rendering post-login** -
  that requires actually logging into the test account and observing the session, which is the
  next thing to check.

### Synthetic isolation stress test (2026-09-25) - found and fixed two real bugs, isolation confirmed

Set out to verify orchestrator instance isolation with synthetic clients (no real Sober/
RevoMacro, no real accounts - see `scripts/orchestrator/test-synthetic-isolation.sh` and the
reusable `scripts/orchestrator/testing/synthetic-client/` test window, built specifically because
this host has none of xterm/xclock/xeyes/xdotool-createwindow available). First pass found
something more fundamental than an isolation gap - **the per-instance window manager didn't start
at all, on either backend.** Both root-caused and fixed, both now applied in `start-instance.sh`:

- **`Xvfb` (the default backend, `lib.sh`'s `XVFB_BIN`, Homebrew-installed) failed outright** -
  `XKB: Failed to compile keymap`. Cause: this Xvfb build ships no xkeyboard-config data of its
  own (confirmed: no `share/X11/xkb` anywhere under its Homebrew prefix). **Fix: `-xkbdir
  /usr/share/X11/xkb`**, pointing it at the system's copy instead.
- **`metacity --replace` failed to connect to any freshly-spawned X server** (Xvfb or Xephyr) -
  `metacity-CRITICAL: Unable to open X display`, despite other clients (raw `xgb`, `xwininfo`,
  `wmctrl`) connecting to the same displays fine. Ruled out a startup race, SELinux (checked
  `ausearch`/`journalctl`, nothing), an arg-parsing quirk (`--display=` vs env var), and
  `XAUTHORITY`/session-manager involvement (`--sm-disable` made no difference) before finding the
  real cause: this shell's environment has `WAYLAND_DISPLAY` set (inherited from the host's own
  Wayland session), which makes GTK3 auto-detect and try the Wayland backend by default - metacity
  is X11-only, so that mismatch produced a misleading error that had nothing to do with the actual
  X11 connection. **Fix: `GDK_BACKEND=x11`** forces the correct backend regardless of what's
  inherited. Doesn't affect the real production Flashback session (X11-only by construction,
  `WAYLAND_DISPLAY` is never set there) - this was specifically a side effect of spawning nested
  test displays from a dev shell that itself has a Wayland session.
- **Re-ran the full test with both fixes applied - complete pass, all checks green**, including
  the one that was blocked before: instance A's display shows only A's window, instance B's shows
  only B's, confirmed via real `wmctrl` output (not assumed). HOME data isolation also still
  confirmed. `start-instance.sh` and the test harness both now use the fixed, real default (Xvfb),
  not the Xephyr workaround.
- Worth keeping in mind for whenever real concurrent accounts get tested: a quick check with
  `wmctrl` on these displays showed no `xauth`-based access control is configured on them - any
  local process that merely sets `$DISPLAY` to another instance's display can connect to it fine.
  Isolation here is **by convention** (nothing tells a process to connect elsewhere), not
  OS-enforced the way the `precise-input` bwrap/uinput isolation is. Fine given we control what
  launches into each display, but not a security boundary against a hostile process running as the
  same local user.
- **Follow-up bug found (2026-09-25) in the test script itself, not the orchestrator**: every run
  of this test was orphaning its Xvfb/metacity/synthetic-client processes instead of cleaning them
  up, discovered when `ps aux` turned up processes from three separate test runs still alive.
  Cause: `DISP_A="$(start_synthetic_instance ...)"` - command substitution always forks a subshell,
  so every `PIDS+=()` inside that function was mutating only the subshell's throwaway copy of the
  array, never the real one `cleanup()` reads from. `cleanup()` had been finding ~0 real PIDs to
  kill on every single run. Fixed by having the function set a global variable directly instead of
  echoing a value through `$(...)`. Re-ran twice after the fix and confirmed `cleanup()` now
  reports the correct count (6 - two instances × Xvfb/metacity/client each) and no new orphans are
  left behind.

### Real panel content (2026-09-25): Applications menu, Places menu, window list

Following the "prove the stock mechanism before customizing" plan, not a visual redesign - still
plain dark bar styling, no icons. `panel/applications.go`, `panel/places.go`, `panel/windows.go`:

- **Applications menu**: real `.desktop` file enumeration (`/usr/share/applications` +
  `~/.local/share/applications`), a from-scratch minimal parser (only reads `Name=`/`Exec=`/
  `Type=`/`Categories=`/`NoDisplay=`/`Hidden=` from `[Desktop Entry]`, ignores localized
  `Name[xx]=` variants and `[Desktop Action ...]` subsections - doesn't need locale support to be a
  real working menu), field-code stripping (`%f`/`%U`/etc.), and real launching via `os/exec`.
  **Verified with a real Go test against actual system files** (not mocked): found 23 real
  applications with correct Name/Exec/Categories parsing.
- **Window list/taskbar**: reads the live `_NET_CLIENT_LIST` (reusing the xgb connection pattern
  `claimAsDock` already established), filters out DOCK-type windows so the panel naturally excludes
  itself without hardcoding a self-match, activates windows via a real `_NET_ACTIVE_WINDOW` client
  message (the same mechanism any EWMH-aware taskbar/pager uses). **Fully verified (2026-09-25,
  follow-up)**: `panel/windows_test.go` now builds the synthetic-client test binary fresh, starts a
  real headless Xvfb + metacity (with both real fixes from the isolation stress test applied), maps
  a real window, and confirms `ListWindows` finds it with the correct title and `ActivateWindow`
  doesn't error - run twice back-to-back with `-count=1` to rule out Go's test cache masking a
  repeat run. No longer just "connects without erroring" - this is the real behavior, confirmed.
- **Places menu**: real XDG user-dirs (parses `~/.config/user-dirs.dirs` the way
  `xdg-user-dirs-update` writes it, falls back to conventional `$HOME/<Name>` paths for accounts
  without that file yet - e.g. this dev machine's throwaway `revuser` test account), only lists
  entries that actually exist on disk, opens via `xdg-open`. **Verified with a real Go test**
  against this machine's actual home directory: found Home + 6 real XDG dirs, all confirmed to
  exist.
- Both frontend pieces (`panel/frontend/src/App.tsx`) built and visually iterated via a live
  Vite dev server preview (`.claude/launch.json`'s `revolutionos-panel-dev`, port 5174) with mock
  data behind a `window.go` presence check - real Applications/window data when actually running in
  Wails, realistic placeholders in a plain browser. This is now the standard workflow for panel UI
  work going forward - full container/qcow2/VM rebuilds are for verifying the native-window/EWMH
  mechanism, not for checking a layout tweak.
- System menu and system tray: initially deliberately skipped (destructive actions, separate
  substantial protocol work) - both since built for real, see "System menu + tray" below. Explicit
  user sign-off obtained first for the System menu's destructive actions specifically, scoped to
  VM-only testing.

### Design system: restyled to match a real RevolutionMacro screenshot (2026-09-25)

Previously the panel and orchestrator dashboard were styled from RevoMacro's embedded CSS *tokens*
only (extracted via `strings` on the binary - see "Reference facts" below), which suggested a dark
default theme. The user shared a real screenshot of RevolutionMacro's actual running UI, which is
ground truth and overrides that guess: **light theme**, blue accent reserved specifically for
action buttons (Create, Start, etc.), and a subtle light-gray pill for the active/selected tab
state - not blue, confirmed via a direct user correction after the first attempt used blue for
both.

- `ui/frontend`: `defaultColorScheme` switched from `dark` to `light`. Top tabs restructured into a
  real `AppShell.Header` (previously inline in `AppShell.Main`) so it reads as a distinct bar like
  the reference, `withBorder` added to both header and footer for the same reason. Tabs `variant`
  changed from `outline` (which rendered white-on-transparent active-tab text in light mode - a
  real bug, confirmed via live-preview style inspection, not assumed) to `pills` with an explicit
  `color="gray.1"` + text-color override, matching the reference's subtle gray highlight instead of
  a filled blue pill.
- `panel/frontend`: same color tokens ported over exactly (`#ffffff` background, `#f1f3f5` active
  pill, `#1a1b1e` text, `#dee2e6` border) so the panel and dashboard read as one consistent design
  language rather than two independently-styled surfaces. No blue anywhere in the panel - it has no
  equivalent of a primary action button, just menu triggers and a taskbar.
- Both verified via the live-preview workflow (colors/behavior inspected directly, not just
  visually eyeballed from a screenshot), `tsc` type-checks clean on both.

### System menu + tray (2026-09-25)

Both explicitly re-authorized by the user after the initial "not attempted" pass above, with a
specific condition on the System menu: full real functionality, but end-to-end testing of the
actual destructive actions confined to the disposable VM, never this dev host.

**System menu** (`panel/system.go`) - real actions, confirmed against actually-available tools on
this host rather than assumed generic systemd calls: `gnome-session-quit --logout/--reboot/
--power-off --no-prompt` (session-integrated, respects real GNOME session state, unlike calling
`loginctl`/`systemctl` directly) for Log Out/Restart/Shut Down, `xdg-screensaver lock` for Lock
Screen (no `gnome-screensaver-command` on this system). Frontend adds a confirm() speed bump on
Restart/Shut Down specifically. **No Go tests for this file, deliberately** - there's no safe way
to test "does this actually reboot" without doing exactly that to whatever machine runs the test.
Verified only at the frontend/UI level so far (renders, opens/closes, no console errors) - the
actual destructive actions are still pending a real VM boot-and-click test, which is the next step.

**System tray** (`panel/tray.go`) - a real, if v1-scoped, implementation of the de-facto freedesktop
tray protocol (there's no fully ratified spec; this is what apps using libappindicator/Qt actually
speak): registers `org.kde.StatusNotifierWatcher` on the session bus, tracks items real apps
register via `RegisterStatusNotifierItem` (trusting the D-Bus message's real `Sender` for identity,
since the spec is loose about what the method argument itself contains), fetches each item's real
`Title` property, activates items via a real `Activate()` call. Steps aside gracefully if another
watcher already owns the bus name rather than fighting over it (confirmed necessary: this dev
machine's own real desktop session already has `gnome-shell` hosting one).

Deliberately not attempted in v1: `IconPixmap` (real icon rendering - shows item titles as text for
now, same pragmatic approach as the window taskbar) and `ContextMenu` (the separate `dbusmenu`
sub-protocol).

**Verified for real**, not just compiled: `panel/tray_test.go` launches a genuinely isolated D-Bus
session bus via `dbus-launch` rather than testing against this host's real one (which would only
exercise the "already owned" fallback path, since `gnome-shell` holds the name there). Registers a
synthetic fake tray item using the real `org.freedesktop.DBus.Properties` machinery and confirms
the watcher correctly tracks it with its real fetched title, and that `Activate` genuinely reaches
the fake item's own handler - not mocked at any layer.

### GDM default session (2026-09-25) - SUPERSEDED, see "greetd replaces GDM" below (2026-09-27)

The AccountsService override this section describes was removed entirely once GDM itself was
dropped (see below) - nothing reads it anymore, since the replacement greeter always launches
Flashback directly rather than consulting a per-user session preference. Left here for history.

Real mechanism, confirmed by direct inspection rather than assumed: `accounts-daemon` seeds a
brand-new user's session preference from `/usr/share/accountsservice/user-templates/{standard,
administrator}` at account-creation time - both ship with `Session=` empty by default (confirmed
on the dev machine, whose own real per-user entry at `/var/lib/AccountsService/users/<user>` has
`Session=gnome`, i.e. the same key this template seeds). Overrode both templates via
`image/system_files/usr/share/accountsservice/user-templates/` to
`Session=gnome-flashback-metacity` (the `xsessions` `.desktop` basename), same override pattern
already used for the panel's systemd units.

**Not yet tested end-to-end**: confirming this actually works means creating a new user account and
checking its seeded AccountsService entry. Didn't want to create/delete a real account on this dev
machine's own user database to test it, and a fully isolated test would need a D-Bus-enabled
container running `accounts-daemon` for real - more setup than seemed proportionate for a
config-file change this well-evidenced. Worth a real check next time a VM gets rebuilt (create a
second user in the running VM, or better, check what session a first-boot-created account gets).

### GDM login investigation, closed by replacing GDM entirely (2026-09-27)

Picked back up the still-open "GDM interactive login rejects a definitively-correct account"
investigation from before. New evidence found this session, via a fresh VM boot and careful
step-by-step `virsh send-key` interaction (revealing the password field's typed content via its
reveal toggle to rule out injection issues, rather than guessing): the failure mode had actually
*changed* from earlier sessions - the account (`b`/`b`) was now recognized as real (no more "user
unknown"), and got as far as a specific "password authentication didn't work" rejection. Along the
way, tabbing through the login screen surfaced a real, previously-undetected bug: GDM's session
picker showed plain **GNOME as the pre-selected default**, not GNOME Flashback (Metacity) - despite
the AccountsService override above being exactly the mechanism meant to prevent that. Root cause
not fully chased down (plausibly the offline-chroot account creation method never triggers
`accounts-daemon` to actually seed the per-user state file the way a real `useradd`/first-login
flow would), but moot regardless of the answer - see below.

**Decision: stop debugging GDM, replace it.** Two independent reasons converged: the login
failures were proving to be a real, unresolved reliability problem across multiple sessions of
investigation ("finicky," in the user's words - not acceptable for a shipped product), and more
fundamentally, GDM's entire raison d'être is GNOME-specific session/account machinery
(AccountsService-seeded defaults, a session picker) that this product has no use for and had
already been actively fighting (this exact session picker bug is a direct symptom of that
mismatch). RevolutionOS replaces every other piece of the desktop shell with its own Wails UI
already (panel, system menu, tray) - the login screen was the one remaining GNOME-native surface.

**Replacement: `greetd` + a new `greeter/` Wails app**, not another traditional display manager
(LightDim/SDDM). greetd's whole design point is that it has zero built-in UI - it only speaks a
small IPC protocol (`greetd-ipc(7)`) and hands session-launching to whatever greeter you configure,
which is exactly the "everything is our own Wails surface" pattern already established. Confirmed
available in Fedora's own repos (`greetd-0.10.3-6.fc44`, no COPR needed), and its package already
ships everything needed except the greeter itself: a `greetd` system user via its own
`sysusers.d` fragment (not something RevolutionOS creates), working PAM stacks for both the real
user login (`/etc/pam.d/greetd`, standard `system-auth`) and the greeter process's own
no-password startup (`/etc/pam.d/greetd-greeter`), and the `greetd.service` unit (aliased to
`display-manager.service`, the same mechanism GDM used - `dnf5 install greetd` plus enabling it is
the whole integration point).

**Protocol implementation** (`greeter/greetd.go`) - verified against the real `greetd-ipc(7)`
manpage spec rather than assumed from memory (a wrong binary-framing detail here would just hang on
read, not fail loudly): 4-byte little-endian length prefix + UTF-8 JSON, snake_case message types
(`create_session`, `post_auth_message_response`, `start_session`, `cancel_session` /
`success`, `error`, `auth_message`). **Tested for real** (`greeter/greetd_test.go`): an
independent fake greetd implementation (its own framing code, not a reuse of the client's encoder)
drives a full realistic exchange - create_session, answer a secret auth prompt, start_session with
the real Flashback launch command - over an actual Unix socket, plus a rejected-login path. Both
pass.

**Login flow** (`greeter/app.go`): dials a fresh greetd connection per attempt (simpler than
tracking greetd's session-cancellation semantics across retries), answers auth prompts with the
given password, and on success calls `StartSession` with `gnome-session
--session=gnome-flashback-metacity` directly - no session picker exists at all now, so there's
nothing left to mis-default the way GDM did. Session environment explicitly sets `GDK_BACKEND=x11`
alongside the `XDG_SESSION_*`/`DESKTOP_SESSION` vars - the same stray-`WAYLAND_DISPLAY`-breaks-GTK3
bug class already root-caused once this project (`start-instance.sh`'s fix), applied defensively
here too rather than assuming greetd's environment is always clean.

**UI** (`greeter/frontend/`) - a single-step Mantine form (username + password together, no
GDM-style two-step username-then-password navigation, which removes a whole class of "which step
am I on" state to get wrong) matching the light theme established for the panel and orchestrator
dashboard. Live-previewed and interaction-tested via `.claude/launch.json`'s
`revolutionos-greeter-dev` entry before ever touching the real build - both the rejected-login path
(error alert, password field cleared, username retained) and the successful-login path (mock
`StartSession` call logged) verified in-browser via a mocked `Login` call, matching the same
`IN_WAILS` preview-mode pattern already used in `panel/frontend`.

**Also switched the base image** in the same pass: `ghcr.io/ublue-os/base-nvidia` instead of
`ghcr.io/ublue-os/bazzite-gnome-nvidia-open:stable`. Rationale: with GDM gone too, there was no
remaining reason to inherit Bazzite's gaming/desktop-specific opinions (gamescope, controller udev
rules, preinstalled Steam/Lutris) - `base-nvidia` is ublue-os's own minimal Fedora Atomic base
(RPMFusion, non-free codecs, their NVIDIA akmod pipeline) with no desktop opinions at all. Real
tradeoff taken on knowingly: unlike Bazzite, there's no `-open` variant of `base-nvidia` (confirmed
- only the closed/proprietary NVIDIA kernel modules are available at this base tier), so this gives
up the open-kernel-module preference from the original Bazzite choice. Revisit if closed-module
akmod build reliability becomes a real practical problem.

**Status**: all code written and unit-tested; a real container build + VM boot test (verifying
greetd actually starts, the greeter renders on the real VT, a real login succeeds, and Flashback
actually comes up afterward) is in progress - see the top-level task list rather than duplicating
build-status prose here, since it'll go stale immediately either way.

**UPDATE (2026-09-27, same night): confirmed complete.** Full pipeline (container build, qcow2,
offline test-account creation, VM boot) run end to end. Real login screen renders via cage,
real PAM auth succeeds against the offline-created account, real GNOME Flashback/Metacity
session launches via the startx wrapper. Screenshots confirmed all three stages visually. The
one open item from this point on is the panel content not rendering - see the next section.

### Blank panel root-cause investigation: WebKit renders internally but is never displayed (2026-09-28)

Continuation of the "panel WebKit content not rendering" item flagged above. Two real diagnostic
passes this session, using `WEBKIT_DEBUG=all`/`G_MESSAGES_DEBUG=all` on
`revolutionos-panel.service` (added at the end of the 2026-09-27 session, actually run for the
first time here) to get WebKit's own internal logging instead of guessing a fifth env var blind.

**Finding, pass 1 (no code change, just the debug env vars):** the panel is NOT failing to render.
The journal shows a completely successful internal pipeline - main HTML resource + 4 sub-resources
all loaded with no errors via the `wails://` scheme handler, 3 successful `runJavaScriptInFrameInScriptWorld`
calls (the React/Mantine bundle actually executing), and WebKit's own layout-milestone dispatcher
explicitly firing `DidFirstVisuallyNonEmptyLayout` and `DidFirstMeaningfulPaint`. WebKit's internal
state is "I painted real content." The window itself is correct too (right size, right position,
right background colour - confirmed via `virsh screenshot`). Nothing between asset-serving and
WebKit's internal paint pipeline is broken. The gap is specifically between WebKit's painted
surface and what actually gets composited into the visible window - a different class of bug than
any of the three previously-tried fixes (all GPU/WebKit-rendering-pipeline-focused: software
rendering fallback, disabling compositing mode, disabling the DMA-BUF renderer).

**Hypothesis tried and ruled out: window-mapping order.** EWMH specifies `_NET_WM_WINDOW_TYPE`
should be set before a window is mapped; `claimAsDock()` (panel/app.go) runs from `OnStartup`,
which fires after Wails has already mapped the window, so the dock type was being set on an
already-live window the whole time - against spec, and a plausible reason Metacity might
inconsistently handle compositing for that window. Fix tried: `runtime.WindowHide()` +
`runtime.WindowShow()` right after setting the dock/strut properties, forcing a fresh,
spec-compliant re-map. **Result: no change.** A second `WEBKIT_DEBUG=all` capture with this fix in
place shows WebKit re-running its entire load from scratch (fresh `WebPage::constructor`, fresh
scheme handler registration, a new `navigationID`) after the show() call, and reaching
`DidFirstVisuallyNonEmptyLayout` + `DidFirstMeaningfulPaint` again - the exact same
internal-success-but-invisible outcome. Whatever's actually wrong survives a full reload
triggered immediately after the dock claim, which rules out map-timing as the cause. Reverted
(panel/app.go, kept only as a code comment) rather than left in, since it added a real cost (an
extra full page-load cycle) for zero benefit.

**Not yet tried / open leads for whoever picks this up:**
- ~~Whether the panel's specific geometry (1280x32 - a 32px-tall window) trips some WebKitGTK
  code path that assumes a more normal window size for compositing.~~ **RULED OUT (2026-09-30)**,
  see below.
- Whether this is VM/virtio-gpu-specific (all testing so far has been in the `revolutionos-test`
  libvirt VM) - worth trying on real hardware with a real GPU to rule out a QEMU-display-specific
  quirk entirely, rather than continuing to iterate purely in the VM. **Still the only untried
  lead** - needs a real-hardware decision from the user, not something to attempt unilaterally.
- ~~GTK/GDK-level inspection (e.g. `GTK_DEBUG=interactive` or similar)~~ **RULED OUT (2026-09-30)**,
  see below.
- ~~Whether `panel/main.go`'s `Frameless: true` combined with the raw X11 dock-type override
  interacts badly with GDK's own surface/backing-store allocation.~~ **RULED OUT (2026-09-30)**,
  see below.

Two `WEBKIT_DEBUG=all` journal captures from this investigation, plus screenshots from every stage
of both VM boot cycles, exist in this session's history if a deeper re-read of the raw evidence is
useful before trying the next hypothesis - see the knowledge-base conversation-history entry for
this project rather than re-deriving from scratch.

### Three more leads ruled out, one real unrelated bug fixed (2026-09-30)

Worked through the three VM-testable leads from the list above. All three came back clean
negative results - same real evidence standard as the rest of this investigation, not guesses.

- **Panel height (32px) - RULED OUT.** Bumped `panelHeight` to 600 (`panel/app.go`), rebuilt,
  booted. Result: the window/background render at 600px exactly as they did at 32px - correct
  size, correct position, correct background colour - but WebKit's content still never composites.
  Identical symptom, just a bigger visible rectangle. Not a geometry issue. Reverted to 32.
- **GDK/GTK-level debug output - RULED OUT, differently than expected.** Added
  `GDK_DEBUG=all` alongside the existing `WEBKIT_DEBUG=all` in `revolutionos-panel.service`,
  rebuilt, booted, captured the full journal for the panel's PID. Result: **zero** `Gdk`-prefixed
  output, vs. `WEBKIT_DEBUG=all` which worked fine logging from the same process in the same run.
  This distro's GTK3 package almost certainly wasn't built with debug hooks enabled
  (`--enable-debug`), so `GDK_DEBUG` is silently a no-op here - not that GDK has nothing to say.
  A debug-enabled GTK rebuild would answer this properly but is a much bigger lift than an env
  var; not attempted. Removed the env var rather than leave a silently-dead setting in the image.
- **`Frameless: true` interaction - RULED OUT.** Flipped to `Frameless: false` in `panel/main.go`,
  rebuilt, booted. Result: the window's behaviour changes a lot (loses dock positioning entirely -
  becomes a normal shadowed floating window, no longer spans the screen width or pins to the top),
  but the WebKit content *still* never renders. Frameless mode isn't interacting with the
  compositing gap - it's a completely separate, unrelated bug in the same binary. Reverted to
  `true`.

**Where this leaves the investigation:** every lead testable inside the VM has now been tried and
eliminated with real evidence. The one remaining lead - VM/virtio-gpu-specificity - structurally
cannot be tested further in this VM by definition; it needs an actual boot on real hardware with a
real GPU, which is a bigger decision (dual-boot, spare hardware, etc.) than continuing to iterate
here. Whoever picks this up next should treat "test on real hardware" as the next concrete step,
not another VM-side guess.

**Real, separate bug found and fixed along the way**: the offline test-account-creation procedure
(qemu-nbd + chroot, used throughout this project's testing) had gone from "worked" to "silently
breaks login every time" on this bootc version (1.16.13). Root cause: `bootc-sysusers-shadow-sync
.service` runs on *every* boot, before `systemd-sysusers.service`, and removes/rewrites anything in
`/etc/shadow` it considers an orphaned or duplicate entry relative to what `systemd-sysusers` would
declare - which includes any manually-`useradd`'d test account, by design (see
`man bootc-sysusers-shadow-sync.service`). The old approach of hand-constructing a shadow line via
`echo "user:$(openssl passwd ...)" >> /etc/shadow` only wrote 2 of the required 9 colon-separated
fields, which this service flagged as unparseable and rewrote destructively on every single boot -
looked exactly like a wrong password from the greeter's side ("Incorrect username or password"),
not like a shadow-file problem, which is why it took a real offline journal read
(`journalctl --root=`) to actually find. **Fix**: use real `chpasswd` inside the chroot instead of
hand-writing shadow lines - it correctly produces all 9 fields, uses this system's actual default
hash scheme (yescrypt, not the `openssl passwd -6` SHA-512 assumed previously), and is (per this
session's evidence) what keeps `bootc-sysusers-shadow-sync` from touching the entry. Two gotchas
worth keeping: `chpasswd` needs `/dev`, `/proc`, `/sys` bind-mounted into the chroot or it fails
outright with no useful error; and it's intermittently flaky immediately after `useradd -m` in the
same chroot session (failed silently on 2 of 5 attempts this session, "cannot open /etc/passwd") -
a retry loop (up to ~5 attempts, 1s apart) reliably gets a clean pass. This is still throwaway
test-account scaffolding per the sign-in/identity section above, not a fix to the real product -
but it unblocks testing in this VM going forward, which the broken version didn't.

### virgl acceleration: host-side blocker found and fixed; panel bug reproduces anyway (2026-10-14)

Picked up the one remaining untested lead from above - whether the compositing gap is
virtio-gpu/lack-of-3D-acceleration-specific - via a different route than real hardware: enabling
`accel3d='yes'` on the existing VM's virtio-gpu device instead of dual-booting physical hardware.

**Host-side blocker found and fixed first.** Three straight attempts to start a `accel3d='yes'`
VM (plain `egl-headless`, `egl-headless` with an explicit `rendernode=`, `spice`+`gl enable='yes'`)
all failed identically: `qemu-system-x86_64: egl: eglInitialize failed: EGL_NOT_INITIALIZED`. Not
a missing-package issue - `libnvidia-egl-gbm.so.1` and `/usr/share/glvnd/egl_vendor.d/10_nvidia.json`
are both present and correctly in place. Root cause, confirmed via `ausearch -m avc`: SELinux was
denying the confined libvirt/qemu domain (`svirt_t`) even a `getattr` on `/usr/bin/nvidia-modprobe`,
which NVIDIA's EGL/GBM init path shells out to - three denials, exactly matching the three failed
attempts, by PID and timestamp. **Fix**: run the VM under session-mode libvirt (`qemu:///session`)
instead of system mode - it runs qemu as the invoking user rather than under `svirt_t`, which
sidesteps the denial entirely with zero change to host SELinux policy. (Disk/nvram files need to be
user-owned for this - libvirt relabels files it manages to `svirt_image_t` dynamically, which
requires owning the file; a root-owned disk/ISO still hits the same class of denial under session
mode. Confirmed separately below.) With that, the VM starts cleanly with `accel3d='yes'` - clean
qemu log, real sustained CPU usage, no crash.

**Could not get a direct screenshot of the accelerated panel.** `virsh screenshot` /
QMP `screendump` returned `internal error: ... unable to execute QEMU command 'screendump':
no surface` against `virtio-vga-gl` with `accel3d='yes'`, on both `egl-headless` and `spice`+`gl`
displays, regardless of how long the VM had been up. This reproduced consistently enough
(including on a VM confirmed actively burning real CPU, i.e. not hung) to read as a QEMU/virgl
limitation around GL-backed surfaces rather than a boot problem - screendump wants a plain
readable framebuffer, which a GL-textured/DMABUF-backed surface may not expose the same way.
Not chased further; whoever picks this up should connect a real SPICE/VNC client interactively
rather than relying on `virsh screenshot` for this specific device combination.

**Separately, built and boot-tested this project's first actual install ISO** (via
`image/disk_config/iso-gnome.toml`, previously present but never run). Two real bugs found and
fixed in the same pass: (1) `bootc-image-builder` needs `--rootfs ext4` explicitly - same class of
"no default root filesystem type" error as the earlier `bootc install to-disk` work above; (2) the
kickstart's `%post` did `bootc switch --mutate-in-place --transport registry
ghcr.io/easyocr/revolutionos:latest` - confirmed via `skopeo inspect` that this image doesn't
resolve (403), which would have bricked every install partway through deployment. Removed the
kickstart override entirely so bootc-image-builder falls back to its default self-contained/offline
installer (image embedded in the ISO, deploys from `/run/install/repo/container`, no registry
pull at install time) - confirmed via a from-scratch install in a plain (non-accelerated,
VNC/std-VGA) VM that this now completes successfully and reboots into a working, logged-in
desktop. (First attempt at this ISO install hit `ENOSPC` - "No space left on device" - partway
through the ostree deployment copy; that was a too-small, 20G throwaway test disk, not a real bug -
a 60G retest completed cleanly. Worth remembering only as a floor for test-disk sizing, 14.6G
image needs real headroom for both the ostree repo objects and the checked-out deployment.)

**The panel bug itself reproduced on this fresh install too** - logged in as `test`/`test`,
desktop and wallpaper rendered correctly, but the top strip of the screen (where the panel docks)
was blank white, same symptom as every prior investigation. This VM had no 3D acceleration
(plain VGA/VNC, not virtio-gpu-gl), so this doesn't by itself tell us whether accel fixes it or
not - the accelerated VM's own panel state is still unconfirmed, per the screendump problem above.
But it's at least one more data point that the bug is not obviously gated on which install path
produced the disk. **Net state of the original question**: still open. The SELinux fix means an
accelerated VM boot is now actually achievable on this host (that part *is* resolved, and is
independently useful for any future accel-dependent VM testing), but it hasn't yet produced a
confirmed look at the panel under acceleration - whoever picks this up next should connect a real
SPICE client to a session-mode, `accel3d='yes'` VM (XML pattern above) and look directly, rather
than re-starting from real-hardware dual-boot.

## Hard requirements

- **Redistributable.** Other people install this, not just the original builder. Needs to be
  genuinely maintainable, not a one-off hand-tuned image.
- **NVIDIA GPU required.** Not targeting AMD/Intel. This matches what RevoMacro itself wants
  (CUDA-accelerated detection AI — OCR, buff/ability detection via ONNX Runtime), so it's not
  an artificial restriction, it's the actual target audience.
- **Full usable desktop, not a minimal kiosk/appliance image.** This ruled out Yocto/Buildroot as
  the base — see "Why Fedora Atomic, not Yocto/Buildroot" below.
- **No VMs.** Multi-instance isolation has to happen without virtualizing anything — no
  per-account VMs, no vGPU. (vGPU was considered and rejected: it's a paid enterprise license,
  deliberately blocked at the driver level on GeForce cards, and solves GPU-sharing *across VMs*
  specifically — irrelevant once instances are plain host processes instead.)

## Why Fedora Atomic (ublue-style), not Yocto/Buildroot

Yocto/Buildroot are for from-scratch embedded appliances with narrow, known hardware — the wrong
tool once "redistributable + full desktop" is the actual target:

- NVIDIA driver packaging for arbitrary end-user hardware is already a *solved, maintained*
  problem in the ublue/Bazzite ecosystem. Reinventing that in Yocto would mean repackaging
  proprietary driver integration from scratch.
- RevoMacro and Sober (the Roblox client used here) are **already flatpaks** — Fedora Atomic's
  native app model. Zero repackaging needed; they just get pre-installed in the image.
- rpm-ostree's atomic update/rollback matters *more* here than for a personal box, specifically
  because this ships to other people's machines — nobody's SSHing in to fix a broken update on
  someone else's install.
- Full GNOME (or whatever desktop) comes for free, satisfying "usable, not minimal."

Concretely: this should be built the way Bazzite itself is built — a custom OCI container image
layered on Fedora's atomic base, deployed via rpm-ostree/bootc. Look at how Bazzite/ublue
structure their image-build repos (Containerfile-based, GitHub Actions) as the template.

## The actual novel work: the multi-instance orchestrator

This is the flagship feature, not a side script. Goal: run N concurrent RevoMacro + Sober pairs
(one per Roblox account) on one machine, with **zero custom capture or input-routing code**, by
giving each instance its own throwaway isolated display instead.

- **CONFIRMED (2026-09-24) via RevoMacro's own bundled `linuxdiag` diagnostic tool** (`flatpak run
  --command=linuxdiag com.revolutionmacro.RevolutionMacro`, JSON output), run in two real
  environments on the dev machine:
  - **GNOME/Mutter (Wayland)** — where the app has been reliably used for actual gameplay so far:
    `backend_candidates`: capture = `portal-window` (PipeWire ScreenCast portal), input =
    `portal-remote-desktop`, keyboard = `eis`. Hotkeys resolve to `focused_gdk`
    (window-focus-dependent), not the global-shortcut portal, despite the app preferring it.
  - **GNOME Flashback / Metacity (X11)** — the currently-reliable path while Liam's own Wayland
    support is still stabilizing: `backend_candidates`: **capture = `xcomposite`, input =
    `x11-xtest`, keyboard = `x11-xtest`**. Neither uses raw uinput.
  - In **neither** tested environment did it fall back to the unscoped raw-uinput path
    (`uinput_write`/`input_read` are available per the flatpak's permissions, but simply weren't
    selected) — it always prefers a properly session/display-scoped mechanism when one exists.

### Resolved: X11 (Flashback) is the actual target, and isolation is structurally free there too

X11's `XTest` (input injection) and `XComposite` (capture) are both inherently scoped to whichever
`$DISPLAY` the client connects to — same free-isolation property originally assumed for a
nested-Wayland-compositor approach, just via X11's older, more battle-tested mechanism instead.
Since Flashback/X11 is also the currently-*reliable* environment (Wayland has known issues right
now per Liam), it's the right v1 target, not a nested Wayland compositor like `cage`:

- Give each account its own X server (`Xephyr` — already present on this host at
  `/usr/bin/Xephyr` — or `Xvfb`) running its own `metacity`, matching the confirmed-working
  Flashback stack exactly.
- Still no custom capture or input-routing code needed, for the same structural reason as the
  original plan — just correctly re-targeted at X11's isolation primitives instead of Wayland's.
- This also fully sidesteps the unrelated DIY `bss-automation` project's old uinput/virtual-gamepad
  isolation problem (see that project's own `vision/README.md`) — that was a different macro using
  a different, deliberately-global input path. Not applicable here.

First orchestrator implementation is in `scripts/orchestrator/` — see that directory's scripts for
usage. **Concurrent logins CONFIRMED working end-to-end (2026-10-11)** — see "True concurrency:
root cause found and fixed" below for the full story (real root cause was a cross-instance port
collision in Sober's own single-instance self-check, fixed with per-instance network namespaces).
Two real accounts ran simultaneously, both fully rendered and in live gameplay, visible to each
other as players in the same game world. A `multisober` CLI now wraps the orchestrator by Roblox
username (`multisober launch <username>` / `multisober addinstance`) rather than internal instance
names, and each instance now shows as a single normal top-level window ("Sober: @username") instead
of nested Xephyr-in-host-WM chrome - see the knowledge-base notes for that work's details.
Still TODO from this list: per-instance GPU/VRAM cost was never actually measured (see below).

**Known annoyance, not yet investigated (2026-10-01):** Roblox/Sober sometimes freezes outright
when joining a server, reportedly tied to malformed responses for moderated assets (an asset that's
been moderated/removed returning something the client doesn't handle gracefully during the
join/asset-load phase) rather than a clean error or placeholder. Directly relevant to orchestrator
reliability specifically because this is a *per-instance* client-side hang - one account's Sober
instance freezing on a bad asset shouldn't be allowed to take down or block the others. No repro
steps, logs, or root-causing done yet - just flagged here so it doesn't get lost before the
concurrent-login testing above happens.

## Build order (do not skip ahead to image-building)

1. **Prototype the orchestrator locally first**, on ordinary Fedora/Bazzite, before touching any
   image-build tooling:
   - `Xephyr`/`Xvfb` + `metacity` per instance (see `scripts/orchestrator/`), not a Wayland
     compositor — see the X11/Flashback finding above.
   - Isolated Sober + RevoMacro profiles per instance, via a per-instance fake `$HOME` (flatpak's
     per-app storage is rooted at `$HOME/.var/app/<id>`, so overriding just `HOME` for the
     `flatpak run` call isolates both apps' data with one mechanism — see
     `scripts/orchestrator/lib.sh`).
   - Still TODO, not yet done: actually launch two instances concurrently with two real accounts
     and verify capture/input stay scoped correctly with nothing bleeding across.
   - Still TODO: **measure per-instance GPU cost** (VRAM + compute) — the bundled CUDA/cuDNN/ONNX
     stack alone was ~2.4GB on disk per instance; this is the real ceiling on how many concurrent
     instances a given card can run, not anything architectural.
2. Once that's proven: wrap it in an actual bootable custom image. **Scaffolded (2026-09-24,
   ahead of step 1's live-test) in `image/`** - Containerfile, build.sh, disk_config/iso-gnome.toml,
   and a build.yml CI workflow, all following the real, current ublue-os/image-template
   conventions (verified via their actual repo, not assumed from memory). Base image is
   `ghcr.io/ublue-os/bazzite-nvidia-open:stable` - the open-kernel-module NVIDIA variant, since
   the dev machine's 3080 qualifies and open modules are easier to keep building across kernel
   updates than the closed akmod path.
   - **NOT YET BUILT OR BOOTED.** These are correct file definitions, not a proven artifact - no
     CI run has been triggered, no image has actually been built, no ISO generated, nothing
     boot-tested. "The Containerfile is written" and "there is a working OS" are different claims.
   - **Real gap found while scaffolding this**: RevolutionMacro has no actual flatpak repo/remote
     - confirmed on the dev machine, its flatpak "origin" has an empty `url=`, meaning it was
     installed from a downloaded `.flatpak` bundle file, not a real server. Sober, by contrast, is
     properly on Flathub (confirmed: `Origin: flathub`). Since redistributing Liam's proprietary
     bundle in this public repo isn't ours to decide, `build.sh` expects it supplied locally at
     `image/local-assets/revolution-macro.flatpak` (gitignored) rather than committing it - real
     long-term fix is asking Liam for an actual flatpak repo to install from instead.
   - **Also unresolved**: making Flashback/Metacity the *default selected* session at the GDM
     login screen (not just installed) needs more than what's in build.sh right now - probably an
     AccountsService default or first-boot script, not confirmed which.
   - **Fixed a real gap (2026-09-24)**: the Containerfile originally didn't install the
     orchestrator into the image at all - it had Sober, RevolutionMacro, and gnome-flashback, but
     not the actual point of this project. Now: `Containerfile` has a `ui-builder` stage that
     compiles `ui/` from source (confirmed working - built successfully in a distrobox on the dev
     machine, needed since the host is immutable and lacks webkit2gtk-devel; needed the
     `webkit2_41` Wails build tag since this Fedora release only ships webkit2gtk-4.1, not the
     4.0 Wails defaults to), copies the binary to `/usr/libexec/revolutionos/`, and `build.sh`
     installs `scripts/orchestrator/` alongside it plus a `.desktop` entry. `app.go`'s path
     resolution was also fixed - it used to only work from a dev checkout (`runtime.Caller`-based,
     relative to source location), now checks the real installed path first and falls back to the
     dev-relative one, so both `wails dev` locally and the packaged image work.
   - Also fixed: the CI workflow was at `image/.github/workflows/build.yml`, which GitHub Actions
     never actually reads (only repo-root `.github/workflows/` is recognized) - moved to the
     correct location. It would never have run as originally committed.
   - Build context changed from `image/` to the repo root, since the Containerfile now needs to
     reach `ui/` and `scripts/` as siblings for the multi-stage build.
3. Branding/first-run UX/packaging polish comes last.

## Optional infrastructure: precise-input (2026-09-25)

`precise-input/` - infrastructure for Liam to optionally integrate into RevolutionMacro later for
lower-latency input than XTest, without giving up the isolation the whole orchestrator depends on.
Not wired into anything else yet; RevolutionMacro and the orchestrator both continue working
exactly as before regardless of whether this is ever adopted. See `precise-input/README.md` for
the full design writeup - summary:

- **Proven with real evidence**: a `bwrap` sandbox exposing only one uinput device can send/receive
  real events through it (injected `KEY_A`, sandboxed reader got `code=30 value=1`/`value=0`), and
  a sandbox scoped to a different instance genuinely cannot see that device at all (`ENOENT`, not
  `EACCES`). This directly answers the old `bss-automation` DIY project's unsolved problem (raw
  uinput being precise but global) - RevolutionOS can offer precise AND isolated together because
  it already controls each instance's sandbox setup.
- **Real gotcha found and solved**: uinput devices are owned `root:input` regardless of creating
  uid; normal access works outside a sandbox via a `systemd-logind` per-session ACL grant that does
  NOT carry into a bwrap sandbox's fresh device namespace (shows up owned by unmapped uid `65534`
  inside). Fixed by chmod'ing the device at creation time rather than relying on ambient
  permissions surviving the sandbox boundary.
- `daemon.py` + `client_example.py`: actually built and tested end-to-end (not just written) -
  real device creation, real Unix socket, real key/mouse commands executed and confirmed, real
  error handling for bad commands.
- **Not yet done**: the capture-side equivalent (design only, see README), wiring this daemon into
  `start-instance.sh`, and an auto-detection mechanism for RevolutionMacro to discover it's running
  under RevolutionOS at all.

## Reference facts gathered so far (dev machine: Bazzite, RTX 3080, driver 615.71.09)

- RevoMacro flatpak: `com.revolutionmacro.RevolutionMacro`, ships its own bundled CUDA 12 /
  cuDNN 9 / ONNX Runtime 1.26.0 stack under `gpu/cuda/` (doesn't depend on a system CUDA
  toolkit, only the driver's `libcuda.so`).
- Confirmed CUDA/GPU inference works end-to-end when the macro is actually gathering (ONNX
  Runtime CUDA execution provider, `backend: ort-cuda`) — init is lazy, only fires once
  detection is actually needed, not at mere app launch.
- Ships extra diagnostic/helper binaries worth knowing about: `linuxdiag` (environment capability
  probe — the JSON dump above), `gpusmoke`, `capturesmoke`, plus an optional unsandboxed
  companion daemon at `/app/libexec/revolution-linuxhostbridge` (currently not installed/needed).
- `settings.yaml` already has a `presets:` list and `windows:` array (with per-window `screen`
  fields) — confirms multi-account *configuration* is native, it's concurrent *execution* that
  isn't.
- Has built-in `networking.remoteControlEnabled` / cloud relay settings — not yet investigated,
  might be relevant to how instances could report status to a future orchestrator UI.

## Orchestrator isolation hardening + Sober single-instance finding (2026-10-11)

Prompted by two things: Liam possibly integrating with `precise-input` soon (making it worth
firming up the orchestrator's isolation story), and research suggesting most real users will run
RevolutionOS in a Proxmox VM with GPU passthrough rather than bare metal (uinput/bwrap should be
transparent to virtualization since both are guest-kernel primitives, but this hasn't been
explicitly validated inside an actual VM yet - still an open item).

**Two real gaps found and fixed in `start-instance.sh`/`lib.sh`:**
- `$XDG_RUNTIME_DIR` was never overridden per-instance - every instance inherited the real host's
  `/run/user/<uid>`, so anything keyed off that path (IPC sockets, lock-style files) would have
  collided across instances. Now each instance gets its own `instance_runtime_dir()` (0700 perms,
  required by the XDG spec), created in `start-instance.sh` and cleaned up in `stop-instance.sh`.
- X11 display isolation was "by convention" only (confirmed via code read, not assumption) - each
  instance used its own `$DISPLAY` but the X server accepted any client, no auth required. Added a
  per-instance MIT-MAGIC-COOKIE via `xauth`/`mcookie`, passed as `-auth` to the X server
  (Xvfb/Xephyr both support it) and `XAUTHORITY` to every client. **Proven with real evidence**: a
  Python Xlib test against a live Xvfb confirmed the right cookie connects, a wrong cookie gets
  refused (`Invalid MIT-MAGIC-COOKIE-1 key`), and no `XAUTHORITY` at all gets refused
  (`Authorization required, but no authorization protocol specified`).

**Sober's single-instance lock - real finding, not assumption:**
Initially assumed (from a secondhand source) this was a deliberate anti-abuse mechanism embedded
deep in the Roblox Android binary translation layer. Direct testing showed otherwise. Running
`flatpak run org.vinegarhq.Sober` a second time while one instance was already live produces:
```
FATAL: Crash: An instance of Sober is already running.
```
This fires immediately after `fs_init` in Sober's own startup log, before any Android/Roblox
translation work begins - strongly indicating a plain filesystem lock check, not something buried
in the emulated Roblox binary itself. Confirmed by elimination: launching a second instance
through the now-isolated orchestrator environment (isolated `$HOME`, which Flatpak already derives
`XDG_DATA/CONFIG/CACHE_HOME` from, plus the new `$XDG_RUNTIME_DIR` isolation) got **past** `fs_init`
entirely and into onboarding flow setup, still alive 15+ seconds later - no crash.

One non-fatal side finding: Flatpak's own portal bookkeeping doesn't fully respect the
`XDG_RUNTIME_DIR` override for its own internal files (`Failed to open file
"/run/user/1000/.flatpak/<id>/bwrapinfo.json"` - still the real host path, not the isolated one) -
produced a GTK warning but didn't block anything.

**Why this matters beyond just "it works":** this means two properly-isolated instances don't
collide because they're genuinely separate environments (same as two real separate Linux user
accounts never would), not because anything in Sober's binary was patched or reverse-engineered
around. The isolation work was already worth doing for its own sake (closing real gaps); it
appears to resolve the Sober lock as a natural side effect rather than requiring any
Sober-specific circumvention.

**Not yet done:**
- Only tested against a fresh, never-logged-in instance going through onboarding - not yet tested
  with two already-logged-in instances (different accounts) running concurrently for an extended
  period.
- VM/Proxmox-specific validation of the uinput/bwrap isolation proof (see `precise-input/README.md`)
  still outstanding - all testing so far has been on bare metal.

### Same-day deep dive: second concurrent instance never presents a visible frame (unresolved)

With `test1` logged in and genuinely playable (confirmed visually - live gameplay, quest popups,
honey/pollen counters all updating), `test2` would launch, run real game logic, but its Xephyr
window stayed permanently black. This took a long investigation to even characterize correctly;
several earlier conclusions in this doc turned out to be wrong once tested further:

**Real, confirmed findings, each tested directly:**
- `nvidia-smi` showed `test1`'s Sober process as an active GPU client; `test2`'s never appeared -
  while `test1` was running, `test2` never even registered as using the GPU.
- `dmesg` showed live (not stale) `[drm:drm_new_set_master] Failed to grab modeset ownership`
  errors timestamped to the exact relaunch - DRM master is exclusive per-GPU, host Xorg holds it
  legitimately, and something in Sober's init path attempts to grab it anyway.
- **Isolation test**: stopped `test1` completely, launched `test2` alone with real hardware Vulkan.
  It *did* register as a GPU client this time, and its log showed a real swapchain actually get
  created (`vulkan: create swapchain with w 1280 h 683`, `swapchain images 3`) - confirming the
  GPU-master conflict is specifically about *concurrent* access, not something broken in test2's
  setup alone.
- **Even so, the window stayed black.** Screenshotted while the process was confirmed alive (PID
  checked immediately before and after) - genuinely no frame ever presented, despite successful
  swapchain creation.
- **Earlier "it died silently" conclusion was wrong.** What looked like a fast silent death was
  actually `ps | grep "sober$"` checks landing at unlucky moments. A longer, properly-synchronized
  test showed the process alive 20-40+ seconds, deep in a legitimate exponential-backoff retry loop
  for an unrelated failing voice-chat TURN API call (8s/16s/32s gaps, HTTP 401) - normal behavior
  for an account without voice chat authorized, nothing to do with rendering.
- **Tried forcing software Vulkan** (Mesa lavapipe, via `VK_ICD_FILENAMES`): got further than
  hardware Vulkan in isolation - real shader compilation (2991 shaders), `SurfaceController::RenderJob`
  starting, successful transition to `stage:LuaApp`. User directly confirmed the quick-sign-in QR
  code rendered and was interactive at one point with this path. Not consistently reproducible
  across relaunches though.
- **Found and fixed a real, separate bug along the way**: `start-instance.sh`'s `RUNTIME_DIR` setup
  only did `mkdir -p` (no-op if already present), so flatpak's own per-launch instance tracking
  files (`.flatpak/<id>/pid` etc.) accumulated stale state across repeated relaunches of the same
  instance - a dead PID left in one of these files was enough to make Sober's own frozen-instance
  self-check false-positive on an otherwise-fresh launch. Fixed with `rm -rf` before `mkdir -p`.

**Dead ends, each eliminated with real evidence, not assumption:**
- No coredump, ever, from any death (`coredumpctl list` checked after every single attempt).
- No OOM kill or any kernel-level signal in `dmesg` around any death.
- Installed Vulkan validation layers (via Homebrew + a patched layer manifest with an absolute
  `library_path`, since Homebrew splits `share/`+`lib/` in a way the relative path in the stock
  manifest doesn't resolve, plus explicit `flatpak run --filesystem=...--env=VK_LAYER_PATH=...`
  since the sandbox doesn't see host paths by default) - the layer loaded (confirmed in the log),
  but produced no VUID/validation error before the process died, and died *faster* than without it.
- Installed `strace` (Homebrush) and traced the full process tree from launch. Under strace, the
  entire sandbox+Sober process tree exited within ~200ms - dramatically faster than any untraced
  run (which ranged from a few seconds to 40+ seconds). This is a real observer-effect confound:
  the trace changes the timing enough that it's not clear it's still observing the same failure
  mode, so its output (mostly normal bwrap sandbox-setup process churn) isn't trustworthy here.

**Correction after further testing:** a `sudo strace` run (see above) left root-owned files in
`test2`'s runtime dir that the normal (non-sudo) `rm -rf` in the test cycle silently failed to
remove - `start-instance.sh`/manual test cycles don't check `rm`'s exit status, so this went
unnoticed for several subsequent "clean" relaunches. Once actually cleaned with `sudo rm -rf` and
relaunched, `test2` **rendered correctly** - real Roblox home screen UI (friends list, avatars,
recommended games), confirmed stable across multiple screenshots several seconds apart, process
alive throughout. So some fraction of the black-screen attempts documented above were this
self-inflicted contamination, not (or not only) the deeper Vulkan/DRM-master issue.

**Where this actually stands now:** the DRM-master concurrency conflict between simultaneous
instances is still real (confirmed via live `dmesg` evidence) and likely still relevant for true
concurrent-instance use, but the single-instance black-screen mystery turned out to have a mundane
contributing cause (contaminated runtime dir from a diagnostic side-effect, not a product bug).
**Not yet re-tested: both instances running fully concurrently on a guaranteed-clean runtime dir** -
that's the actual next step to re-confirm whether the original concurrency conflict still reproduces
now that this confound is removed, before concluding anything further about VinegarHQ's code.

### Follow-up same day: the real blocker for fresh instances was a flatpak override, not isolation

After the above, two actual test instances (`test1`/`test2`, created via `create-instance.sh`) were
launched for real end-to-end testing. Every attempt - through `start-instance.sh` directly, through
the same script with manual `disown` added, and even fully manual step-by-step replication - died
consistently a couple seconds into Sober's startup, every single time, with Xephyr staying up but
metacity/Sober/RevoMacro all disappearing.

Several wrong turns chasing this, each ruled out with real evidence before moving on:
- Suspected the Bash-tool-invocation session boundary killing backgrounded children on tool-call
  exit (real effect, confirmed via `nohup`+`disown` fixing it once) - but the same death still
  happened when the user ran it directly in their own persistent terminal, ruling this out as the
  *main* cause.
- Found and fixed a real bug along the way: `start-instance.sh`'s background processes weren't
  disowned at all, so a stray `Ctrl+C` (or any signal to the invoking shell's process group) could
  take them down. Fixed by adding `disown "$!"` immediately after each `&` in the script itself.
  Genuinely correct fix, kept - but not what was causing *this* specific symptom.
- Checked SELinux (`data_home_t` on the instance home dir vs. `user_home_dir_t` on the real one -
  a real mismatch) but zero AVC denials anywhere (`ausearch`, `dmesg`), so not actually enforced
  against here. Ruled out.

**Actual cause**: Sober has its own permission-mismatch self-check. On launch under the isolated
`$HOME`, it showed its own dialog: *"Permission Update Required... Copy, inspect and run the full
command below in a terminal. Restart Sober once done."* - with the command
`flatpak override --user --device=input org.vinegarhq.Sober`. This override already existed for
the real user (`~/.local/share/flatpak/overrides/org.vinegarhq.Sober`), but flatpak's user-level
overrides are stored per-`$HOME` - so an isolated instance with a different `$HOME` genuinely never
had it applied, regardless of how well the X11/XDG_RUNTIME_DIR isolation worked.

**Fix**: run the same override once per instance, with `HOME` pointed at that instance's isolated
home, before first launch:
```
HOME="$(instance_home <name>)" flatpak override --user --device=input org.vinegarhq.Sober
```
Confirmed working after this for both `test1` and `test2` - Sober progressed well past where it
previously died (`devices_init`, `global_window_init`, no crash).

Now wired into `create-instance.sh` itself, so every new instance gets this applied automatically
at creation time rather than needing to be remembered as a manual step.

## True concurrency: root cause found and fixed (2026-10-11, later the same day)

With the above fixes in place, a single instance ran solo reliably, but running two instances
*concurrently* reliably produced Sober's own "There's a frozen instance of this app running in
the background and causing issues" dialog on the second one - not a crash, a deliberate
self-check misfiring. Timing was inconsistent (sometimes instant, sometimes ~15s into idle,
once ~2s into live gameplay), which was the first clue it wasn't a fixed startup checkpoint.

**Root cause**, confirmed via a live `bpftrace` syscall capture (strace is blocked - see below):
Sober does a bind-if-primary-else-connect-to-primary handshake on `127.0.0.1:55142` as its
single-instance check. Every orchestrator instance shares the host's real network namespace
(flatpak's manifest grants `shared=network`), so a second instance's `connect()` to that port
lands directly in a FIRST instance's real listener instead of failing cleanly - confirmed via
`ss -tlnp` showing only one real listener system-wide at any time. Whatever the primary expects
from that connection, it isn't a foreign instance answering, and that mismatch is what the
dialog is reacting to.

**Dead end tried first**: an `LD_PRELOAD` shim (`scripts/orchestrator/port-remap-shim.c`, still
in the repo but unwired) that intercepts just `bind()`/`connect()` to `127.0.0.1:55142` and
remaps the port per-instance. Verified working perfectly in isolation against plain test
processes. But flatpak's own manual is explicit: it "always unsets or overrides" `LD_PRELOAD`
(along with `PATH` and `LD_LIBRARY_PATH`) for every sandboxed launch, no override exists.
Confirmed directly - the var never reaches the sandboxed process's own environment no matter
how it's passed to `flatpak run`. Deliberate sandbox hardening, not a gap with a workaround.

**Actual fix**: a real per-instance network namespace (veth pair + NAT to the host), the same
category of isolation `$DISPLAY`/`$HOME`/`$XDG_RUNTIME_DIR` already had. Implemented in
`lib.sh` (`setup_instance_netns`/`teardown_instance_netns`/`netns_exec_as_user` and the
supporting naming/addressing helpers), wired into `start-instance.sh` (netns created right
after display allocation; only the Sober/RevoMacro launches go through it - Xephyr/metacity
stay on the host, since neither needs network and it sidesteps any risk around X11's
network-namespace-scoped abstract socket) and `stop-instance.sh` (torn down on stop).
Addressing is `10.200.<display-num>.0/30` per instance. DNS inside each namespace points at
`1.1.1.1`/`8.8.8.8` directly rather than copying the host's `/etc/resolv.conf` - this host
points at `127.0.0.53` (systemd-resolved's stub), and loopback addresses never route out
through a veth, so a naive copy would leave the namespace with a dead resolver.

**Confirmed end to end**: both `test1` and `test2` launched concurrently, each verified to be
in its own distinct network namespace (`/proc/<pid>/ns/net` matching that instance's own
`ip netns exec revos-<name> readlink /proc/self/ns/net`), each with its own independent
`LISTEN` on `127.0.0.1:55142` simultaneously (different PIDs, zero conflict), neither hitting
the frozen-instance dialog, both fully rendered and in live gameplay with genuinely different
account progress - and each could see the other as a player in the same game world in their
respective screenshots, confirming real multiplayer networking continues to work correctly
through the NAT'd connections, not just that the processes are alive.

Also confirmed and worth remembering: `strace` is categorically blocked for this binary -
Sober detects ptrace and refuses to start with its own "enable ptrace (kernel.yama.ptrace_scope)"
dialog, confirmed via a host (non-orchestrator) launch under `strace`. `bpftrace` (a different
kernel mechanism, not ptrace-based) is NOT detected and is the way to get real syscall
visibility going forward - not available via Homebrew on this box, needs
`sudo rpm-ostree install bpftrace` plus a reboot to apply (image-based OS, no live layering).
