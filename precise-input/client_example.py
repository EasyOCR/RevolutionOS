#!/usr/bin/env python3
"""
Minimal example client for precise-input's daemon.py. The protocol is just
newline-delimited JSON over a Unix socket, so a client in any language
(including Go, for RevolutionMacro itself) is a handful of lines - this is
here as a runnable reference, not a required dependency.
"""
import json
import socket
import sys


class PreciseInput:
    def __init__(self, socket_path: str):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(socket_path)
        self.buf = self.sock.makefile("rwb")

    def _send(self, cmd: dict) -> dict:
        self.buf.write((json.dumps(cmd) + "\n").encode())
        self.buf.flush()
        return json.loads(self.buf.readline())

    def key_tap(self, code: str) -> dict:
        return self._send({"type": "key", "code": code, "action": "tap"})

    def key_press(self, code: str) -> dict:
        return self._send({"type": "key", "code": code, "action": "press"})

    def key_release(self, code: str) -> dict:
        return self._send({"type": "key", "code": code, "action": "release"})

    def mouse_move(self, dx: int, dy: int) -> dict:
        return self._send({"type": "mouse_move", "dx": dx, "dy": dy})

    def mouse_click(self, button: str = "BTN_LEFT") -> dict:
        self._send({"type": "mouse_button", "code": button, "action": "press"})
        return self._send({"type": "mouse_button", "code": button, "action": "release"})

    def close(self):
        self.sock.close()


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <socket-path>", file=sys.stderr)
        sys.exit(1)

    client = PreciseInput(sys.argv[1])
    print(client.key_tap("KEY_A"))
    print(client.mouse_move(10, 0))
    client.close()
