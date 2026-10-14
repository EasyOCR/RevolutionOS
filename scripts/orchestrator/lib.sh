#!/usr/bin/env bash
# Shared config/helpers for the RevolutionOS multi-instance orchestrator.
# Isolation strategy: each instance gets its own X11 display (Xephyr/Xvfb + metacity,
# matching the confirmed-working Flashback stack) and its own fake $HOME, since flatpak
# roots per-app storage at $HOME/.var/app/<id> — overriding HOME isolates both Sober's
# and RevolutionMacro's data with a single mechanism, no per-app special-casing needed.
set -euo pipefail

INSTANCES_ROOT="${REVOLUTIONOS_INSTANCES_ROOT:-$HOME/.local/share/revolutionos/instances}"
SOBER_APP_ID="org.vinegarhq.Sober"
REVOMACRO_APP_ID="com.revolutionmacro.RevolutionMacro"
DISPLAY_BASE=10

# Homebrew-installed Xvfb, since this host (Bazzite/rpm-ostree) has no system Xvfb package.
XVFB_BIN="${REVOLUTIONOS_XVFB_BIN:-/home/linuxbrew/.linuxbrew/Cellar/xorg-server/21.1.24/bin/Xvfb}"

instance_dir() {
    echo "$INSTANCES_ROOT/$1"
}

instance_home() {
    echo "$(instance_dir "$1")/home"
}

instance_display_file() {
    echo "$(instance_dir "$1")/display"
}

instance_pid_file() {
    echo "$(instance_dir "$1")/pids"
}

instance_runtime_dir() {
    echo "$(instance_dir "$1")/runtime"
}

instance_xauth_file() {
    echo "$(instance_dir "$1")/Xauthority"
}

# Sober's own single-instance self-check does a bind-if-primary-else-connect-to-primary
# handshake on 127.0.0.1:55142 (confirmed via a live bpftrace syscall capture). Every
# orchestrator instance shares the host's real network namespace (flatpak's manifest grants
# `shared=network`), so a second instance's connect() to that port was landing directly in
# a FIRST instance's real listener instead of failing cleanly - that cross-instance
# reachability is what gets misread as a "frozen" sibling.
# TRIED AND DEAD-ENDED: an LD_PRELOAD shim (port-remap-shim.c/.so still in this directory,
# unused) that would have intercepted just that one port's bind()/connect() calls - much
# smaller than full netns, but flatpak's own manual is explicit that it "always unsets or
# overrides" LD_PRELOAD (along with PATH and LD_LIBRARY_PATH) for every sandboxed launch,
# with no override flag. Confirmed directly: the var is completely absent from the
# sandboxed process's own /proc/<pid>/environ regardless of how it's passed to `flatpak
# run`. Not fixable from the launching side at all - flatpak sandboxes are hardened
# specifically against this kind of injection.
# The only thing that actually isolates loopback itself is giving each instance a real
# network namespace (veth pair + NAT to the host for genuine internet access).
# Addressing: 10.200.<display-num>.0/30 per instance, keyed off the same display number
# already allocated by allocate_display() - host end .1, namespace end .2. Veth names are
# kept short (IFNAMSIZ=16 incl. NUL) since "revolutionos-instance-name" would overflow it.
NETNS_NAT_SUBNET="10.200.0.0/16"

instance_netns_name() {
    echo "revos-$1"
}

instance_veth_host() {
    echo "rvos-h$2"
}

instance_veth_ns() {
    echo "rvos-n$2"
}

instance_netns_host_ip() {
    echo "10.200.$2.1"
}

instance_netns_ns_ip() {
    echo "10.200.$2.2"
}

# Idempotent: safe to call every start-instance.sh run. $1=instance name, $2=display number.
setup_instance_netns() {
    local name="$1" dispnum="$2"
    local ns veth_h veth_n host_ip ns_ip uplink

    ns="$(instance_netns_name "$name")"
    veth_h="$(instance_veth_host "$name" "$dispnum")"
    veth_n="$(instance_veth_ns "$name" "$dispnum")"
    host_ip="$(instance_netns_host_ip "$name" "$dispnum")"
    ns_ip="$(instance_netns_ns_ip "$name" "$dispnum")"
    uplink="$(ip route show default | awk '/default/ {print $5; exit}')"

    # Tear down first rather than checking existence piecemeal - a half-created netns from
    # a previous crashed run is worse than the small cost of a clean rebuild every launch.
    teardown_instance_netns "$name" "$dispnum" || true

    sudo ip netns add "$ns"
    sudo ip link add "$veth_h" type veth peer name "$veth_n"
    sudo ip link set "$veth_n" netns "$ns"
    sudo ip addr add "$host_ip/30" dev "$veth_h"
    sudo ip link set "$veth_h" up
    sudo ip netns exec "$ns" ip addr add "$ns_ip/30" dev "$veth_n"
    sudo ip netns exec "$ns" ip link set "$veth_n" up
    sudo ip netns exec "$ns" ip link set lo up
    sudo ip netns exec "$ns" ip route add default via "$host_ip"

    # ip netns exec auto-bind-mounts /etc/netns/<ns>/resolv.conf over /etc/resolv.conf
    # inside the namespace, if present - this is the documented mechanism, not a hack.
    # NOT a plain copy of the host's /etc/resolv.conf: this host points at 127.0.0.53
    # (systemd-resolved's stub), and loopback addresses never route out through a veth -
    # they stay local to whichever namespace generated them, so the namespace would just
    # be talking to its own (nonexistent) stub resolver. Real, directly-reachable DNS
    # servers instead, resolved via the NAT path like any other internet traffic.
    sudo mkdir -p "/etc/netns/$ns"
    printf 'nameserver 1.1.1.1\nnameserver 8.8.8.8\n' | sudo tee "/etc/netns/$ns/resolv.conf" >/dev/null

    sudo sysctl -q net.ipv4.ip_forward=1

    # One MASQUERADE rule covers the whole NAT subnet for every instance, not per-instance -
    # check-then-add since iptables has no native idempotent "insert if absent".
    if ! sudo iptables -t nat -C POSTROUTING -s "$NETNS_NAT_SUBNET" -o "$uplink" -j MASQUERADE 2>/dev/null; then
        sudo iptables -t nat -A POSTROUTING -s "$NETNS_NAT_SUBNET" -o "$uplink" -j MASQUERADE
    fi
    if ! sudo iptables -C FORWARD -i "$veth_h" -o "$uplink" -j ACCEPT 2>/dev/null; then
        sudo iptables -A FORWARD -i "$veth_h" -o "$uplink" -j ACCEPT
    fi
    if ! sudo iptables -C FORWARD -i "$uplink" -o "$veth_h" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; then
        sudo iptables -A FORWARD -i "$uplink" -o "$veth_h" -m state --state RELATED,ESTABLISHED -j ACCEPT
    fi
}

teardown_instance_netns() {
    local name="$1" dispnum="$2"
    local ns veth_h

    ns="$(instance_netns_name "$name")"
    veth_h="$(instance_veth_host "$name" "$dispnum")"

    sudo ip link del "$veth_h" 2>/dev/null || true
    sudo ip netns del "$ns" 2>/dev/null || true
    sudo rm -rf "/etc/netns/$ns" 2>/dev/null || true
}

# Runs a command inside the instance's netns, as the real user (not root) - ip netns exec
# requires root to enter the namespace, but the launched process must run as the normal
# user, not root, so this nests a second sudo drop-privilege call inside the first. Learned
# the hard way earlier this session: sudo's env_reset forces HOME to the target user's home
# regardless of explicit `sudo VAR=val` on the command line, so env vars are passed through
# an explicit `env` call instead, which isn't subject to that sudo-specific override.
netns_exec_as_user() {
    local name="$1"; shift
    local ns
    ns="$(instance_netns_name "$name")"
    sudo ip netns exec "$ns" sudo -u "$(id -un)" env "$@"
}

# Finds the lowest free display number at/above DISPLAY_BASE by checking for an existing
# X11 socket. Not race-safe against two orchestrator invocations starting simultaneously.
allocate_display() {
    local n=$DISPLAY_BASE
    while [ -e "/tmp/.X11-unix/X$n" ]; do
        n=$((n + 1))
    done
    echo "$n"
}

# The visible "windows inside windows" look - an outer "RevolutionOS: <name>" frame
# (the host's own window manager decorating the Xephyr window) with an inner "Sober" frame
# (the NESTED metacity, running inside Xephyr, decorating Sober's own window) - is two
# separate window managers each adding their own title bar to what's conceptually one
# window. Fix: strip the INNER window's decorations via the legacy _MOTIF_WM_HINTS
# property (what metacity and most WMs still respect for "no decorations, otherwise act
# normal" - still focusable/resizable, just no title bar) and maximize it to exactly fill
# the nested screen, then rename the OUTER Xephyr window - the one actually visible on the
# host desktop, and the one whose title is fully under our control via Xephyr's own
# -title flag - to "Sober: @<username>" once the account's username becomes readable from
# Sober's own local cache. Runs in the background after Sober launches; both steps are
# polling loops since neither the window nor the username exist yet at call time.
# $1=instance name, $2=nested $DISPLAY, $3=nested XAUTHORITY file, $4=host $DISPLAY.
apply_instance_window_cosmetics() {
    local name="$1" disp="$2" xauth_file="$3" host_display="$4"
    local sober_win="" tries username_file uname

    username_file="$(instance_home "$name")/.var/app/$SOBER_APP_ID/data/sober/appData/LocalStorage/appStorage.json"

    # wmctrl, not xdotool search --name: confirmed directly that xdotool's name search
    # doesn't reliably match windows in this nested metacity setup (returned nothing, even
    # unanchored, while `wmctrl -l` found the window immediately) - not worth the time
    # chasing why, wmctrl works.
    for tries in $(seq 1 60); do
        sober_win="$(DISPLAY="$disp" XAUTHORITY="$xauth_file" wmctrl -l 2>/dev/null | awk '$0 ~ / Sober$/ {print $1; exit}')"
        [ -n "$sober_win" ] && break
        sleep 0.5
    done
    if [ -n "$sober_win" ]; then
        DISPLAY="$disp" XAUTHORITY="$xauth_file" xprop -id "$sober_win" -f _MOTIF_WM_HINTS 32c \
            -set _MOTIF_WM_HINTS "0x2, 0x0, 0x0, 0x0, 0x0" 2>/dev/null || true
        DISPLAY="$disp" XAUTHORITY="$xauth_file" wmctrl -i -r "$sober_win" -b add,maximized_vert,maximized_horz 2>/dev/null || true
    fi

    for tries in $(seq 1 120); do
        if [ -f "$username_file" ]; then
            uname="$(grep -oE '"Username":"[^"]*"' "$username_file" 2>/dev/null | head -1 | sed -E 's/"Username":"([^"]*)"/\1/')"
            if [ -n "$uname" ]; then
                DISPLAY="$host_display" wmctrl -r "RevolutionOS: $name" -N "Sober: @$uname" 2>/dev/null || true
                break
            fi
        fi
        sleep 1
    done
}

require_instance_exists() {
    local name="$1"
    if [ ! -d "$(instance_dir "$name")" ]; then
        echo "No such instance: $name (run create-instance.sh first)" >&2
        exit 1
    fi
}

# Reads the Roblox @username an instance is logged in as, straight from Sober's own local
# cache - same source as the window-title cosmetics above. Echoes nothing (not an error)
# for an instance that's never logged in yet, since that's a normal, expected state right
# after create-instance.sh.
instance_username() {
    local name="$1" f
    f="$(instance_home "$name")/.var/app/$SOBER_APP_ID/data/sober/appData/LocalStorage/appStorage.json"
    [ -f "$f" ] || return 0
    grep -oE '"Username":"[^"]*"' "$f" 2>/dev/null | head -1 | sed -E 's/"Username":"([^"]*)"/\1/'
}

# Case-insensitive match (Roblox logins are case-insensitive even though the stored
# username has a canonical casing) across every created instance. Echoes the matching
# instance's name, or nothing if no instance is logged in as that account.
find_instance_by_username() {
    local want="$1" dir name got
    [ -d "$INSTANCES_ROOT" ] || return 0
    shopt -s nullglob
    for dir in "$INSTANCES_ROOT"/*/; do
        name="$(basename "$dir")"
        got="$(instance_username "$name")"
        if [ -n "$got" ] && [ "${got,,}" = "${want,,}" ]; then
            echo "$name"
            shopt -u nullglob
            return 0
        fi
    done
    shopt -u nullglob
}

# Next free "acctN" name for addinstance - internal instance names are an implementation
# detail once multisober exists (everything's addressed by Roblox username from here on),
# so this just needs to be unique, not meaningful.
next_auto_instance_name() {
    local n=1
    while [ -d "$(instance_dir "acct$n")" ]; do
        n=$((n + 1))
    done
    echo "acct$n"
}
