#!/usr/bin/env bash
# Stress-tests orchestrator instance isolation with synthetic clients - no real
# Sober/RevolutionMacro, no real Roblox accounts. Spins up two throwaway
# instances (own X11 display + own fake $HOME, same mechanism start-instance.sh
# uses), each running the synthetic-client test window, then empirically checks
# what actually is and isn't isolated between them. Cleans up fully afterward -
# these are disposable test instances, not real user profiles.
#
# Usage: ./test-synthetic-isolation.sh
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

CLIENT_BIN="$SCRIPT_DIR/testing/synthetic-client/synthetic-client"
INSTANCE_A="synth-test-a"
INSTANCE_B="synth-test-b"
PIDS=()
FAILURES=0

if [ ! -x "$CLIENT_BIN" ]; then
    echo "Synthetic client not built - run: (cd $SCRIPT_DIR/testing/synthetic-client && go build -o synthetic-client .)" >&2
    exit 1
fi

check() {
    local desc="$1" ok="$2"
    if [ "$ok" = "0" ]; then
        echo "  PASS: $desc"
    else
        echo "  FAIL: $desc"
        FAILURES=$((FAILURES + 1))
    fi
}

cleanup() {
    echo "--- Cleaning up ${#PIDS[@]} process(es) and test instance dirs ---"
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    sleep 0.3
    for pid in "${PIDS[@]}"; do
        kill -9 "$pid" 2>/dev/null || true
    done
    rm -rf "$(instance_dir "$INSTANCE_A")" "$(instance_dir "$INSTANCE_B")"
}
trap cleanup EXIT

start_synthetic_instance() {
    local name="$1" title="$2"
    mkdir -p "$(instance_home "$name")"

    local disp_num disp
    disp_num="$(allocate_display)"
    disp=":$disp_num"
    # Sets $START_INSTANCE_DISP as the "return value" rather than echoing +
    # capturing via $(...) - command substitution always forks a subshell, and
    # every PIDS+=() below would silently modify only that subshell's own copy
    # of the array, never the real one. Confirmed the hard way: cleanup() had
    # been finding ~0 real PIDs to kill every run, orphaning Xvfb/metacity/
    # synthetic-client processes across multiple test runs today.
    START_INSTANCE_DISP=""

    # Xvfb, headless, matching start-instance.sh's actual default backend -
    # needs the same -xkbdir fix that script now uses (see its comment for
    # why). Xephyr was used here before that fix was found; switched back to
    # Xvfb so this test exercises the real default path, and so it doesn't
    # pop visible windows on screen.
    "$XVFB_BIN" "$disp" -screen 0 640x480x24 -xkbdir /usr/share/X11/xkb >/dev/null 2>&1 &
    PIDS+=("$!")

    for _ in $(seq 1 40); do
        [ -e "/tmp/.X11-unix/X$disp_num" ] && break
        sleep 0.1
    done
    if [ ! -e "/tmp/.X11-unix/X$disp_num" ]; then
        echo "X server for '$name' never came up on $disp - aborting" >&2
        return 1
    fi

    # GDK_BACKEND=x11: see start-instance.sh's comment - WAYLAND_DISPLAY leaking
    # in from this shell's own (host) environment made GTK3 try the Wayland
    # backend by default, which metacity can't use, producing a misleading
    # "Unable to open X display" error unrelated to the actual X connection.
    DISPLAY="$disp" GDK_BACKEND=x11 metacity --replace >/dev/null 2>&1 &
    PIDS+=("$!")
    sleep 0.3

    # Output MUST be redirected here, not just for tidiness: this call is inside
    # a function whose stdout is captured via $(...) by the caller, and this
    # process never exits on its own (blocks until killed). An un-redirected
    # background process inheriting that pipe's write end keeps it open forever,
    # hanging the calling $(...) indefinitely even after $disp has already been
    # echoed - confirmed the hard way (real hang, not a hypothetical).
    DISPLAY="$disp" HOME="$(instance_home "$name")" "$CLIENT_BIN" "$title" >/dev/null 2>&1 &
    PIDS+=("$!")

    echo "$disp" > "$(instance_display_file "$name")"
    echo "own-home-marker-for-$name" > "$(instance_home "$name")/marker.txt"

    START_INSTANCE_DISP="$disp"
}

echo "=== Starting two synthetic instances (no real apps, no real accounts) ==="
start_synthetic_instance "$INSTANCE_A" "WINDOW-FROM-INSTANCE-A"
DISP_A="$START_INSTANCE_DISP"
start_synthetic_instance "$INSTANCE_B" "WINDOW-FROM-INSTANCE-B"
DISP_B="$START_INSTANCE_DISP"
if [ -z "$DISP_A" ] || [ -z "$DISP_B" ]; then
    echo "One or both synthetic instances failed to start an X server - aborting test." >&2
    exit 1
fi
echo "Instance A on $DISP_A, instance B on $DISP_B"
sleep 1

echo ""
echo "=== Check 1: each instance's own display only shows its own window ==="
LIST_A="$(DISPLAY="$DISP_A" wmctrl -l 2>/dev/null || true)"
LIST_B="$(DISPLAY="$DISP_B" wmctrl -l 2>/dev/null || true)"
echo "  wmctrl on $DISP_A: $LIST_A"
echo "  wmctrl on $DISP_B: $LIST_B"
echo "$LIST_A" | grep -q "WINDOW-FROM-INSTANCE-A"; check "instance A's display shows A's window" $?
! echo "$LIST_A" | grep -q "WINDOW-FROM-INSTANCE-B"; check "instance A's display does NOT show B's window" $?
echo "$LIST_B" | grep -q "WINDOW-FROM-INSTANCE-B"; check "instance B's display shows B's window" $?
! echo "$LIST_B" | grep -q "WINDOW-FROM-INSTANCE-A"; check "instance B's display does NOT show A's window" $?

echo ""
echo "=== Check 2 (honesty check, expected to 'fail' isolation): can a process"
echo "    that merely KNOWS the other instance's \$DISPLAY connect to it anyway? ==="
CROSS="$(DISPLAY="$DISP_B" timeout 2 wmctrl -l 2>&1)"
if echo "$CROSS" | grep -q "WINDOW-FROM-INSTANCE-B"; then
    echo "  EXPECTED: a process that sets DISPLAY=$DISP_B can see instance B's window fine."
    echo "  This confirms X11 display isolation here is BY CONVENTION (nothing tells a"
    echo "  process to connect elsewhere), NOT by OS-enforced access control like the"
    echo "  precise-input bwrap/uinput isolation is. No xauth cookie restricts these"
    echo "  Xvfb instances. Fine for this project (we control what launches into each"
    echo "  display) but worth being honest about - it is not a security boundary"
    echo "  against a hostile process running as the same local user."
else
    echo "  UNEXPECTED: cross-display connection was blocked. Something (xauth?) is"
    echo "  restricting access that wasn't expected to be there - worth investigating."
fi

echo ""
echo "=== Check 3: HOME data isolation (marker files) ==="
MARKER_A="$(cat "$(instance_home "$INSTANCE_A")/marker.txt" 2>/dev/null)"
MARKER_B="$(cat "$(instance_home "$INSTANCE_B")/marker.txt" 2>/dev/null)"
[ "$MARKER_A" = "own-home-marker-for-$INSTANCE_A" ]; check "instance A's HOME has its own marker" $?
[ "$MARKER_B" = "own-home-marker-for-$INSTANCE_B" ]; check "instance B's HOME has its own marker" $?
! grep -rq "$INSTANCE_B" "$(instance_home "$INSTANCE_A")" 2>/dev/null; check "instance A's HOME contains no reference to instance B at all" $?
! grep -rq "$INSTANCE_A" "$(instance_home "$INSTANCE_B")" 2>/dev/null; check "instance B's HOME contains no reference to instance A at all" $?

echo ""
echo "=== Summary: $FAILURES failed check(s) ==="
[ "$FAILURES" -eq 0 ]
