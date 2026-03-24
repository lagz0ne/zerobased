package socat

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepStale(t *testing.T) {
	baseDir := t.TempDir()
	m := New(baseDir)

	// Create fake stale socket files (simulating crash leftovers)
	projectDir := filepath.Join(baseDir, "testproject")
	os.MkdirAll(projectDir, 0755)

	// Create a Unix socket file that persists — use syscall directly
	// since Go's net.Listen("unix") removes on Close()
	staleSock := filepath.Join(projectDir, ".s.PGSQL.5432")
	fd, err := net.Listen("unix", staleSock)
	if err != nil {
		t.Fatal(err)
	}
	// Get the underlying file descriptor and close without removing
	rawConn, _ := fd.(*net.UnixListener).SyscallConn()
	rawConn.Control(func(fd uintptr) {})
	// Close listener but recreate the socket file to simulate a crash leftover
	// (Go removes socket on close, so we need to re-create)
	fd.Close()

	// Re-create the socket via a listener we abandon (simulating process death)
	fd2, err := net.Listen("unix", staleSock)
	if err != nil {
		t.Fatal(err)
	}
	// Leak the listener (don't close) — on test cleanup TempDir handles it
	_ = fd2

	// Verify socket file exists
	if _, err := os.Lstat(staleSock); err != nil {
		t.Fatal("stale socket should exist on disk")
	}

	// Sweep should remove it (no active bridges in manager)
	n := m.SweepStale()
	if n != 1 {
		t.Fatalf("expected 1 stale socket removed, got %d", n)
	}

	// Socket file should be gone
	if _, err := os.Lstat(staleSock); !os.IsNotExist(err) {
		t.Fatal("stale socket should have been removed")
	}

	// Empty project dir should also be removed
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Fatal("empty project dir should have been removed")
	}
}

func TestSweepStalePreservesActive(t *testing.T) {
	baseDir := t.TempDir()
	m := New(baseDir)

	// Start a TCP server to serve as target
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// Create an active bridge
	sockPath, err := m.Bridge("testproject", ".s.PGSQL.5432", tcpLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Remove(sockPath)

	// Sweep should NOT remove the active bridge's socket
	n := m.SweepStale()
	if n != 0 {
		t.Fatalf("expected 0 stale sockets, got %d", n)
	}

	// Socket should still exist
	if _, err := os.Lstat(sockPath); err != nil {
		t.Fatal("active socket should still exist")
	}
}

func TestBridgeReplacesExisting(t *testing.T) {
	baseDir := t.TempDir()
	m := New(baseDir)

	// Start two TCP servers
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln1.Close()
	go func() {
		for {
			c, err := ln1.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	go func() {
		for {
			c, err := ln2.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// Create bridge to first target
	sock1, err := m.Bridge("proj", "test.sock", ln1.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	// Replace bridge with second target — should not error
	sock2, err := m.Bridge("proj", "test.sock", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if sock1 != sock2 {
		t.Fatalf("socket paths should be identical: %s vs %s", sock1, sock2)
	}

	// Only one bridge should be tracked
	sockets := m.ListSockets()
	if len(sockets) != 1 {
		t.Fatalf("expected 1 bridge, got %d", len(sockets))
	}

	m.RemoveAll()
}

func TestCheckHealthRemovesMissing(t *testing.T) {
	baseDir := t.TempDir()
	m := New(baseDir)

	// Start a TCP server
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	sockPath, err := m.Bridge("proj", "test.sock", tcpLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	// Delete the socket file externally (simulates manual deletion)
	os.Remove(sockPath)

	// Health check should detect and clean up
	unhealthy := m.CheckHealth()
	if len(unhealthy) != 1 {
		t.Fatalf("expected 1 unhealthy bridge, got %d", len(unhealthy))
	}

	if len(m.ListSockets()) != 0 {
		t.Fatal("bridge should have been removed")
	}
}

func TestCheckHealthRemovesUnreachable(t *testing.T) {
	baseDir := t.TempDir()
	m := New(baseDir)

	// Start a TCP server, then close it
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := tcpLn.Addr().String()
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	sockPath, err := m.Bridge("proj", "test.sock", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = sockPath

	// Close the TCP server — target becomes unreachable
	tcpLn.Close()
	time.Sleep(50 * time.Millisecond) // let it close

	// Health check should detect unreachable target
	unhealthy := m.CheckHealth()
	if len(unhealthy) != 1 {
		t.Fatalf("expected 1 unhealthy bridge, got %d", len(unhealthy))
	}

	if len(m.ListSockets()) != 0 {
		t.Fatal("bridge should have been removed")
	}
}
