#!/bin/bash
# Plays the boot sound async from early in boot, fading it out once greetd
# actually comes up (i.e. real boot completion, not a fixed timer - boot time
# varies too much across hardware for a hardcoded fade point in the track
# itself to reliably line up with reaching the login screen).
set -u

AUDIO=/usr/share/revolutionos/boot-sound.mp3
SOCK=/run/revolutionos-boot-sound.sock
FADE_STEPS=15
FADE_STEP_DELAY=0.15
# Safety cap matching the track's own length - if greetd never comes up for
# some reason, don't poll forever past where the track would've ended anyway.
MAX_WAIT_TENTHS=470

rm -f "$SOCK"
mpv --no-terminal --really-quiet --input-ipc-server="$SOCK" "$AUDIO" &
mpv_pid=$!

for _ in $(seq 1 "$MAX_WAIT_TENTHS"); do
    systemctl is-active --quiet greetd.service && break
    kill -0 "$mpv_pid" 2>/dev/null || exit 0
    sleep 0.1
done

if [ -S "$SOCK" ]; then
    for i in $(seq "$FADE_STEPS" -1 0); do
        vol=$((i * 100 / FADE_STEPS))
        echo '{ "command": ["set_property", "volume", '"$vol"'] }' | socat - "$SOCK" >/dev/null 2>&1
        sleep "$FADE_STEP_DELAY"
    done
    echo '{ "command": ["quit"] }' | socat - "$SOCK" >/dev/null 2>&1
fi

wait "$mpv_pid" 2>/dev/null
rm -f "$SOCK"
