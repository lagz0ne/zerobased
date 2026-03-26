package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
	"github.com/lagz0ne/zerobased/internal/run"
)

// Global flags parsed before subcommand dispatch.
var (
	dockerHost string
	envPrefix  = "ZB" // default prefix for env vars; "" removes prefix
	profile    string // route profiles (comma-separated)
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
		case strings.HasPrefix(args[0], "--profile="):
			profile = strings.TrimPrefix(args[0], "--profile=")
			args = args[1:]
		case args[0] == "--profile" && len(args) > 1:
			profile = args[1]
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
	case "domain":
		cmdDomain()
	case "share":
		cmdShare()
	case "unshare":
		cmdUnshare()
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
  --profile <names>          Route profiles from zerobased.routes.yaml (comma-separated, e.g. staging,debug)

Commands:
  start [-d]                               Start daemon (-d for background)
  stop                                     Stop daemon + cleanup
  logs [-f]                                Show daemon logs (-f to follow)
  run [-p port] [name] <cmd>               Wrap dev server, register route
  env [--export] [project]                 Print connection strings
  ps                                       Show all discovered services
  get <service> [-t template] [-v k=v]     Print one connection string
  domain add <domain> [--ttl 2h]           Add external domain (default TTL: 4h)
  domain add <domain> --persistent         Add domain that never expires
  domain list                              Show configured domains + TTL
  domain rm @N                             Remove domain by index
  share [@N]                               Show shareable URLs for all/one domain
  unshare @N | --all                       Remove domain(s) + deregister routes

Routefile (zerobased.routes — path-based gateway):
  /api     api               myapp.localhost/api → api container
  /ws      ws                myapp.localhost/ws  → ws container
  /        frontend          myapp.localhost/    → frontend container

Routefile with profiles (zerobased.routes.yaml):
  profiles:
    default:
      routes:
        /api: api
        /: frontend
    debug:
      extends: [default]
      routes:
        /db: postgres://staging-db:5432

  External targets: https://, wss://, postgres://, nats://, redis://
  Usage: zerobased start --profile debug

Templates (zerobased get -t):
  zerobased get postgres -t 'postgresql://{{user}}:{{pass}}@/{{db}}?host={{socket_dir}}' \
    -v user=postgres -v pass=secret -v db=mydb

  Variables: project, service, container_port, method, conn, url,
             socket, socket_dir (socket types), host, port (http/port types)

Shell eval:
  eval "$(zerobased env --export acountee)"`)
}

var cachedZBDir string

func zerobasedDir() string {
	if cachedZBDir == "" {
		home, _ := os.UserHomeDir()
		cachedZBDir = filepath.Join(home, ".zerobased")
	}
	return cachedZBDir
}

func pidFile() string { return filepath.Join(zerobasedDir(), "daemon.pid") }
func logFile() string { return filepath.Join(zerobasedDir(), "daemon.log") }

func cmdStart() {
	detached := false
	for _, arg := range os.Args[2:] {
		if arg == "-d" || arg == "--detach" {
			detached = true
		}
	}

	if detached {
		// Check if already running — validate PID is actually a zerobased process
		if pid, err := readPID(); err == nil {
			if isZerobasedProcess(pid) {
				log.Fatalf("daemon already running (pid %d)", pid)
			}
			// Stale PID file — clean it up
			os.Remove(pidFile())
			log.Printf("removed stale pid file (pid %d no longer running)", pid)
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

		fmt.Printf("daemon started (pid %d) — Ctrl+C to detach\n", cmd.Process.Pid)

		// Follow logs until Ctrl+C (daemon keeps running)
		followLogs()
	}

	// Foreground mode
	opts := daemon.Options{
		BaseDir:    daemon.DefaultBaseDir(),
		DockerHost: dockerHost,
		Profiles:   parseProfiles(),
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
	// Kill background daemon if running — verify it's actually zerobased
	if pid, err := readPID(); err == nil {
		if isZerobasedProcess(pid) {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.SIGTERM); err == nil {
					fmt.Printf("sent SIGTERM to daemon (pid %d)\n", pid)
				}
			}
		} else {
			log.Printf("stale pid file (pid %d is not zerobased), removing", pid)
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
	lines := 100
	for i := 2; i < len(os.Args); i++ {
		switch {
		case os.Args[i] == "-f" || os.Args[i] == "--follow":
			follow = true
		case strings.HasPrefix(os.Args[i], "-n") && len(os.Args[i]) > 2:
			fmt.Sscanf(os.Args[i][2:], "%d", &lines)
		case os.Args[i] == "-n" && i+1 < len(os.Args):
			fmt.Sscanf(os.Args[i+1], "%d", &lines)
			i++
		}
	}

	showLogs(lines, follow)
}

// followLogs shows recent logs then follows. Ctrl+C returns (doesn't kill daemon).
func followLogs() {
	showLogs(20, true)
}

func showLogs(lines int, follow bool) {
	f, err := os.Open(logFile())
	if err != nil {
		if follow {
			// Log file may not exist yet — wait for it
			for i := 0; i < 30; i++ {
				time.Sleep(200 * time.Millisecond)
				f, err = os.Open(logFile())
				if err == nil {
					break
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "no daemon logs found")
			return
		}
	}
	defer f.Close()

	printLastLines(f, lines)

	if !follow {
		return
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	buf := make([]byte, 4096)
	for {
		select {
		case <-sigs:
			signal.Stop(sigs)
			fmt.Println() // clean line after ^C
			return
		default:
			n, _ := f.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
			} else {
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
}

// printLastLines prints the last n lines from a file, seeking from the end.
func printLastLines(f *os.File, n int) {
	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return
	}

	// Read from end to find n newlines
	size := stat.Size()
	bufSize := int64(8192)
	if bufSize > size {
		bufSize = size
	}

	found := 0
	offset := size
	for offset > 0 && found <= n {
		readSize := bufSize
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize
		buf := make([]byte, readSize)
		f.ReadAt(buf, offset)
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				found++
				if found > n {
					// Print from this position
					offset += int64(i) + 1
					break
				}
			}
		}
	}

	f.Seek(offset, 0)
	io.Copy(os.Stdout, f)
}

func isZerobasedProcess(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return true // can't read cmdline (not Linux) — assume ours since PID file exists
	}
	return strings.Contains(string(cmdline), "zerobased")
}

// parseProfiles splits the --profile flag into a string slice.
func parseProfiles() []string {
	if profile == "" {
		return nil
	}
	var result []string
	for _, p := range strings.Split(profile, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
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
		fmt.Fprintln(os.Stderr, "usage: zerobased run [-p port] [name] <command> [args...]")
		os.Exit(1)
	}

	// Parse run-specific flags
	port := 0
	var remaining []string
	for i := 0; i < len(args); i++ {
		if (args[i] == "-p" || args[i] == "--port") && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &port)
			i++
		} else {
			remaining = append(remaining, args[i])
		}
	}
	args = remaining

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
		Port:       port,
		DockerHost: dockerHost,
		EnvPrefix:  envPrefix,
		Profiles:   parseProfiles(),
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

	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	endpoints := env.EndpointsFromContainers(daemon.DefaultBaseDir(), containers, project)

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
	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	endpoints := env.EndpointsFromContainers(daemon.DefaultBaseDir(), containers, "")
	if len(endpoints) == 0 {
		fmt.Println("no compose services running")
		return
	}

	// Group by project
	grouped := make(map[string][]env.ServiceEndpoint)
	for _, ep := range endpoints {
		grouped[ep.Project] = append(grouped[ep.Project], ep)
	}

	projectNames := make([]string, 0, len(grouped))
	for p := range grouped {
		projectNames = append(projectNames, p)
	}
	sort.Strings(projectNames)

	for _, proj := range projectNames {
		fmt.Printf("\n%s:\n", proj)
		for _, ep := range grouped[proj] {
			fmt.Printf("  %-15s %-6s %d → %s\n", ep.Service, ep.Method, ep.ContainerPort, ep.ConnString)
		}
	}
}

func cmdGet() {
	// Parse: zerobased get <service> [-t template] [-v key=val]...
	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zerobased get <service> [-t template] [-v key=val]...")
		os.Exit(1)
	}

	target := ""
	tmpl := ""
	userVars := map[string]string{}

	for i := 0; i < len(args); i++ {
		switch {
		case (args[i] == "-t" || args[i] == "--template") && i+1 < len(args):
			tmpl = args[i+1]
			i++
		case (args[i] == "-v" || args[i] == "--var") && i+1 < len(args):
			if k, v, ok := strings.Cut(args[i+1], "="); ok {
				userVars[k] = v
			}
			i++
		default:
			if target == "" {
				target = args[i]
			}
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: zerobased get <service> [-t template] [-v key=val]...")
		os.Exit(1)
	}

	containers, cleanup, err := listContainers()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer cleanup()

	endpoints := env.EndpointsFromContainers(daemon.DefaultBaseDir(), containers, "")
	found := false
	for _, ep := range endpoints {
		if ep.Service != target {
			continue
		}
		found = true
		if tmpl != "" {
			vars := env.TemplateVars(ep)
			for k, v := range userVars {
				vars[k] = v
			}
			fmt.Println(env.RenderTemplate(tmpl, vars))
		} else {
			fmt.Println(ep.ConnString)
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "service %q not found\n", target)
		os.Exit(1)
	}
}
