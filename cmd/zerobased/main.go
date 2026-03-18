package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
	"github.com/lagz0ne/zerobased/internal/run"
)

func main() {
	log.SetFlags(log.Ltime)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "run":
		cmdRun()
	case "env":
		cmdEnv()
	case "ps":
		cmdPs()
	case "get":
		cmdGet()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`zerobased — zero-config Docker service router

Usage:
  zerobased start              Start daemon (watches docker.sock, manages Caddy)
  zerobased stop               Stop daemon + Caddy + cleanup all sockets
  zerobased run [name] <cmd>   Wrap dev server, register route, cleanup on exit
  zerobased env [project]      Print connection strings for a project
  zerobased ps                 Show all discovered services across all projects
  zerobased get <service>      Print one connection string`)
}

func cmdStart() {
	baseDir := daemon.DefaultBaseDir()
	d, err := daemon.New(baseDir)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("shutting down...")
		d.Stop(ctx)
		cancel()
	}()

	log.Println("zerobased daemon starting")
	if err := d.Start(ctx); err != nil {
		if ctx.Err() == nil {
			log.Fatalf("daemon: %v", err)
		}
	}
}

func cmdStop() {
	baseDir := daemon.DefaultBaseDir()
	d, err := daemon.New(baseDir)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	ctx := context.Background()
	d.Stop(ctx)
	log.Println("stopped")
}

func cmdRun() {
	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased run [name] <command> [args...]")
		os.Exit(1)
	}

	// If first arg doesn't look like a command (no / and not in PATH), treat it as the name
	name := ""
	cmdArgs := args
	if len(args) > 1 && !strings.Contains(args[0], "/") {
		if _, err := exec.LookPath(args[0]); err != nil {
			name = args[0]
			cmdArgs = args[1:]
		}
	}

	if err := run.Run(name, cmdArgs, 0); err != nil {
		log.Fatal(err)
	}
}

// listContainers creates a docker client, lists running compose containers, and returns a cleanup func.
func listContainers() ([]*docker.ContainerInfo, func(), error) {
	dc, err := docker.New()
	if err != nil {
		return nil, nil, err
	}
	containers, err := dc.ListRunning(context.Background())
	if err != nil {
		dc.Close()
		return nil, nil, err
	}
	return containers, func() { dc.Close() }, nil
}

func cmdEnv() {
	project := ""
	if len(os.Args) > 2 {
		project = os.Args[2]
	}

	baseDir := daemon.DefaultBaseDir()
	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	var endpoints []env.ServiceEndpoint
	for _, c := range containers {
		if project != "" && c.Project != project {
			continue
		}
		for _, pb := range c.Ports {
			if pb.Proto != "tcp" {
				continue
			}
			method := classifier.ClassifyFromLabels(pb.ContainerPort, c.Labels)
			if method == classifier.Internal {
				continue
			}
			ep := env.ForPort(baseDir, c.Project, c.Service, pb.ContainerPort, method)
			endpoints = append(endpoints, ep)
		}
	}

	if len(endpoints) == 0 {
		if project != "" {
			fmt.Fprintf(os.Stderr, "no services found for project %q\n", project)
		} else {
			fmt.Fprintln(os.Stderr, "no services found")
		}
		os.Exit(1)
	}

	fmt.Print(env.PrintEndpoints(endpoints))
}

func cmdPs() {
	baseDir := daemon.DefaultBaseDir()
	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	if len(containers) == 0 {
		fmt.Println("no compose services running")
		return
	}

	// Group by project
	projects := make(map[string][]*docker.ContainerInfo)
	for _, c := range containers {
		projects[c.Project] = append(projects[c.Project], c)
	}

	projectNames := make([]string, 0, len(projects))
	for p := range projects {
		projectNames = append(projectNames, p)
	}
	sort.Strings(projectNames)

	for _, proj := range projectNames {
		fmt.Printf("\n%s:\n", proj)
		for _, c := range projects[proj] {
			for _, pb := range c.Ports {
				if pb.Proto != "tcp" {
					continue
				}
				method := classifier.ClassifyFromLabels(pb.ContainerPort, c.Labels)
				if method == classifier.Internal {
					continue
				}
				ep := env.ForPort(baseDir, c.Project, c.Service, pb.ContainerPort, method)
				fmt.Printf("  %-15s %-6s %d → %s\n", c.Service, method, pb.ContainerPort, ep.ConnString)
			}
		}
	}
}

func cmdGet() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: zerobased get <service> [port]")
		os.Exit(1)
	}

	target := os.Args[2]
	baseDir := daemon.DefaultBaseDir()
	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	for _, c := range containers {
		if c.Service != target && c.Name != target {
			continue
		}
		for _, pb := range c.Ports {
			if pb.Proto != "tcp" {
				continue
			}
			method := classifier.ClassifyFromLabels(pb.ContainerPort, c.Labels)
			if method == classifier.Internal {
				continue
			}
			ep := env.ForPort(baseDir, c.Project, c.Service, pb.ContainerPort, method)
			fmt.Println(ep.ConnString)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "service %q not found\n", target)
	os.Exit(1)
}
