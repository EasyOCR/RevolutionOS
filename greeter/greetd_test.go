package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
)

// fakeGreetd is a minimal, independent implementation of the greetd-ipc wire
// protocol (its own read/write framing, not a reuse of greetdConn's) so the
// test actually exercises our client against a real peer speaking the real
// protocol, rather than testing our encoder against our own decoder.
type fakeGreetd struct {
	t        *testing.T
	listener net.Listener
	// handle is called once per accepted connection and drives the whole
	// exchange for that connection.
	handle func(conn net.Conn)
}

func newFakeGreetd(t *testing.T, handle func(conn net.Conn)) string {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "greetd.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	}()

	return sockPath
}

func readFrame(t *testing.T, conn net.Conn) map[string]any {
	t.Helper()
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		t.Fatalf("read length prefix: %v", err)
	}
	n := binary.LittleEndian.Uint32(lenBuf[:])
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(buf, &msg); err != nil {
		t.Fatalf("unmarshal request %q: %v", buf, err)
	}
	return msg
}

func writeFrame(t *testing.T, conn net.Conn, msg map[string]any) {
	t.Helper()
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		t.Fatalf("write length prefix: %v", err)
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
}

// TestGreetdLoginFlow drives a full realistic exchange - create_session,
// answer a secret auth prompt, start_session - against a real (if minimal)
// greetd-ipc peer, verifying our framing and message shapes are actually
// correct on the wire rather than just internally self-consistent.
func TestGreetdLoginFlow(t *testing.T) {
	var gotUsername, gotResponse string
	var gotCmd, gotEnv []string

	sock := newFakeGreetd(t, func(conn net.Conn) {
		req := readFrame(t, conn)
		if req["type"] != "create_session" {
			t.Errorf("expected create_session, got %v", req["type"])
		}
		gotUsername, _ = req["username"].(string)
		writeFrame(t, conn, map[string]any{
			"type":              "auth_message",
			"auth_message_type": "secret",
			"auth_message":      "Password:",
		})

		req = readFrame(t, conn)
		if req["type"] != "post_auth_message_response" {
			t.Errorf("expected post_auth_message_response, got %v", req["type"])
		}
		gotResponse, _ = req["response"].(string)
		writeFrame(t, conn, map[string]any{"type": "success"})

		req = readFrame(t, conn)
		if req["type"] != "start_session" {
			t.Errorf("expected start_session, got %v", req["type"])
		}
		for _, v := range req["cmd"].([]any) {
			gotCmd = append(gotCmd, v.(string))
		}
		for _, v := range req["env"].([]any) {
			gotEnv = append(gotEnv, v.(string))
		}
		writeFrame(t, conn, map[string]any{"type": "success"})
	})

	g, err := dialGreetdAt(sock)
	if err != nil {
		t.Fatalf("dialGreetdAt: %v", err)
	}
	defer g.Close()

	resp, err := g.createSession("revuser")
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if resp["type"] != "auth_message" || resp["auth_message_type"] != "secret" {
		t.Fatalf("unexpected create_session response: %+v", resp)
	}

	resp, err = g.postAuthMessageResponse("hunter2")
	if err != nil {
		t.Fatalf("postAuthMessageResponse: %v", err)
	}
	if resp["type"] != "success" {
		t.Fatalf("expected success after auth, got %+v", resp)
	}

	resp, err = g.startSession([]string{"gnome-session", "--session=gnome-flashback-metacity"}, []string{"XDG_SESSION_TYPE=x11"})
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	if resp["type"] != "success" {
		t.Fatalf("expected success after start_session, got %+v", resp)
	}

	if gotUsername != "revuser" {
		t.Errorf("server saw username %q, want revuser", gotUsername)
	}
	if gotResponse != "hunter2" {
		t.Errorf("server saw auth response %q, want hunter2", gotResponse)
	}
	if len(gotCmd) != 2 || gotCmd[0] != "gnome-session" {
		t.Errorf("server saw cmd %v, want gnome-session --session=...", gotCmd)
	}
	if len(gotEnv) != 1 || gotEnv[0] != "XDG_SESSION_TYPE=x11" {
		t.Errorf("server saw env %v, want [XDG_SESSION_TYPE=x11]", gotEnv)
	}
}

// TestGreetdAuthError verifies a rejected login surfaces as an error response
// rather than the client hanging or misparsing it as success.
func TestGreetdAuthError(t *testing.T) {
	sock := newFakeGreetd(t, func(conn net.Conn) {
		readFrame(t, conn) // create_session
		writeFrame(t, conn, map[string]any{
			"type":              "auth_message",
			"auth_message_type": "secret",
			"auth_message":      "Password:",
		})
		readFrame(t, conn) // post_auth_message_response
		writeFrame(t, conn, map[string]any{
			"type":        "error",
			"error_type":  "auth_error",
			"description": "authentication failed",
		})
	})

	g, err := dialGreetdAt(sock)
	if err != nil {
		t.Fatalf("dialGreetdAt: %v", err)
	}
	defer g.Close()

	if _, err := g.createSession("revuser"); err != nil {
		t.Fatalf("createSession: %v", err)
	}
	resp, err := g.postAuthMessageResponse("wrongpass")
	if err != nil {
		t.Fatalf("postAuthMessageResponse: %v", err)
	}
	if resp["type"] != "error" || resp["error_type"] != "auth_error" {
		t.Fatalf("expected auth_error, got %+v", resp)
	}
}
