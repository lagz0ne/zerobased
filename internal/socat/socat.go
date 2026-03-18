package socat

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// bridge holds a running Unix→TCP bridge.
type bridge struct {
	listener net.Listener
	done     chan struct{}
}

// Manager tracks Unix socket → TCP bridges (pure Go, no socat dependency).
type Manager struct {
	mu      sync.Mutex
	bridges map[string]*bridge // key: socket path
	baseDir string             // ~/.zerobased/sockets
}

// New creates a bridge manager with the given base socket directory.
func New(baseDir string) *Manager {
	return &Manager{
		bridges: make(map[string]*bridge),
		baseDir: baseDir,
	}
}

// Bridge creates a Unix socket that forwards to a TCP address.
// socketDir is the project-specific subdirectory (e.g., "acountee").
// filename is the socket file name (e.g., ".s.PGSQL.5432").
// target is the TCP address (e.g., "172.17.0.2:5432").
func (m *Manager) Bridge(socketDir, filename, target string) (string, error) {
	dir := filepath.Join(m.baseDir, socketDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	sockPath := filepath.Join(dir, filename)

	// Remove stale socket if it exists
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("listen %s: %w", sockPath, err)
	}

	// Make socket world-accessible
	os.Chmod(sockPath, 0666)

	b := &bridge{listener: ln, done: make(chan struct{})}

	m.mu.Lock()
	m.bridges[sockPath] = b
	m.mu.Unlock()

	// Accept loop
	go func() {
		defer close(b.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go proxy(conn, target)
		}
	}()

	return sockPath, nil
}

// proxy copies data bidirectionally between a Unix socket connection and a TCP target.
func proxy(src net.Conn, target string) {
	dst, err := net.Dial("tcp", target)
	if err != nil {
		src.Close()
		return
	}

	go func() {
		io.Copy(dst, src)
		dst.Close()
	}()
	io.Copy(src, dst)
	src.Close()
}

// Remove closes a bridge and removes the socket file.
func (m *Manager) Remove(sockPath string) {
	m.mu.Lock()
	b, ok := m.bridges[sockPath]
	if ok {
		delete(m.bridges, sockPath)
	}
	m.mu.Unlock()

	if ok && b != nil {
		b.listener.Close()
		<-b.done
	}
	os.Remove(sockPath)
}

// RemoveAll closes all bridges and cleans up sockets.
func (m *Manager) RemoveAll() {
	m.mu.Lock()
	all := make(map[string]*bridge, len(m.bridges))
	for k, v := range m.bridges {
		all[k] = v
	}
	m.bridges = make(map[string]*bridge)
	m.mu.Unlock()

	for sockPath, b := range all {
		if b != nil {
			b.listener.Close()
			<-b.done
		}
		os.Remove(sockPath)
	}
}

// RemoveByPrefix closes all bridges whose socket path starts with prefix.
func (m *Manager) RemoveByPrefix(prefix string) {
	m.mu.Lock()
	var toRemove []string
	for sockPath := range m.bridges {
		if len(sockPath) >= len(prefix) && sockPath[:len(prefix)] == prefix {
			toRemove = append(toRemove, sockPath)
		}
	}
	m.mu.Unlock()

	for _, sockPath := range toRemove {
		m.Remove(sockPath)
	}
}

// ListSockets returns all active socket paths.
func (m *Manager) ListSockets() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	paths := make([]string, 0, len(m.bridges))
	for p := range m.bridges {
		paths = append(paths, p)
	}
	return paths
}
