#!/bin/bash
set -ouex pipefail

cp -avf "/ctx/system_files"/. /

### Compiles /etc/dconf/db/local.d/* (default desktop background, etc - see
### system_files/etc/dconf/db/local.d/00-revolutionos-background) into the
### binary db dconf actually reads. Needed at build time since nothing on a
### fresh image would otherwise trigger this the way an RPM %post trigger
### does on a traditional install.
dconf update

### RevolutionOS orchestrator - the actual point of this whole image. Binary
### is already in place via the Containerfile's multi-stage COPY (built from
### ui/ in the ui-builder stage); this installs the scripts it shells out to
### (matches app.go's installedScriptsDir constant - keep these in sync) and
### makes sure the binary's executable.
###
### Scripts themselves live in the separate, private EasyOCR/precise-input
### repo now (split out 2026-10-14, along with precise-input/ itself - see
### that repo's own history for why: both are about the same thing, isolated
### multi-instance Sober macroing). Cloned here with a GitHub token from a
### BuildKit secret mount (see Containerfile) rather than baked into this
### repo's own history or image layers. Missing token = warn and skip rather
### than hard-fail the whole build, same reasoning as the RevolutionMacro
### bundle below - this still unblocks local dev builds of everything else.
if [ -s /run/secrets/github_token ]; then
    git clone --depth 1 \
        "https://x-access-token:$(cat /run/secrets/github_token)@github.com/EasyOCR/precise-input.git" \
        /tmp/precise-input
    mkdir -p /usr/libexec/revolutionos/orchestrator
    cp -avf /tmp/precise-input/orchestrator/. /usr/libexec/revolutionos/orchestrator/
    rm -rf /tmp/precise-input
    chmod +x /usr/libexec/revolutionos/orchestrator/*.sh
else
    echo "WARNING: no github_token secret provided - the orchestrator will NOT be included in this image." >&2
    echo "Pass one with: podman build --secret id=github_token,src=/path/to/token-file ..." >&2
    mkdir -p /usr/libexec/revolutionos/orchestrator
fi
### multisober has no .sh suffix (invoked as `multisober launch ...`, not
### `multisober.sh launch ...`), so the glob above skips it - chmod explicitly.
### Guarded the same as the clone above - nothing to chmod/link if the
### orchestrator wasn't fetched.
if [ -f /usr/libexec/revolutionos/orchestrator/multisober ]; then
    chmod +x /usr/libexec/revolutionos/orchestrator/multisober
    ln -sf /usr/libexec/revolutionos/orchestrator/multisober /usr/bin/multisober
fi
chmod +x /usr/libexec/revolutionos/revolutionos
chmod +x /usr/libexec/revolutionos/revolutionos-panel
chmod +x /usr/libexec/revolutionos/revolutionos-greeter

### Passwordless sudo for the orchestrator's netns setup (lib.sh's
### setup_instance_netns/teardown_instance_netns/netns_exec_as_user - see
### scripts/orchestrator/lib.sh for why this is needed: Sober's own
### single-instance self-check collides across instances on 127.0.0.1:55142
### unless each gets a real network namespace). These run as bare `sudo` with
### no code change needed on the dev machine, where an interactive terminal
### (or a YubiKey-backed passwordless policy) can satisfy the prompt - but the
### real product launches instances from the Wails GUI, which has no TTY at
### all, so an unprivileged `sudo ip netns add` would just hang forever
### waiting for a password nothing can supply. Scoped NOPASSWD entries for
### exactly the commands netns setup/teardown/exec actually run, not a blanket
### sudo grant - every image user gets this, since multi-account orchestration
### is the product's core feature, not an opt-in admin capability.
cat > /etc/sudoers.d/revolutionos-orchestrator <<'EOF'
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip netns add *, /usr/sbin/ip netns del *, /usr/sbin/ip netns exec *
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip link add * type veth peer name *
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip link del *
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip addr add * dev *
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip link set * up
%wheel ALL=(root) NOPASSWD: /usr/sbin/ip route add default via *
%wheel ALL=(root) NOPASSWD: /usr/sbin/sysctl -q net.ipv4.ip_forward=1
%wheel ALL=(root) NOPASSWD: /usr/sbin/iptables -t nat -C POSTROUTING *, /usr/sbin/iptables -t nat -A POSTROUTING *
%wheel ALL=(root) NOPASSWD: /usr/sbin/iptables -C FORWARD *, /usr/sbin/iptables -A FORWARD *
EOF
chmod 440 /etc/sudoers.d/revolutionos-orchestrator
visudo -cf /etc/sudoers.d/revolutionos-orchestrator

### Test login account (test/test) - this image has no installer/first-boot
### wizard yet to create a real user, and boot-testing in a VM needs
### something to actually log in as through the greeter. wheel membership
### matches what the orchestrator's sudoers rules above assume any real
### product user would have. Fine for a disposable pre-release test image;
### revisit if/when a real first-boot account flow exists.
useradd -m -G wheel test
echo "test:test" | chpasswd

### Dev-only: claude-code inside the image itself, so Claude can operate
### directly in a booted RevolutionOS session instead of only driving it from
### outside via screenshots/virsh. Same package already layered on the dev
### machine's own Bazzite host (confirmed via rpm-ostree status), gated here
### so it never lands in a real product build.
if [ "${DEV_BUILD:-0}" = "1" ]; then
    dnf5 install -y claude-code
fi

### Sober (Roblox client) - properly distributed via Flathub, confirmed on
### the dev machine (`flatpak info org.vinegarhq.Sober` -> Origin: flathub).
flatpak install -y --noninteractive flathub org.vinegarhq.Sober

### RevolutionMacro - NOT on Flathub, no public flatpak repo exists for it
### (confirmed: the dev machine's install has an empty remote `url=` - it was
### installed from a downloaded .flatpak bundle file, not a real repo). Since
### that bundle is Liam's proprietary binary, it isn't committed to this git
### repo - it has to be supplied locally before building this image.
###
### Before running this build, place the current RevolutionMacro .flatpak
### bundle at image/local-assets/revolution-macro.flatpak (gitignored).
if [ -f /ctx/local-assets/revolution-macro.flatpak ]; then
    flatpak install -y --noninteractive --bundle /ctx/local-assets/revolution-macro.flatpak
else
    echo "WARNING: image/local-assets/revolution-macro.flatpak not found - RevolutionMacro will NOT be pre-installed in this image." >&2
    echo "See image/build_files/build.sh for why, and PROJECT.md for the longer-term fix (ask Liam for a real flatpak repo)." >&2
fi

### Default session: GNOME Flashback/Metacity, not stock GNOME/Mutter.
### RevolutionMacro's Wayland support isn't reliable yet (per Liam); X11
### Flashback is the currently-working path, confirmed via linuxdiag showing
### xcomposite/x11-xtest backends (both correctly per-$DISPLAY scoped - see
### PROJECT.md's "Resolved: X11 (Flashback) is the actual target" section).
dnf5 install -y gnome-flashback

### No session-selection mechanism needed here (this used to be an
### AccountsService user-templates override so GDM's session picker
### defaulted to Flashback - see git history if that ever needs resurrecting).
### greetd's replacement for GDM (below) has no picker at all: our own
### greeter always starts gnome-flashback-metacity directly
### (greeter/app.go's flashbackSessionCmd), so there's nothing to default or
### mis-default in the first place.

### Replace the real gnome-panel with our own Wails-rendered one. gnome-panel
### is pulled into the Flashback session purely via a `Wants=gnome-panel.service`
### line in gnome-session@gnome-flashback-metacity.target.d/session.conf (a
### vendor drop-in, confirmed by reading it directly on the dev machine, where
### Flashback already runs reliably) - `Wants=` is a soft dependency, so
### masking the unit is enough to disable it cleanly without touching that
### vendor file. `revolutionos-panel.service` (shipped via system_files) gets
### pulled in the same way, via our own additive drop-in
### (etc/systemd/user/gnome-session@.../revolutionos-panel.conf) rather than
### editing the vendor one. Masking works at build time with no running
### systemd - it's just a symlink to /dev/null under /etc/systemd/user/.
systemctl --global mask gnome-panel.service

### Login manager: greetd + our own Wails-rendered greeter (greeter/), not
### GDM. Decided 2026-09-27 after GDM's interactive login proved genuinely
### unreliable in VM testing (see PROJECT.md's GDM investigation) and, more
### fundamentally, because GDM's GNOME-specific machinery (its own session
### picker, AccountsService-driven defaults) fights a product that replaces
### the entire UI layer anyway. greetd itself makes zero assumptions about
### what greeter runs - see /etc/greetd/config.toml (overridden via
### system_files) pointing it at our binary instead of the bundled agreety.
###
### The `greetd` system user (created by the package's own sysusers.d
### fragment, not by us) needs `video` group membership to get GPU access for
### our WebKit-rendered greeter - greetd's own default config.toml documents
### this requirement for graphical greeters in general.
###
### cage (a minimal Wayland kiosk compositor) hosts the greeter - greetd
### launches greeters on a bare VT with no display server at all (confirmed
### via a real VM boot crash, "failed to init GTK"), so something has to
### provide one. Tried `startx`/Xorg first since the rest of the product is
### X11-only, but that's a decision about the user session launched *after*
### login (a separate greetd-managed process) - the greeter itself has no
### X11-specific requirement (no EWMH dock-claiming like the panel), and
### graphical greetd greeters are overwhelmingly documented via a Wayland
### kiosk compositor rather than startx. See system_files' greetd
### config.toml for the actual `cage -s -- ...` invocation.
###
### xorg-x11-xinit provides `startx`, which greeter/app.go's
### flashbackSessionCmd wraps the *real session launch* in - confirmed by a
### real VM login (2026-09-27) that authenticated fully but then
### gnome-flashback.service died with "cannot open display: ". GDM used to
### spawn Xorg for the user session itself automatically; nothing does that
### now that it's gone, and it's a wholly separate concern from the greeter's
### own cage/Wayland display above (a different process, a different VT
### lifetime).
dnf5 install -y greetd cage xorg-x11-xinit
usermod -aG video greetd

### Disable GDM defensively rather than assuming it's present - base-nvidia
### is a generic base, not a desktop spin, so whether it ships GDM at all
### isn't guaranteed the way it was on Bazzite. `systemctl disable` on a
### wholly nonexistent unit is a hard error under this script's `set -e`, so
### check first rather than let that abort the whole build.
if systemctl list-unit-files gdm.service &>/dev/null; then
    systemctl disable gdm.service
fi
systemctl enable greetd.service

### Boot sound - plays async from early boot, faded out once greetd actually
### comes up (see usr/libexec/revolutionos/boot-sound.sh - waits for
### greetd.service to go active rather than a fixed timer, since boot time
### varies too much across hardware to hardcode when the track should end).
### mpv for playback (scriptable via its JSON IPC socket for the live
### volume-fade control that needs), socat to actually talk to that socket.
dnf5 install -y mpv socat
chmod +x /usr/libexec/revolutionos/boot-sound.sh
systemctl enable revolutionos-boot-sound.service
