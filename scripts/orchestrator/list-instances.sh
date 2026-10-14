#!/usr/bin/env bash
# Lists all created instances and whether each is currently running.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

if [ ! -d "$INSTANCES_ROOT" ]; then
    echo "No instances created yet."
    exit 0
fi

shopt -s nullglob
dirs=("$INSTANCES_ROOT"/*/)
shopt -u nullglob

if [ ${#dirs[@]} -eq 0 ]; then
    echo "No instances created yet."
    exit 0
fi

for dir in "${dirs[@]}"; do
    name="$(basename "$dir")"
    pid_file="$(instance_pid_file "$name")"
    disp_file="$(instance_display_file "$name")"
    if [ -f "$pid_file" ]; then
        disp="$(cat "$disp_file" 2>/dev/null || echo '?')"
        echo "$name: running on $disp"
    else
        echo "$name: stopped"
    fi
done
