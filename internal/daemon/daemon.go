package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
	"github.com/lagz0ne/zerobased/internal/ports"
	"github.com/lagz0ne/zerobased/internal/socat"
)

// ServiceEntry tracks one exposed port of a container.
type ServiceEntry struct {
	ContainerID   string
	Project       string
	Service       string
	ContainerPort uint16
	Method        classifier.ExposeMethod
	IP            string
	RouteID       string // Caddy route ID
	SocketPath    string // socat socket path
}

// Daemon watches Docker events and manages routing.
type Daemon struct {
	docker  *docker.Client
	caddy   *caddy.Manager
	socat   *socat.Manager
	baseDir string // ~/.zerobased/sockets

	mu       sync.Mutex
	services map[string][]ServiceEntry // key: container ID
}

// Options configures the daemon.
type Options struct {
	BaseDir    string // socket base directory (~/.zerobased/sockets)
	DockerHost string // custom docker host (empty = default)
}

// New creates a daemon with the given options.
func New(opts Options) (*Daemon, error) {
	dc, err := docker.NewWithHost(opts.DockerHost)
	if err != nil {
		return nil, err
	}
	baseDir := opts.BaseDir
	if baseDir == "" {
		baseDir = DefaultBaseDir()
	}

	cm := caddy.NewFromWrapper(dc)
	sm := socat.New(baseDir)

	return &Daemon{
		docker:   dc,
		caddy:    cm,
		socat:    sm,
		baseDir:  baseDir,
		services: make(map[string][]ServiceEntry),
	}, nil
}

// Start begins the daemon: starts Caddy, scans existing containers, then watches events.
func (d *Daemon) Start(ctx context.Context) error {
	log.Println("starting caddy...")
	if err := d.caddy.Start(ctx); err != nil {
		return fmt.Errorf("start caddy: %w", err)
	}
	if err := d.caddy.EnsureHTTPServer(); err != nil {
		return fmt.Errorf("caddy http server: %w", err)
	}
	log.Println("caddy ready")

	// Scan existing running containers
	existing, err := d.docker.ListRunning(ctx)
	if err != nil {
		log.Printf("warning: failed to list running containers: %v", err)
	} else {
		for _, info := range existing {
			d.handleStart(info)
		}
	}

	// Watch for new events
	events, errs := d.docker.Events(ctx)
	log.Println("watching docker events...")

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			return fmt.Errorf("docker events: %w", err)
		case event := <-events:
			switch event.Action {
			case "start":
				info, err := d.docker.Inspect(ctx, event.Actor.ID)
				if err != nil {
					log.Printf("inspect %s: %v", event.Actor.ID[:12], err)
					continue
				}
				if info.Project == "" {
					continue // skip non-compose containers
				}
				d.handleStart(info)
			case "die":
				d.handleStop(event.Actor.ID)
			}
		}
	}
}

// Stop cleans up all routes, sockets, and the Caddy container.
func (d *Daemon) Stop(ctx context.Context) {
	d.mu.Lock()
	allEntries := make([]ServiceEntry, 0)
	for _, entries := range d.services {
		allEntries = append(allEntries, entries...)
	}
	d.services = make(map[string][]ServiceEntry)
	d.mu.Unlock()

	for _, entry := range allEntries {
		if entry.RouteID != "" {
			d.caddy.RemoveRoute(entry.RouteID)
		}
	}

	d.socat.RemoveAll()
	d.caddy.Stop(ctx)
	d.docker.Close()
}

// Services returns all tracked service entries (for ps command).
func (d *Daemon) Services() map[string][]ServiceEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Return a copy
	result := make(map[string][]ServiceEntry, len(d.services))
	for k, v := range d.services {
		entries := make([]ServiceEntry, len(v))
		copy(entries, v)
		result[k] = entries
	}
	return result
}

func (d *Daemon) handleStart(info *docker.ContainerInfo) {
	if info.IP == "" {
		log.Printf("skip %s/%s: no IP", info.Project, info.Service)
		return
	}

	var entries []ServiceEntry

	for _, pb := range info.Ports {
		if pb.Proto != "tcp" {
			continue
		}

		method := classifier.ClassifyFromLabels(pb.ContainerPort, info.Labels)
		if method == classifier.Internal {
			continue
		}

		entry := ServiceEntry{
			ContainerID:   info.ID,
			Project:       info.Project,
			Service:       info.Service,
			ContainerPort: pb.ContainerPort,
			Method:        method,
			IP:            info.IP,
		}

		target := fmt.Sprintf("%s:%d", info.IP, pb.ContainerPort)
		routeID := fmt.Sprintf("zb-%s-%s-%d", info.Project, info.Service, pb.ContainerPort)

		switch method {
		case classifier.Socket:
			filename := classifier.SocketFilename(info.Service, pb.ContainerPort)
			sockPath, err := d.socat.Bridge(info.Project, filename, target)
			if err != nil {
				log.Printf("socat %s: %v", routeID, err)
				continue
			}
			entry.SocketPath = sockPath
			ep := env.ForPort(d.baseDir, info.Project, info.Service, pb.ContainerPort, method)
			log.Printf("  socket  %s/%s:%d → %s", info.Project, info.Service, pb.ContainerPort, ep.ConnString)

		case classifier.HTTP:
			hostname := env.Hostname(info.Project, info.Service, pb.ContainerPort)
			if err := d.caddy.AddHTTPRoute(routeID, hostname, target); err != nil {
				log.Printf("caddy http %s: %v", routeID, err)
				continue
			}
			entry.RouteID = routeID
			log.Printf("  http    %s/%s:%d → http://%s", info.Project, info.Service, pb.ContainerPort, hostname)

		case classifier.Port:
			hashPort := ports.DeterministicPort(info.Project, info.Service, pb.ContainerPort)
			if err := d.caddy.AddTCPRoute(routeID, hashPort, target); err != nil {
				log.Printf("caddy tcp %s: %v", routeID, err)
				continue
			}
			entry.RouteID = routeID
			log.Printf("  port    %s/%s:%d → localhost:%d", info.Project, info.Service, pb.ContainerPort, hashPort)
		}

		entries = append(entries, entry)
	}

	if len(entries) > 0 {
		d.mu.Lock()
		d.services[info.ID] = entries
		d.mu.Unlock()
		log.Printf("registered %s/%s (%d ports)", info.Project, info.Service, len(entries))
	}
}

func (d *Daemon) handleStop(containerID string) {
	d.mu.Lock()
	entries, ok := d.services[containerID]
	if ok {
		delete(d.services, containerID)
	}
	d.mu.Unlock()

	if !ok {
		return
	}

	for _, entry := range entries {
		if entry.RouteID != "" {
			d.caddy.RemoveRoute(entry.RouteID)
		}
		if entry.SocketPath != "" {
			d.socat.Remove(entry.SocketPath)
		}
		log.Printf("  cleaned %s/%s:%d", entry.Project, entry.Service, entry.ContainerPort)
	}
	log.Printf("deregistered %s (%d ports)", containerID[:12], len(entries))
}

// BaseDir returns the socket base directory.
func (d *Daemon) BaseDir() string {
	return d.baseDir
}

// DefaultBaseDir returns the default socket base directory (~/.zerobased/sockets).
func DefaultBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zerobased", "sockets")
}
