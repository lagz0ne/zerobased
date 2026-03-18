package socat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Manager tracks socat processes bridging Unix sockets to container IPs.
type Manager struct {
	mu        sync.Mutex
	processes map[string]*os.Process // key: socket path
	baseDir   string                 // ~/.zerobased/sockets
}

// New creates a socat manager with the given base socket directory.
func New(baseDir string) *Manager {
	return &Manager{
		processes: make(map[string]*os.Process),
		baseDir:   baseDir,
	}
}

// Bridge creates a Unix socket that forwards to a TCP address via socat.
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

	// socat UNIX-LISTEN:<sock>,fork TCP:<container-ip>:<port>
	cmd := exec.Command("socat",
		fmt.Sprintf("UNIX-LISTEN:%s,fork,mode=0666", sockPath),
		fmt.Sprintf("TCP:%s", target),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("socat start: %w", err)
	}

	m.mu.Lock()
	m.processes[sockPath] = cmd.Process
	m.mu.Unlock()

	// Reap process in background
	go cmd.Wait()

	return sockPath, nil
}

// Remove kills the socat process for a socket and removes the socket file.
func (m *Manager) Remove(sockPath string) {
	m.mu.Lock()
	proc, ok := m.processes[sockPath]
	if ok {
		delete(m.processes, sockPath)
	}
	m.mu.Unlock()

	if ok && proc != nil {
		proc.Kill()
	}
	os.Remove(sockPath)
}

// RemoveAll kills all tracked socat processes and cleans up sockets.
func (m *Manager) RemoveAll() {
	m.mu.Lock()
	procs := make(map[string]*os.Process, len(m.processes))
	for k, v := range m.processes {
		procs[k] = v
	}
	m.processes = make(map[string]*os.Process)
	m.mu.Unlock()

	for sockPath, proc := range procs {
		if proc != nil {
			proc.Kill()
		}
		os.Remove(sockPath)
	}
}

// RemoveByPrefix kills all socat processes whose socket path starts with prefix.
// Used to clean up all sockets for a project or container.
func (m *Manager) RemoveByPrefix(prefix string) {
	m.mu.Lock()
	var toRemove []string
	for sockPath := range m.processes {
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
	paths := make([]string, 0, len(m.processes))
	for p := range m.processes {
		paths = append(paths, p)
	}
	return paths
}
