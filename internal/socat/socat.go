package socat

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// bridge holds a running Unix→TCP bridge.
type bridge struct {
	listener net.Listener
	target   string
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

// SweepStale removes all socket files under baseDir that exist on disk
// but have no active bridge. Called on daemon startup to clean up after crashes.
func (m *Manager) SweepStale() int {
	removed := 0
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return 0 // baseDir doesn't exist yet — nothing to sweep
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(m.baseDir, entry.Name())
		sockets, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, sock := range sockets {
			sockPath := filepath.Join(projectDir, sock.Name())
			info, err := os.Lstat(sockPath)
			if err != nil {
				continue
			}
			// Only remove Unix socket files (mode has ModeSocket bit)
			if info.Mode()&os.ModeSocket == 0 {
				continue
			}

			m.mu.Lock()
			_, active := m.bridges[sockPath]
			m.mu.Unlock()

			if !active {
				os.Remove(sockPath)
				removed++
			}
		}

		// Remove empty project directories
		remaining, _ := os.ReadDir(projectDir)
		if len(remaining) == 0 {
			os.Remove(projectDir)
		}
	}

	return removed
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

	// Close existing bridge if we're replacing it (e.g., container IP changed)
	m.mu.Lock()
	if old, exists := m.bridges[sockPath]; exists {
		old.listener.Close()
		<-old.done
		delete(m.bridges, sockPath)
	}
	m.mu.Unlock()

	// Remove stale socket file
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("listen %s: %w", sockPath, err)
	}

	// Make socket world-accessible
	os.Chmod(sockPath, 0666)

	b := &bridge{listener: ln, target: target, done: make(chan struct{})}

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
	dst, err := net.DialTimeout("tcp", target, 5*time.Second)
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
		if strings.HasPrefix(sockPath, prefix) {
			toRemove = append(toRemove, sockPath)
		}
	}
	m.mu.Unlock()

	for _, sockPath := range toRemove {
		m.Remove(sockPath)
	}
}

// CheckHealth verifies all bridges are healthy (socket exists, target reachable).
// Returns paths of unhealthy bridges that were removed.
func (m *Manager) CheckHealth() []string {
	m.mu.Lock()
	snapshot := make(map[string]*bridge, len(m.bridges))
	for k, v := range m.bridges {
		snapshot[k] = v
	}
	m.mu.Unlock()

	type result struct {
		path string
		ok   bool
	}
	ch := make(chan result, len(snapshot))

	for sockPath, b := range snapshot {
		go func(sp string, br *bridge) {
			if _, err := os.Lstat(sp); os.IsNotExist(err) {
				log.Printf("health: socket file missing: %s", sp)
				ch <- result{sp, false}
				return
			}
			conn, err := net.DialTimeout("tcp", br.target, 2*time.Second)
			if err != nil {
				log.Printf("health: target unreachable %s → %s", sp, br.target)
				ch <- result{sp, false}
				return
			}
			conn.Close()
			ch <- result{sp, true}
		}(sockPath, b)
	}

	var unhealthy []string
	for range len(snapshot) {
		if r := <-ch; !r.ok {
			unhealthy = append(unhealthy, r.path)
		}
	}

	for _, sockPath := range unhealthy {
		m.Remove(sockPath)
	}
	return unhealthy
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
