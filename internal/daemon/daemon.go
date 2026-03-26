package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
	"github.com/lagz0ne/zerobased/internal/ports"
	"github.com/lagz0ne/zerobased/internal/routes"
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
	docker   *docker.Client
	caddy    *caddy.Manager
	socat    *socat.Manager
	baseDir  string   // ~/.zerobased/sockets
	profiles []string // route profiles

	mu         sync.Mutex
	services   map[string][]ServiceEntry // key: container ID
	routefiles map[string]*routes.File   // key: project name, nil = no routefile
	domains    []DomainEntry             // external domains for multi-domain routing
}

// Options configures the daemon.
type Options struct {
	BaseDir    string   // socket base directory (~/.zerobased/sockets)
	DockerHost string   // custom docker host (empty = default)
	Profiles   []string // route profiles (empty = "default")
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

	d := &Daemon{
		docker:     dc,
		caddy:      cm,
		socat:      sm,
		baseDir:    baseDir,
		profiles:   opts.Profiles,
		services:   make(map[string][]ServiceEntry),
		routefiles: make(map[string]*routes.File),
	}
	d.domains = LoadDomains()
	return d, nil
}

// Start begins the daemon: starts Caddy, scans existing containers, then watches events.
func (d *Daemon) Start(ctx context.Context) error {
	// Sweep stale sockets left by a previous crash
	if n := d.socat.SweepStale(); n > 0 {
		log.Printf("swept %d stale socket(s) from previous run", n)
	}

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

	// Log domains on start
	if len(d.domains) > 0 {
		for i, de := range d.domains {
			if de.Persistent {
				log.Printf("domain @%d: %s (persistent)", i+1, de.Domain)
			} else {
				log.Printf("domain @%d: %s (%s)", i+1, de.Domain, FormatTTL(de.TTLRemaining()))
			}
		}
	}

	// Start periodic health check for bridges
	go d.healthLoop(ctx)

	// Handle SIGHUP for domain reload
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sighup:
				log.Println("SIGHUP received — reloading domains")
				d.ReloadDomains()
			}
		}
	}()

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
	domains := DomainNames(d.domains)
	d.mu.Unlock()
	for _, entry := range allEntries {
		if entry.RouteID != "" {
			d.caddy.RemoveRoute(entry.RouteID)
			for _, domain := range domains {
				d.caddy.RemoveRoute(domainRouteID(entry.RouteID, domain))
			}
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

// loadRoutefile loads and caches the routefile for a project.
// Returns nil if no routefile exists.
func (d *Daemon) loadRoutefile(info *docker.ContainerInfo) *routes.File {
	d.mu.Lock()
	rf, cached := d.routefiles[info.Project]
	d.mu.Unlock()

	if cached {
		return rf
	}

	// Look for zerobased.routes in the compose project working directory
	workDir := info.Labels["com.docker.compose.project.working_dir"]
	if workDir == "" {
		d.mu.Lock()
		d.routefiles[info.Project] = nil
		d.mu.Unlock()
		return nil
	}

	rf, err := routes.LoadWithProfile(workDir, d.profiles)
	if err != nil {
		log.Printf("warning: %s routefile: %v", info.Project, err)
		rf = nil
	}
	if rf != nil {
		rf.Gateway = fmt.Sprintf("%s.localhost", info.Project)
		log.Printf("loaded routefile for %s → gateway %s (%d routes)", info.Project, rf.Gateway, len(rf.Entries))

		// Register external routes immediately (they don't depend on containers)
		d.registerExternalRoutes(rf, info.Project)
	}

	d.mu.Lock()
	d.routefiles[info.Project] = rf
	d.mu.Unlock()
	return rf
}

// registerExternalRoutes registers Caddy routes for external targets (https, wss)
// and socat bridges for external TCP targets (postgres, nats, redis).
func (d *Daemon) registerExternalRoutes(rf *routes.File, project string) {
	group := fmt.Sprintf("zb-gw-%s", project)

	for _, entry := range rf.Entries {
		if !entry.Target.External {
			continue
		}

		routeID := fmt.Sprintf("zb-%s-ext-%s", project, strings.ReplaceAll(entry.Path, "/", "_"))
		dialAddr := fmt.Sprintf("%s:%s", entry.Target.Host, entry.Target.Port)

		switch entry.Target.Scheme {
		case "https", "wss":
			if err := d.caddy.AddExternalRoute(routeID, rf.Gateway, entry.Path, dialAddr, group); err != nil {
				log.Printf("caddy ext %s: %v", routeID, err)
				continue
			}
			log.Printf("  extern  %s → %s%s", entry.Target.Raw, rf.Gateway, entry.Path)

		case "postgres", "nats", "redis":
			filename := fmt.Sprintf("%s-%s.sock", entry.Target.Scheme, entry.Target.Port)
			sockPath, err := d.socat.Bridge(project, filename, dialAddr)
			if err != nil {
				log.Printf("socat ext %s: %v", routeID, err)
				continue
			}
			log.Printf("  extern  %s → %s", entry.Target.Raw, sockPath)
		}
	}
}

func (d *Daemon) handleStart(info *docker.ContainerInfo) {
	if info.IP == "" {
		log.Printf("skip %s/%s: no IP", info.Project, info.Service)
		return
	}

	rf := d.loadRoutefile(info)

	// Snapshot domains under lock to avoid data race with ReloadDomains/sweepExpiredDomains
	d.mu.Lock()
	domainSnapshot := make([]DomainEntry, len(d.domains))
	copy(domainSnapshot, d.domains)
	d.mu.Unlock()

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
			// If routefile exists and has an entry for this service, use path routing
			if rf != nil {
				if re := rf.FindService(info.Service); re != nil {
					group := fmt.Sprintf("zb-gw-%s", info.Project)
					if err := d.caddy.AddPathRoute(routeID, rf.Gateway, re.Path, target, group); err != nil {
						log.Printf("caddy path %s: %v", routeID, err)
						continue
					}
					entry.RouteID = routeID
					log.Printf("  path    %s/%s:%d → http://%s%s", info.Project, info.Service, pb.ContainerPort, rf.Gateway, re.Path)
					// Register domain variants for path routing
					for _, de := range domainSnapshot {
						if de.IsExpired() {
							continue
						}
						extRouteID := domainRouteID(routeID, de.Domain)
						gateway := env.GatewayForDomain(info.Project, de.Domain)
						extGroup := fmt.Sprintf("zb-gw-%s-ext-%s", info.Project, de.Domain)
						d.caddy.AddPathRoute(extRouteID, gateway, re.Path, target, extGroup)
						log.Printf("  path    %s/%s:%d → http://%s%s (ext)", info.Project, info.Service, pb.ContainerPort, gateway, re.Path)
					}
					break
				}
			}
			// Fallback: hostname-based routing (no routefile or service not in routefile)
			hostname := env.Hostname(info.Project, info.Service, pb.ContainerPort)
			if err := d.caddy.AddHTTPRoute(routeID, hostname, target); err != nil {
				log.Printf("caddy http %s: %v", routeID, err)
				continue
			}
			entry.RouteID = routeID
			log.Printf("  http    %s/%s:%d → http://%s", info.Project, info.Service, pb.ContainerPort, hostname)
			// Register domain variants for hostname routing
			for _, de := range domainSnapshot {
				if de.IsExpired() {
					continue
				}
				extRouteID := domainRouteID(routeID, de.Domain)
				extHostname := env.HostnameForDomain(info.Project, info.Service, pb.ContainerPort, de.Domain)
				d.caddy.AddHTTPRoute(extRouteID, extHostname, target)
				log.Printf("  http    %s/%s:%d → http://%s (ext)", info.Project, info.Service, pb.ContainerPort, extHostname)
			}

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
	domains := DomainNames(d.domains)
	d.mu.Unlock()

	if !ok {
		return
	}

	project := entries[0].Project
	for _, entry := range entries {
		if entry.RouteID != "" {
			d.caddy.RemoveRoute(entry.RouteID)
			// Clean up domain route variants
			for _, domain := range domains {
				d.caddy.RemoveRoute(domainRouteID(entry.RouteID, domain))
			}
		}
		if entry.SocketPath != "" {
			d.socat.Remove(entry.SocketPath)
		}
		log.Printf("  cleaned %s/%s:%d", entry.Project, entry.Service, entry.ContainerPort)
	}

	// If no more containers for this project, clear cached routefile
	if project != "" {
		d.mu.Lock()
		hasProject := false
		for _, svcEntries := range d.services {
			if len(svcEntries) > 0 && svcEntries[0].Project == project {
				hasProject = true
				break
			}
		}
		if !hasProject {
			delete(d.routefiles, project)
		}
		d.mu.Unlock()
	}

	log.Printf("deregistered %s (%d ports)", containerID[:12], len(entries))
}

// healthLoop periodically checks bridge health and cleans up broken bridges.
// When unhealthy bridges are found, the corresponding service entries are removed
// so they get re-created on the next container start event.
func (d *Daemon) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sweepExpiredDomains()
			unhealthy := d.socat.CheckHealth()
			if len(unhealthy) == 0 {
				continue
			}

			unhealthySet := make(map[string]bool, len(unhealthy))
			for _, p := range unhealthy {
				unhealthySet[p] = true
			}

			d.mu.Lock()
			for cid, entries := range d.services {
				var kept []ServiceEntry
				for _, e := range entries {
					if e.SocketPath != "" && unhealthySet[e.SocketPath] {
						log.Printf("health: removed stale entry %s/%s:%d", e.Project, e.Service, e.ContainerPort)
						continue
					}
					kept = append(kept, e)
				}
				if len(kept) == 0 {
					delete(d.services, cid)
				} else {
					d.services[cid] = kept
				}
			}
			d.mu.Unlock()
		}
	}
}

// Domains returns a copy of the current domain entries.
func (d *Daemon) Domains() []DomainEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]DomainEntry, len(d.domains))
	copy(result, d.domains)
	return result
}

// ReloadDomains re-reads the domains file and reconciles Caddy routes.
// Called on SIGHUP from domain add/rm commands.
func (d *Daemon) ReloadDomains() {
	newEntries := LoadDomains()

	d.mu.Lock()
	oldDomains := DomainNames(d.domains)
	d.domains = newEntries
	newDomains := DomainNames(d.domains)

	// Collect all current service entries
	var allEntries []ServiceEntry
	for _, entries := range d.services {
		allEntries = append(allEntries, entries...)
	}
	routefiles := d.routefiles
	d.mu.Unlock()

	// Build sets for diff
	oldSet := make(map[string]bool, len(oldDomains))
	for _, dom := range oldDomains {
		oldSet[dom] = true
	}
	newSet := make(map[string]bool, len(newDomains))
	for _, dom := range newDomains {
		newSet[dom] = true
	}

	// Deregister routes for removed domains
	for _, domain := range oldDomains {
		if newSet[domain] {
			continue
		}
		for _, entry := range allEntries {
			if entry.RouteID != "" {
				d.caddy.RemoveRoute(domainRouteID(entry.RouteID, domain))
			}
		}
		log.Printf("domain removed: %s (routes deregistered)", domain)
	}

	// Register routes for added domains
	for _, domain := range newDomains {
		if oldSet[domain] {
			continue
		}
		for _, entry := range allEntries {
			if entry.RouteID == "" || entry.Method != classifier.HTTP {
				continue
			}
			target := fmt.Sprintf("%s:%d", entry.IP, entry.ContainerPort)
			extRouteID := domainRouteID(entry.RouteID, domain)

			rf := routefiles[entry.Project]
			if rf != nil {
				if re := rf.FindService(entry.Service); re != nil {
					gateway := env.GatewayForDomain(entry.Project, domain)
					group := fmt.Sprintf("zb-gw-%s-ext-%s", entry.Project, domain)
					d.caddy.AddPathRoute(extRouteID, gateway, re.Path, target, group)
					continue
				}
			}
			hostname := env.HostnameForDomain(entry.Project, entry.Service, entry.ContainerPort, domain)
			d.caddy.AddHTTPRoute(extRouteID, hostname, target)
		}
		log.Printf("domain added: %s (routes registered for %d services)", domain, len(allEntries))
	}
}

// sweepExpiredDomains removes expired domains and their routes.
func (d *Daemon) sweepExpiredDomains() {
	removed := SweepExpired()
	if len(removed) == 0 {
		return
	}

	d.mu.Lock()
	d.domains = LoadDomains()
	var allEntries []ServiceEntry
	for _, entries := range d.services {
		allEntries = append(allEntries, entries...)
	}
	d.mu.Unlock()

	for _, domain := range removed {
		for _, entry := range allEntries {
			if entry.RouteID != "" {
				d.caddy.RemoveRoute(domainRouteID(entry.RouteID, domain))
			}
		}
		log.Printf("domain expired: %s (routes cleaned up)", domain)
	}
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
