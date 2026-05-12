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
	"github.com/lagz0ne/zerobased/internal/up"
)

// Global flags parsed before subcommand dispatch.
var (
	dockerHost string
	envPrefix  = "ZB" // default prefix for env vars; "" removes prefix
	profile    string // route profiles (comma-separated)
	version    = "dev"
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
		printUsage(nil)
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
	case "up":
		cmdUp()
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
	case "version":
		fmt.Println(version)
	case "help", "--help", "-h":
		printUsage(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "next: run `zerobased help` or `zerobased help <command>`")
		os.Exit(1)
	}
}

func printUsage(args []string) {
	if len(args) > 0 {
		printCommandHelp(args[0])
		return
	}
	fmt.Printf(`zerobased — zero-config Docker service router
Version: %s

Usage:
  zerobased [flags] <command> [args...]

Flags:
  -H, --docker-host <host>   Docker daemon socket (default: $DOCKER_HOST or unix:///var/run/docker.sock)
  --prefix <prefix>          Env var prefix (default: ZB → ZB_POSTGRES_5432; "" → POSTGRES_5432)
  --profile <names>          Route profiles from zerobased.routes.yaml (comma-separated, e.g. staging,debug)

Commands:
  help [command]                           Show guided help
  version                                  Print build version
  start [-d]                               Start daemon (-d for background)
  up [--profile name] [--set k=v]          Load zerobased.yaml and run configured services
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

Fast path:
  1. zerobased start -d
  2. docker compose up -d
  3. zerobased ps
  4. zerobased get <service>        # or: eval "$(zerobased env --export <project>)"

If stuck:
  zerobased logs -f                  # daemon diagnostics
  zerobased stop && zerobased start -d
  zerobased help <command>

Supported routefile (zerobased.routes.yaml):
  Use zerobased.routes.yaml for explicit routing, profiles, and external upstreams.
  It is the only supported routefile format. LLMs/automation should emit this
  file next to docker-compose.yml.

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
  If you omit --profile, define profiles.default.
  External URL targets keep scheme/host/port only; the route path comes from the YAML key.

Portless app config (zerobased.yaml):
  zerobased up loads zerobased.yaml, allocates named endpoints, injects env,
  and registers generated localhost hosts for configured services and routes.

Templates (zerobased get -t):
  zerobased get postgres -t 'postgresql://{{user}}:{{pass}}@/{{db}}?host={{socket_dir}}' \
    -v user=postgres -v pass=secret -v db=mydb

  Variables: project, service, container_port, method, conn, url,
             socket, socket_dir (socket types), host, port (http/port types)

Shell eval:
  eval "$(zerobased env --export acountee)"`, version)
}

func printCommandHelp(command string) {
	fmt.Printf("Version: %s\n\n", version)
	switch command {
	case "start":
		fmt.Println(`Usage: zerobased start [-d]

Start the Docker-watching daemon and local Caddy gateway.

Examples:
  zerobased start              # foreground
  zerobased start -d           # background, then follow logs

Next:
  docker compose up -d
  zerobased ps
  zerobased logs -f`)
	case "stop":
		fmt.Println(`Usage: zerobased stop

Stop the daemon, remove Caddy routes, and clean zerobased-managed sockets.

Next:
  zerobased start -d           # start clean again
  zerobased logs               # inspect previous run`)
	case "logs":
		fmt.Println(`Usage: zerobased logs [-f] [-n lines]

Show daemon logs.

Examples:
  zerobased logs
  zerobased logs -f
  zerobased logs -n 200`)
	case "run":
		fmt.Println(`Usage: zerobased run [-p port] [name] <command> [args...]

Wrap a host dev server, register a route, inject ZB_* env vars, and clean up on exit.

Routefiles:
  Use zerobased.routes.yaml next to docker-compose.yml. It is the only supported
  routefile format for explicit routes, profiles, and external upstreams.

Examples:
  zerobased run web pnpm dev
  zerobased run -p 3000 web pnpm dev

Next:
  zerobased ps
  zerobased get <service>`)
	case "up":
		fmt.Println(`Usage: zerobased up [--profile name] [--set k=v]...

Load zerobased.yaml, allocate named local endpoints, start configured services,
and register generated localhost routes.

Examples:
  zerobased up
  zerobased up --profile staging --set db.url=postgres://staging/app

Notes:
  zerobased.yaml is the app/runtime config; zerobased.routes.yaml remains the
  routefile format for Docker/daemon routing.
  This runtime currently supports env_port delivery and local HTTP/TCP endpoints.
  Routed HTTP services need a matching same-name ports.<service>: http entry.`)
	case "env":
		fmt.Println(`Usage: zerobased env [--export] [project]

Print all discovered connection strings, optionally as shell exports.

Examples:
  zerobased env
  zerobased env acountee
  eval "$(zerobased env --export acountee)"

Next:
  zerobased ps                  # find project/service names
  zerobased get <service>`)
	case "ps":
		fmt.Println(`Usage: zerobased ps

Show all discovered Docker Compose services and their connection strings.

If empty:
  zerobased start -d
  docker compose up -d
  zerobased logs -f`)
	case "get":
		fmt.Println(`Usage: zerobased get <service> [-t template] [-v key=val]...

Print one connection string. Use templates for app-specific URLs.

Examples:
  zerobased get postgres
  zerobased get postgres -t 'postgresql://{{user}}:{{pass}}@/{{db}}?host={{socket_dir}}' \
    -v user=postgres -v pass=secret -v db=mydb

Next:
  zerobased ps                  # find service names
  zerobased env --export <project>`)
	case "domain":
		fmt.Println(`Usage:
  zerobased domain add <domain> [--ttl 2h]
  zerobased domain add <domain> --persistent
  zerobased domain list
  zerobased domain rm @N

Add external domains for shareable preview URLs.

Next:
  zerobased share
  zerobased unshare @N`)
	case "share":
		fmt.Println(`Usage: zerobased share [@N]

Show shareable URLs for configured external domains.

If no domains:
  zerobased domain add <domain>

If no services:
  zerobased start -d
  docker compose up -d`)
	case "unshare":
		fmt.Println(`Usage: zerobased unshare @N | --all

Remove configured external domains and deregister their routes.

Next:
  zerobased domain list
  zerobased share`)
	case "version":
		fmt.Println(`Usage: zerobased version

Print the build version embedded by release builds.`)
	default:
		fmt.Fprintf(os.Stderr, "unknown help topic: %s\n", command)
		fmt.Fprintln(os.Stderr, "next: run `zerobased help`")
		os.Exit(1)
	}
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
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
		if isHelp(arg) {
			printCommandHelp("start")
			return
		}
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
	for _, arg := range os.Args[2:] {
		if isHelp(arg) {
			printCommandHelp("stop")
			return
		}
	}

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
		case isHelp(os.Args[i]):
			printCommandHelp("logs")
			return
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

func parseSetArg(arg string) (string, string, error) {
	key, value, ok := strings.Cut(arg, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", fmt.Errorf("--set expects key=value, got %q", arg)
	}
	return strings.TrimSpace(key), value, nil
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
	if len(args) > 0 && isHelp(args[0]) {
		printCommandHelp("run")
		return
	}
	if len(args) == 0 {
		printCommandHelp("run")
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

func cmdUp() {
	args := os.Args[2:]
	if len(args) > 0 && isHelp(args[0]) {
		printCommandHelp("up")
		return
	}

	currentProfile := profile
	overrides := map[string]string{}

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--profile" && i+1 < len(args):
			currentProfile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--profile="):
			currentProfile = strings.TrimPrefix(args[i], "--profile=")
		case args[i] == "--set" && i+1 < len(args):
			key, value, err := parseSetArg(args[i+1])
			if err != nil {
				log.Fatal(err)
			}
			overrides[key] = value
			i++
		case strings.HasPrefix(args[i], "--set="):
			key, value, err := parseSetArg(strings.TrimPrefix(args[i], "--set="))
			if err != nil {
				log.Fatal(err)
			}
			overrides[key] = value
		default:
			log.Fatalf("up: unknown arg %q", args[i])
		}
	}

	if strings.Contains(currentProfile, ",") {
		log.Fatal("up: only one --profile value is supported")
	}

	if err := up.Run(up.Options{
		Profile:   currentProfile,
		Overrides: overrides,
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
		case isHelp(arg):
			printCommandHelp("env")
			return
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
		fmt.Fprintln(os.Stderr, "next: run `zerobased ps`; if empty, run `zerobased start -d` then `docker compose up -d`")
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
		fmt.Println("next: run `zerobased start -d`, then `docker compose up -d`, then `zerobased ps`")
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
	if len(args) > 0 && isHelp(args[0]) {
		printCommandHelp("get")
		return
	}
	if len(args) == 0 {
		printCommandHelp("get")
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
		printCommandHelp("get")
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
		fmt.Fprintln(os.Stderr, "next: run `zerobased ps` to list service names")
		os.Exit(1)
	}
}
