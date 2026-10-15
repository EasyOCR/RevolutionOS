package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

// greetdConn wraps a connection to greetd's IPC socket, implementing the
// greetd-ipc wire protocol: a 4-byte little-endian length prefix followed by
// a UTF-8 JSON payload, in both directions - confirmed against the
// greetd-ipc(7) manpage's hexdump (0x2c 00 00 00 = 44 bytes, little-endian)
// rather than assumed, since a wrong framing detail here would just hang on
// read instead of failing loudly.
type greetdConn struct {
	conn net.Conn
}

// dialGreetd connects to the socket greetd hands the greeter via
// $GREETD_SOCK. Only meaningful when actually launched by greetd - see
// greeter/README or /etc/greetd/config.toml in system_files.
func dialGreetd() (*greetdConn, error) {
	return dialGreetdAt(os.Getenv("GREETD_SOCK"))
}

func dialGreetdAt(sock string) (*greetdConn, error) {
	if sock == "" {
		return nil, fmt.Errorf("GREETD_SOCK not set - not running under greetd")
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("dial greetd socket: %w", err)
	}
	return &greetdConn{conn: c}, nil
}

func (g *greetdConn) Close() error {
	return g.conn.Close()
}

func (g *greetdConn) roundTrip(req any) (map[string]any, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := g.conn.Write(lenBuf[:]); err != nil {
		return nil, fmt.Errorf("write length prefix: %w", err)
	}
	if _, err := g.conn.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	var respLenBuf [4]byte
	if _, err := io.ReadFull(g.conn, respLenBuf[:]); err != nil {
		return nil, fmt.Errorf("read response length: %w", err)
	}
	respLen := binary.LittleEndian.Uint32(respLenBuf[:])
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(g.conn, respBuf); err != nil {
		return nil, fmt.Errorf("read response payload: %w", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response %q: %w", respBuf, err)
	}
	return resp, nil
}

func (g *greetdConn) createSession(username string) (map[string]any, error) {
	return g.roundTrip(map[string]any{
		"type":     "create_session",
		"username": username,
	})
}

func (g *greetdConn) postAuthMessageResponse(response string) (map[string]any, error) {
	return g.roundTrip(map[string]any{
		"type":     "post_auth_message_response",
		"response": response,
	})
}

func (g *greetdConn) startSession(cmd []string, env []string) (map[string]any, error) {
	return g.roundTrip(map[string]any{
		"type": "start_session",
		"cmd":  cmd,
		"env":  env,
	})
}

func (g *greetdConn) cancelSession() (map[string]any, error) {
	return g.roundTrip(map[string]any{
		"type": "cancel_session",
	})
}
