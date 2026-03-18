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

// Global flags parsed before subcommand dispatch.
var (
	dockerHost string
	envPrefix  = "ZB" // default prefix for env vars; "" removes prefix
)

func main() {
	log.SetFlags(log.Ltime)

	// Parse global flags before subcommand
	args := os.Args[1:]
	for len(args) > 0 {
		switch {
		case args[0] == "-H" && len(args) > 1:
			dockerHost = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "--docker-host="):
			dockerHost = strings.TrimPrefix(args[0], "--docker-host=")
			args = args[1:]
		case args[0] == "--docker-host" && len(args) > 1:
			dockerHost = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "--prefix="):
			envPrefix = strings.TrimPrefix(args[0], "--prefix=")
			args = args[1:]
		case args[0] == "--prefix" && len(args) > 1:
			envPrefix = args[1]
			args = args[2:]
		default:
			goto dispatch
		}
	}

dispatch:
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Stash remaining args for subcommands
	os.Args = append([]string{os.Args[0]}, args...)

	switch args[0] {
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
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`zerobased — zero-config Docker service router

Usage:
  zerobased [flags] <command> [args...]

Flags:
  -H, --docker-host <host>   Docker daemon socket (default: $DOCKER_HOST or unix:///var/run/docker.sock)
  --prefix <prefix>          Env var prefix (default: ZB → ZB_POSTGRES_5432; "" → POSTGRES_5432)

Commands:
  start              Start daemon (watches docker.sock, manages Caddy)
  stop               Stop daemon + Caddy + cleanup all sockets
  run [name] <cmd>   Wrap dev server, inject ZB_* env vars, register route
  env [--export] [project]   Print connection strings (--export for shell eval)
  ps                 Show all discovered services across all projects
  get <service>      Print one connection string

Environment injection (zerobased run):
  ZB_POSTGRES_5432   postgresql://postgres@/postgres?host=~/.zerobased/sockets/...
  ZB_NATS_4222       localhost:26987
  ZB_NATS_80         http://nats-80.acountee.localhost

Shell eval:
  eval "$(zerobased env --export acountee)"`)
}

func cmdStart() {
	opts := daemon.Options{
		BaseDir:    daemon.DefaultBaseDir(),
		DockerHost: dockerHost,
	}
	d, err := daemon.New(opts)
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
	opts := daemon.Options{
		BaseDir:    daemon.DefaultBaseDir(),
		DockerHost: dockerHost,
	}
	d, err := daemon.New(opts)
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

	name := ""
	cmdArgs := args
	if len(args) > 1 && !strings.Contains(args[0], "/") {
		if _, err := exec.LookPath(args[0]); err != nil {
			name = args[0]
			cmdArgs = args[1:]
		}
	}

	if err := run.Run(run.Options{
		Name:       name,
		Args:       cmdArgs,
		DockerHost: dockerHost,
		EnvPrefix:  envPrefix,
	}); err != nil {
		log.Fatal(err)
	}
}

// listContainers creates a docker client, lists running compose containers, and returns a cleanup func.
func listContainers() ([]*docker.ContainerInfo, func(), error) {
	dc, err := docker.NewWithHost(dockerHost)
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
	export := false
	project := ""
	for _, arg := range os.Args[2:] {
		switch {
		case arg == "--export" || arg == "-e":
			export = true
		default:
			project = arg
		}
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

	if export {
		fmt.Print(env.PrintExport(envPrefix, endpoints))
	} else {
		fmt.Print(env.PrintEndpoints(endpoints))
	}
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
