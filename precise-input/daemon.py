#!/usr/bin/env python3
"""
Per-instance precise input daemon.

Creates a uinput virtual keyboard+mouse and listens on a Unix socket for
newline-delimited JSON commands, injecting them as real kernel input events
via evdev's UInput. Meant to run *inside* the same isolated environment
(bwrap sandbox / dedicated X11 display) as one orchestrator instance - the
socket and the device are both scoped to that one instance by construction,
not by anything this daemon does itself.

Why this exists instead of just using RevolutionMacro's existing XTest path:
XTest goes through the X server's normal event dispatch, coupled to that
display's timing/focus state. Raw uinput is lower-latency and has no
dependency on window focus at all - but raw uinput is normally a *global*
kernel device, visible to every process on the machine, which is exactly
the isolation problem this whole project's orchestrator was built to avoid
for RevolutionMacro's existing input paths. This daemon's actual contribution
isn't "precise input" (evdev/uinput is well-known) - it's proving that
precise AND isolated can coexist, which the confirmed root cause of the old
bss-automation DIY project's gamepad isolation problem said couldn't be done
with a naive approach.

Protocol (newline-delimited JSON over the Unix socket):
  {"type": "key", "code": "KEY_A", "action": "press"}
  {"type": "key", "code": "KEY_A", "action": "release"}
  {"type": "key", "code": "KEY_A", "action": "tap"}          # press+release
  {"type": "mouse_move", "dx": 10, "dy": -5}
  {"type": "mouse_button", "code": "BTN_LEFT", "action": "press"}
  {"type": "mouse_button", "code": "BTN_LEFT", "action": "release"}
Each command gets one line back: {"ok": true} or {"ok": false, "error": "..."}
"""
import json
import os
import socket
import socketserver
import sys
import threading

import evdev
from evdev import UInput, ecodes as e

# Curated rather than "every dir(e) attr starting with KEY_" - that grabbed
# some non-keycode constants the kernel's UI_SET_KEYBIT rejects with EINVAL.
# This covers letters, digits, common punctuation/control keys, and
# function keys - enough for real macro/game input, not literally every
# obscure key evdev knows about.
_LETTERS = [f"KEY_{c}" for c in "ABCDEFGHIJKLMNOPQRSTUVWXYZ"]
_DIGITS = [f"KEY_{d}" for d in "0123456789"]
_FUNCTION = [f"KEY_F{i}" for i in range(1, 13)]
_OTHER = [
    "KEY_SPACE", "KEY_ENTER", "KEY_ESC", "KEY_TAB", "KEY_BACKSPACE",
    "KEY_LEFTSHIFT", "KEY_RIGHTSHIFT", "KEY_LEFTCTRL", "KEY_RIGHTCTRL",
    "KEY_LEFTALT", "KEY_RIGHTALT",
    "KEY_UP", "KEY_DOWN", "KEY_LEFT", "KEY_RIGHT",
    "KEY_MINUS", "KEY_EQUAL", "KEY_COMMA", "KEY_DOT", "KEY_SLASH",
]
KEY_CAPS = [getattr(e, name) for name in _LETTERS + _DIGITS + _FUNCTION + _OTHER]
BTN_CAPS = [e.BTN_LEFT, e.BTN_RIGHT, e.BTN_MIDDLE]

CAPABILITIES = {
    e.EV_KEY: KEY_CAPS + BTN_CAPS,
    e.EV_REL: [e.REL_X, e.REL_Y],
}


class InputDeviceHandler:
    def __init__(self, name: str):
        self.ui = UInput(CAPABILITIES, name=name)
        self.lock = threading.Lock()

    def device_path(self) -> str | None:
        for candidate in evdev.list_devices():
            try:
                if evdev.InputDevice(candidate).name == self.ui.device.name:
                    return candidate
            except Exception:
                continue
        return None

    def handle(self, cmd: dict) -> dict:
        with self.lock:
            try:
                self._dispatch(cmd)
                return {"ok": True}
            except Exception as exc:
                return {"ok": False, "error": str(exc)}

    def _dispatch(self, cmd: dict) -> None:
        kind = cmd.get("type")
        if kind == "key":
            code = getattr(e, cmd["code"])
            action = cmd["action"]
            if action == "press":
                self.ui.write(e.EV_KEY, code, 1)
                self.ui.syn()
            elif action == "release":
                self.ui.write(e.EV_KEY, code, 0)
                self.ui.syn()
            elif action == "tap":
                self.ui.write(e.EV_KEY, code, 1)
                self.ui.syn()
                self.ui.write(e.EV_KEY, code, 0)
                self.ui.syn()
            else:
                raise ValueError(f"unknown key action: {action}")
        elif kind == "mouse_move":
            self.ui.write(e.EV_REL, e.REL_X, int(cmd["dx"]))
            self.ui.write(e.EV_REL, e.REL_Y, int(cmd["dy"]))
            self.ui.syn()
        elif kind == "mouse_button":
            code = getattr(e, cmd["code"])
            value = 1 if cmd["action"] == "press" else 0
            self.ui.write(e.EV_KEY, code, value)
            self.ui.syn()
        else:
            raise ValueError(f"unknown command type: {kind}")

    def close(self):
        self.ui.close()


def make_handler_class(device: InputDeviceHandler):
    class Handler(socketserver.StreamRequestHandler):
        def handle(self):
            for line in self.rfile:
                line = line.strip()
                if not line:
                    continue
                try:
                    cmd = json.loads(line)
                except json.JSONDecodeError as exc:
                    result = {"ok": False, "error": f"bad json: {exc}"}
                else:
                    result = device.handle(cmd)
                self.wfile.write((json.dumps(result) + "\n").encode())
    return Handler


def main():
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <instance-name> <socket-path>", file=sys.stderr)
        sys.exit(1)

    instance_name, socket_path = sys.argv[1], sys.argv[2]
    device_name = f"revolutionos-{instance_name}-input"

    if os.path.exists(socket_path):
        os.remove(socket_path)

    device = InputDeviceHandler(device_name)
    print(f"created device {device_name}", flush=True)

    path = device.device_path()
    if path:
        # We create this node, so we control its permissions - see README
        # for why this needs root (device is owned root:input regardless of
        # which uid calls UI_DEV_CREATE, confirmed on the dev machine) and
        # why relying on ambient group/ACL membership doesn't work once
        # this runs inside an isolated sandbox.
        try:
            os.chmod(path, 0o660)
        except PermissionError:
            print(f"WARNING: could not chmod {path} - run as root, or pre-grant access", file=sys.stderr)

    server = socketserver.UnixStreamServer(socket_path, make_handler_class(device))
    os.chmod(socket_path, 0o660)
    print(f"listening on {socket_path}", flush=True)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        device.close()
        server.server_close()
        if os.path.exists(socket_path):
            os.remove(socket_path)


if __name__ == "__main__":
    main()
