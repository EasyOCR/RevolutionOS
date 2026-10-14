#!/usr/bin/env bash
# Creates a new isolated instance (just the directory structure + fake $HOME).
# Doesn't launch anything. After creating one, use start-instance.sh, then log into
# the desired Roblox account inside that instance's Sober before relying on it.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

if [ $# -ne 1 ]; then
    echo "Usage: $0 <instance-name>" >&2
    exit 1
fi

NAME="$1"
DIR="$(instance_dir "$NAME")"

if [ -d "$DIR" ]; then
    echo "Instance '$NAME' already exists at $DIR" >&2
    exit 1
fi

mkdir -p "$(instance_home "$NAME")"

# Flatpak user-level overrides are stored per-$HOME - Sober's existing device=input override
# (set for the real user at some point) never applies to an isolated instance's own $HOME, and
# Sober detects the mismatch itself and refuses to get past startup until this is applied for
# that specific $HOME too. Confirmed the hard way: every instance died ~2s into startup until
# this was applied, traced via Sober's own "Permission Update Required" dialog, not guessed.
HOME="$(instance_home "$NAME")" flatpak override --user --device=input "$SOBER_APP_ID"

echo "Created instance '$NAME' with isolated home at $(instance_home "$NAME")"
echo "Next: $SCRIPT_DIR/start-instance.sh $NAME, then log into the desired Roblox account."
