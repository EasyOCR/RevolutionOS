#!/usr/bin/env bash
# Stops all processes for a named instance (X server, metacity, Sober, RevolutionMacro).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

if [ $# -ne 1 ]; then
    echo "Usage: $0 <instance-name>" >&2
    exit 1
fi

NAME="$1"
require_instance_exists "$NAME"

PID_FILE="$(instance_pid_file "$NAME")"

if [ ! -f "$PID_FILE" ]; then
    echo "Instance '$NAME' doesn't look like it's running (no $PID_FILE)." >&2
    exit 0
fi

while read -r pid; do
    [ -n "$pid" ] || continue
    if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
    fi
done < "$PID_FILE"

# Display number is needed to derive this instance's veth names for teardown - read it
# before deleting the display file.
DISPLAY_FILE="$(instance_display_file "$NAME")"
if [ -f "$DISPLAY_FILE" ]; then
    DISP_NUM="$(cat "$DISPLAY_FILE" | tr -d ':')"
    teardown_instance_netns "$NAME" "$DISP_NUM"
fi

rm -f "$PID_FILE" "$(instance_display_file "$NAME")" "$(instance_xauth_file "$NAME")"
rm -rf "$(instance_runtime_dir "$NAME")"
echo "Stopped instance '$NAME'."
