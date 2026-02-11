package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestCorePingPong(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "core.sock")
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx)
	}()

	var conn net.Conn
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("unix", socket)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("failed to connect to uds server")
	}
	cmd := "{\"v\":\"1\",\"id\":\"req-1\",\"type\":\"ping\",\"payload\":{}}\n"
	if _, err := conn.Write([]byte(cmd)); err != nil {
		t.Fatalf("failed to write ping command: %v", err)
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}

	if resp["type"] != "pong" {
		t.Fatalf("expected type=pong, got: %v", resp["type"])
	}
	if resp["id"] != "req-1" {
		t.Fatalf("expected id=req-1, got: %v", resp["id"])
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %v", resp["ok"])
	}

	_ = conn.Close()
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestCoreInvalidCommand(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "core.sock")
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	var conn net.Conn
	for i := 0; i < 100; i++ {
		c, err := net.Dial("unix", socket)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("failed to connect to uds server")
	}
	if _, err := fmt.Fprintln(conn, `{"v":"1","id":"bad-1","type":"unknown","payload":{}}`); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["type"] != "error" {
		t.Fatalf("expected type=error got %v", resp["type"])
	}
	if resp["ok"] != false {
		t.Fatalf("expected ok=false got %v", resp["ok"])
	}

	_ = conn.Close()
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}
