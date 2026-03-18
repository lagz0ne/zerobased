package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

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
	case "logs":
		cmdLogs()
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
  start [-d]         Start daemon (-d for detached/background mode)
  stop               Stop daemon + Caddy + cleanup all sockets
  logs [-f]          Show daemon logs (-f to follow)
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

func zerobasedDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zerobased")
}

func pidFile() string  { return filepath.Join(zerobasedDir(), "daemon.pid") }
func logFile() string  { return filepath.Join(zerobasedDir(), "daemon.log") }

func cmdStart() {
	detached := false
	for _, arg := range os.Args[2:] {
		if arg == "-d" || arg == "--detach" {
			detached = true
		}
	}

	if detached {
		// Check if already running
		if pid, err := readPID(); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					log.Fatalf("daemon already running (pid %d)", pid)
				}
			}
		}

		// Re-exec ourselves without -d, redirecting to log file
		os.MkdirAll(zerobasedDir(), 0755)
		lf, err := os.OpenFile(logFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("open log: %v", err)
		}

		// Build args without -d
		var args []string
		for _, a := range os.Args[1:] {
			if a != "-d" && a != "--detach" {
				args = append(args, a)
			}
		}

		exe, _ := os.Executable()
		cmd := exec.Command(exe, args...)
		cmd.Stdout = lf
		cmd.Stderr = lf
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := cmd.Start(); err != nil {
			log.Fatalf("start daemon: %v", err)
		}

		os.WriteFile(pidFile(), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)
		lf.Close()

		// Wait briefly to catch immediate failures
		time.Sleep(500 * time.Millisecond)
		if cmd.ProcessState != nil {
			log.Fatalf("daemon exited immediately — check %s", logFile())
		}

		fmt.Printf("daemon started (pid %d)\n", cmd.Process.Pid)
		fmt.Printf("logs: %s\n", logFile())
		return
	}

	// Foreground mode
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
	// Kill background daemon if running
	if pid, err := readPID(); err == nil {
		if process, err := os.FindProcess(pid); err == nil {
			if err := process.Signal(syscall.SIGTERM); err == nil {
				fmt.Printf("sent SIGTERM to daemon (pid %d)\n", pid)
			}
		}
		os.Remove(pidFile())
	}

	// Also clean up Caddy container
	opts := daemon.Options{
		BaseDir:    daemon.DefaultBaseDir(),
		DockerHost: dockerHost,
	}
	d, err := daemon.New(opts)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	d.Stop(context.Background())
	log.Println("stopped")
}

func cmdLogs() {
	follow := false
	for _, arg := range os.Args[2:] {
		if arg == "-f" || arg == "--follow" {
			follow = true
		}
	}

	lf := logFile()
	if _, err := os.Stat(lf); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "no daemon logs found")
		os.Exit(1)
	}

	if follow {
		cmd := exec.Command("tail", "-f", lf)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	} else {
		cmd := exec.Command("tail", "-100", lf)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFile())
	if err != nil {
		return 0, err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return 0, err
	}
	return pid, nil
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
