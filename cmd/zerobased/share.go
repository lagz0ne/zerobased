package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/env"
)

func cmdDomain() {
	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased domain <add|rm|list> [args...]")
		os.Exit(1)
	}

	switch args[0] {
	case "add":
		cmdDomainAdd(args[1:])
	case "rm", "remove":
		cmdDomainRm(args[1:])
	case "list", "ls":
		cmdDomainList()
	default:
		fmt.Fprintf(os.Stderr, "unknown domain subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdDomainAdd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased domain add <domain> [--ttl <duration>] [--persistent]")
		os.Exit(1)
	}

	domain := ""
	ttl := daemon.DefaultTTL
	persistent := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--persistent":
			persistent = true
		case args[i] == "--ttl" && i+1 < len(args):
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				log.Fatalf("invalid TTL %q: %v", args[i+1], err)
			}
			ttl = d
			i++
		case strings.HasPrefix(args[i], "--ttl="):
			d, err := time.ParseDuration(strings.TrimPrefix(args[i], "--ttl="))
			if err != nil {
				log.Fatalf("invalid TTL: %v", err)
			}
			ttl = d
		default:
			if domain == "" {
				domain = args[i]
			}
		}
	}

	if domain == "" {
		fmt.Fprintln(os.Stderr, "usage: zerobased domain add <domain> [--ttl <duration>] [--persistent]")
		os.Exit(1)
	}

	idx, err := daemon.AddDomain(domain, ttl, persistent)
	if err != nil {
		log.Fatalf("add domain: %v", err)
	}

	if persistent {
		fmt.Printf("@%d  %s  (persistent)\n", idx, domain)
	} else {
		fmt.Printf("@%d  %s  (expires in %s)\n", idx, domain, daemon.FormatTTL(ttl))
	}

	// Signal daemon to reload domains
	signalDaemon()
}

func cmdDomainRm(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased domain rm @N")
		os.Exit(1)
	}

	if args[0] == "--all" {
		n := daemon.RemoveAllDomains()
		fmt.Printf("removed %d domain(s)\n", n)
		signalDaemon()
		return
	}

	idx := daemon.ParseDomainRef(args[0])
	if idx == 0 {
		fmt.Fprintf(os.Stderr, "invalid domain ref %q — use @N (e.g., @1)\n", args[0])
		os.Exit(1)
	}

	name, err := daemon.RemoveDomain(idx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("removed @%d %s\n", idx, name)
	signalDaemon()
}

func cmdDomainList() {
	entries := daemon.LoadDomains()
	if len(entries) == 0 {
		fmt.Println("no domains configured")
		return
	}

	for i, e := range entries {
		if e.Persistent {
			fmt.Printf("  @%d  %-35s  persistent\n", i+1, e.Domain)
		} else if e.IsExpired() {
			fmt.Printf("  @%d  %-35s  expired\n", i+1, e.Domain)
		} else {
			fmt.Printf("  @%d  %-35s  %s\n", i+1, e.Domain, daemon.FormatTTL(e.TTLRemaining()))
		}
	}
}

func cmdShare() {
	args := os.Args[2:]

	// Parse optional @N filter
	var filterIdx int
	if len(args) > 0 {
		filterIdx = daemon.ParseDomainRef(args[0])
	}

	entries := daemon.LoadDomains()
	if len(entries) == 0 {
		fmt.Println("no domains configured — use: zerobased domain add <domain>")
		return
	}

	if filterIdx > 0 {
		if filterIdx > len(entries) {
			log.Fatalf("invalid ref @%d (have %d domains)", filterIdx, len(entries))
		}
		entries = []daemon.DomainEntry{entries[filterIdx-1]}
	}

	// Get running services
	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	endpoints := env.EndpointsFromContainers(daemon.DefaultBaseDir(), containers, "")
	if len(endpoints) == 0 {
		fmt.Println("no services running")
		return
	}

	// Group by project
	grouped := make(map[string][]env.ServiceEndpoint)
	for _, ep := range endpoints {
		grouped[ep.Project] = append(grouped[ep.Project], ep)
	}

	projects := make([]string, 0, len(grouped))
	for p := range grouped {
		projects = append(projects, p)
	}
	sort.Strings(projects)

	for i, e := range entries {
		if e.IsExpired() {
			continue
		}
		idx := i + 1
		if filterIdx > 0 {
			idx = filterIdx
		}

		ttlStr := "persistent"
		if !e.Persistent {
			ttlStr = daemon.FormatTTL(e.TTLRemaining())
		}
		fmt.Printf("@%d  %s  (%s)\n", idx, e.Domain, ttlStr)

		for _, proj := range projects {
			for _, ep := range grouped[proj] {
				switch ep.Method {
				case "http":
					url := fmt.Sprintf("http://%s", env.HostnameForDomain(ep.Project, ep.Service, ep.ContainerPort, e.Domain))
					fmt.Printf("    %-15s %s\n", ep.Service, url)
				default:
					// Socket/port services don't get external URLs
				}
			}
		}
		fmt.Println()
	}
}

func cmdUnshare() {
	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased unshare @N | --all")
		os.Exit(1)
	}

	if args[0] == "--all" {
		n := daemon.RemoveAllDomains()
		fmt.Printf("removed %d domain(s)\n", n)
		signalDaemon()
		return
	}

	idx := daemon.ParseDomainRef(args[0])
	if idx == 0 {
		fmt.Fprintf(os.Stderr, "invalid ref %q — use @N (e.g., @1) or --all\n", args[0])
		os.Exit(1)
	}

	name, err := daemon.RemoveDomain(idx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("unshared @%d %s\n", idx, name)
	signalDaemon()
}

// signalDaemon sends SIGHUP to the running daemon to reload domains.
func signalDaemon() {
	pid, err := readPID()
	if err != nil {
		return // daemon not running, routes will be registered on next start
	}
	if !isZerobasedProcess(pid) {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if err := process.Signal(syscall.SIGHUP); err == nil {
		log.Printf("signaled daemon (pid %d) to reload domains", pid)
	}
}

