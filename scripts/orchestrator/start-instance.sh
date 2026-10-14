#!/usr/bin/env bash
# Starts an instance: allocates an isolated X11 display, launches metacity on it
# (matching the confirmed-working Flashback stack), then launches Sober and
# RevolutionMacro both pointed at that display with HOME overridden to the
# instance's isolated profile dir.
#
# Isolation mechanisms: X11 XTest/XComposite (inherently per-$DISPLAY), per-instance xauth
# cookie (X server itself refuses foreign clients), per-instance XDG_RUNTIME_DIR, and a real
# per-instance network namespace (veth pair + NAT) for Sober/RevoMacro specifically - this
# last one is load-bearing, not cosmetic: Sober's own single-instance self-check does a
# bind-if-primary-else-connect-to-primary handshake on 127.0.0.1:55142, which reaches across
# instances when the network namespace is shared (flatpak's manifest grants shared=network),
# confirmed via a live bpftrace syscall capture (see knowledge-base notes). No amount of
# $HOME/$DISPLAY/$XDG_RUNTIME_DIR isolation touches that since loopback isn't namespaced by
# any of them. (Tried an LD_PRELOAD shim first - much smaller, but flatpak's own manual says
# it "always unsets or overrides" LD_PRELOAD for every sandboxed launch, confirmed directly,
# no override exists. A real netns is the only thing that actually isolates loopback.)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

# Captured before anything below overrides $DISPLAY for the nested instance - this is the
# host's own real display, needed later to rename the Xephyr window as it actually appears
# on the host desktop (Xephyr is a client of the host's X server, not the nested one).
HOST_DISPLAY="${DISPLAY:-:0}"

BACKEND="xvfb"
GEOMETRY="1280x720"
NAME=""

while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND="$2"; shift 2 ;;
        --visible) BACKEND="xephyr"; shift ;;
        --geometry) GEOMETRY="$2"; shift 2 ;;
        -*) echo "Unknown flag: $1" >&2; exit 1 ;;
        *) NAME="$1"; shift ;;
    esac
done

if [ -z "$NAME" ]; then
    echo "Usage: $0 <instance-name> [--backend xephyr|xvfb] [--visible] [--geometry WxH]" >&2
    echo "  --visible is shorthand for --backend xephyr (shows a real window instead of headless)" >&2
    exit 1
fi

require_instance_exists "$NAME"

HOME_DIR="$(instance_home "$NAME")"
PID_FILE="$(instance_pid_file "$NAME")"

if [ -f "$PID_FILE" ]; then
    echo "Instance '$NAME' looks like it's already running (found $PID_FILE)." >&2
    echo "Run stop-instance.sh $NAME first if that's stale." >&2
    exit 1
fi

DISP_NUM="$(allocate_display)"
DISP=":$DISP_NUM"
PIDS=()

setup_instance_netns "$NAME" "$DISP_NUM"

# XDG_RUNTIME_DIR was never overridden before this - every instance inherited the real
# host's /run/user/<uid>, meaning anything keyed off that path (IPC sockets, lock files)
# collided across instances. The XDG spec requires 0700 perms owned by the user, and
# some apps check this and refuse to run otherwise - not just a nice-to-have.
RUNTIME_DIR="$(instance_runtime_dir "$NAME")"
# rm before mkdir, not just mkdir -p: flatpak leaves its own per-launch instance tracking
# files under here (.flatpak/<id>/pid etc.) that don't always get cleaned up when a process
# is killed directly rather than exiting on its own. A stale pid file pointing at a dead
# process was enough to make Sober's own frozen-instance self-check false-positive on a
# completely fresh launch - caught live, not guessed.
rm -rf "$RUNTIME_DIR"
mkdir -p "$RUNTIME_DIR"
chmod 700 "$RUNTIME_DIR"

# Previously isolation here was "by convention" only - each instance used its own
# $DISPLAY but the X server accepted any client, since no auth was required. A process
# that connected to a different instance's display (whether by bug or by something
# inside the sandbox deliberately trying) would have been let straight in. A per-instance
# MIT-MAGIC-COOKIE closes that: the X server itself refuses clients that don't present it.
XAUTH_FILE="$(instance_xauth_file "$NAME")"
rm -f "$XAUTH_FILE"
COOKIE="$(mcookie)"
xauth -f "$XAUTH_FILE" add "$DISP" . "$COOKIE" >/dev/null
chmod 600 "$XAUTH_FILE"

cleanup_on_failure() {
    echo "Startup failed, cleaning up ${#PIDS[@]} process(es)..." >&2
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    rm -f "$(instance_display_file "$NAME")"
    rm -rf "$RUNTIME_DIR"
    rm -f "$XAUTH_FILE"
    teardown_instance_netns "$NAME" "$DISP_NUM"
}
trap cleanup_on_failure ERR

case "$BACKEND" in
    xephyr)
        # -dpi 96: without this, Xephyr picks its own default DPI for the nested screen
        # instead of matching the host's (confirmed host Xft.dpi is 96) - UI inside ended up
        # rendering absurdly oversized, caught live via user report, not guessed in advance.
        Xephyr "$DISP" -auth "$XAUTH_FILE" -screen "$GEOMETRY" -dpi 96 -resizeable -title "RevolutionOS: $NAME" &
        PIDS+=("$!"); disown "$!"
        ;;
    xvfb)
        # -xkbdir: this Xvfb build (Homebrew) ships no xkeyboard-config data of
        # its own - without this it dies immediately with "XKB: Failed to
        # compile keymap" / "Failed to activate virtual core keyboard".
        # Confirmed via a standalone repro, not guessed.
        "$XVFB_BIN" "$DISP" -auth "$XAUTH_FILE" -screen 0 "${GEOMETRY}x24" -xkbdir /usr/share/X11/xkb &
        PIDS+=("$!"); disown "$!"
        ;;
    *)
        echo "Unknown backend: $BACKEND (use xephyr or xvfb)" >&2
        exit 1
        ;;
esac

# Wait for the X server's socket to actually appear before launching clients against it.
for _ in $(seq 1 40); do
    [ -e "/tmp/.X11-unix/X$DISP_NUM" ] && break
    sleep 0.25
done
if [ ! -e "/tmp/.X11-unix/X$DISP_NUM" ]; then
    echo "X server for '$NAME' never came up on $DISP" >&2
    exit 1
fi

echo "$DISP" > "$(instance_display_file "$NAME")"

# GDK_BACKEND=x11: this shell's environment has WAYLAND_DISPLAY set (inherited
# from the host's own session), which makes GTK3 auto-detect and try the
# Wayland backend by default - metacity is X11-only, so that mismatch made it
# fail with a misleading "Unable to open X display" error that had nothing to
# do with the actual X connection. Confirmed via a standalone repro (bare
# metacity against a known-good Xvfb still failed until this was set), not
# guessed - this was the real root cause of the orchestrator's per-instance
# window manager never starting at all.
DISPLAY="$DISP" XAUTHORITY="$XAUTH_FILE" XDG_RUNTIME_DIR="$RUNTIME_DIR" GDK_BACKEND=x11 metacity --replace &
PIDS+=("$!"); disown "$!"
sleep 0.5

# Only the flatpak app launches go into the per-instance netns, not Xephyr/metacity -
# neither needs network access, and keeping them on the host avoids any risk around X11's
# abstract-namespace socket (which IS network-namespace-scoped, unlike the filesystem one
# at /tmp/.X11-unix which bwrap already bind-mounts in regardless of network namespace).
netns_exec_as_user "$NAME" DISPLAY="$DISP" XAUTHORITY="$XAUTH_FILE" HOME="$HOME_DIR" XDG_RUNTIME_DIR="$RUNTIME_DIR" flatpak run "$SOBER_APP_ID" &
PIDS+=("$!"); disown "$!"

# Headless (xvfb) has no visible window to de-nest or rename - only bother for xephyr.
if [ "$BACKEND" = "xephyr" ]; then
    apply_instance_window_cosmetics "$NAME" "$DISP" "$XAUTH_FILE" "$HOST_DISPLAY" &
    disown "$!"
fi

# Give Sober a head start before RevoMacro attaches - matches the confirmed-working
# Flashback launch order (RevoMacro expects Sober's window to already exist).
sleep 2

netns_exec_as_user "$NAME" DISPLAY="$DISP" XAUTHORITY="$XAUTH_FILE" HOME="$HOME_DIR" XDG_RUNTIME_DIR="$RUNTIME_DIR" flatpak run "$REVOMACRO_APP_ID" &
PIDS+=("$!"); disown "$!"

trap - ERR
printf '%s\n' "${PIDS[@]}" > "$PID_FILE"

echo "Started instance '$NAME' on $DISP (backend: $BACKEND)."
echo "PIDs: ${PIDS[*]}"
