package up

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/fabric"
)

const (
	configFilename = "zerobased.yaml"
	readyTimeout   = 30 * time.Second
	probeTimeout   = 2 * time.Second
)

type Options struct {
	Dir       string
	Profile   string
	Overrides map[string]string
	Shell     string
}

type Plan struct {
	Config       *fabric.Config
	Identity     fabric.IdentityFacts
	Hosts        *fabric.HostGraph
	Ports        map[string]PlannedPort
	PortFacts    map[string]fabric.PortFacts
	Rendered     *fabric.RenderedConfig
	ServiceOrder []string
}

type PlannedPort struct {
	Intent   fabric.EndpointIntent
	Port     int
	Occupied bool
}

func LoadConfig(startDir string) (*fabric.Config, string, error) {
	dir, err := findConfigDir(startDir)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, configFilename))
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", configFilename, err)
	}
	cfg, err := fabric.ParseConfig(data)
	if err != nil {
		return nil, "", err
	}
	if err := fabric.ValidateConfig(cfg); err != nil {
		return nil, "", err
	}
	return cfg, dir, nil
}

func BuildPlan(cfg *fabric.Config, input fabric.IdentityInput, profile string, overrides map[string]string) (*Plan, error) {
	identity, err := fabric.ResolveIdentity(cfg.Identity, input)
	if err != nil {
		return nil, err
	}
	hosts, err := fabric.BuildHostGraph(cfg, identity)
	if err != nil {
		return nil, err
	}
	intents, err := fabric.PlanEndpointIntents(cfg, identity)
	if err != nil {
		return nil, err
	}
	ports, facts, err := resolvePorts(cfg, hosts, intents)
	if err != nil {
		return nil, err
	}
	rendered, err := fabric.RenderConfig(cfg, fabric.RenderInput{
		Ports:     facts,
		Profile:   profile,
		Overrides: overrides,
		Run: map[string]string{
			"repo":  identity.RawRepo,
			"scope": identity.RawScope,
		},
	})
	if err != nil {
		return nil, err
	}
	order, err := orderServices(cfg, rendered.ActiveServices)
	if err != nil {
		return nil, err
	}
	plan := &Plan{
		Config:       cfg,
		Identity:     identity,
		Hosts:        hosts,
		Ports:        ports,
		PortFacts:    facts,
		Rendered:     rendered,
		ServiceOrder: order,
	}
	if err := validateRuntimePlan(plan); err != nil {
		return nil, err
	}
	if err := injectServiceMap(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func Run(opts Options) error {
	startDir := opts.Dir
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		startDir = cwd
	}

	cfg, cfgDir, err := LoadConfig(startDir)
	if err != nil {
		return err
	}
	identityInput, err := discoverIdentity(cfgDir)
	if err != nil {
		return err
	}
	plan, err := BuildPlan(cfg, identityInput, opts.Profile, opts.Overrides)
	if err != nil {
		return err
	}

	var (
		dc *docker.Client
		cm *caddy.Manager
	)
	if planHasHTTP(plan) {
		dc, err = docker.New()
		if err != nil {
			return fmt.Errorf("init docker for gateway: %w", err)
		}
		defer dc.Close()
		cm = caddy.NewFromWrapper(dc)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cm.Start(ctx); err != nil {
			return fmt.Errorf("start gateway: %w", err)
		}
		if err := cm.EnsureHTTPServer(); err != nil {
			return fmt.Errorf("init gateway config: %w", err)
		}
	}

	processes := map[string]*exec.Cmd{}
	var routeIDs []string
	cleanup := func() {
		for _, routeID := range routeIDs {
			if cm != nil {
				_ = cm.RemoveRoute(routeID)
			}
		}
		stopProcesses(processes)
	}
	defer cleanup()

	for _, name := range plan.ServiceOrder {
		started, err := startService(name, plan, opts.Shell)
		if err != nil {
			return err
		}
		if started != nil {
			processes[name] = started
		}
	}

	if cm != nil {
		routeIDs, err = registerRoutes(cm, plan)
		if err != nil {
			return err
		}
	}
	printHosts(plan)
	return waitForShutdown(processes)
}

func findConfigDir(start string) (string, error) {
	dir := start
	for {
		path := filepath.Join(dir, configFilename)
		if _, err := os.Stat(path); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", configFilename, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found", configFilename)
		}
		dir = parent
	}
}

func discoverIdentity(dir string) (fabric.IdentityInput, error) {
	repoRoot := firstNonEmpty(strings.TrimSpace(gitOutput(dir, "rev-parse", "--show-toplevel")), dir)
	repo := filepath.Base(repoRoot)
	branch := strings.TrimSpace(gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch == "HEAD" {
		branch = ""
	}
	return fabric.IdentityInput{
		Repo:     repo,
		Branch:   branch,
		Worktree: filepath.Base(dir),
		App:      repo,
		Domain:   "localhost",
	}, nil
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func resolvePorts(cfg *fabric.Config, hosts *fabric.HostGraph, intents map[string]fabric.EndpointIntent) (map[string]PlannedPort, map[string]fabric.PortFacts, error) {
	names := make([]string, 0, len(intents))
	for name := range intents {
		names = append(names, name)
	}
	sort.Strings(names)

	ports := map[string]PlannedPort{}
	facts := map[string]fabric.PortFacts{}
	for _, name := range names {
		intent := intents[name]
		if intent.Mode == fabric.PortSocket {
			return nil, nil, fmt.Errorf("ports.%s: socket runtime not implemented", name)
		}
		port, occupied, err := selectPort(intent)
		if err != nil {
			return nil, nil, fmt.Errorf("ports.%s: %w", name, err)
		}

		fact := fabric.PortFacts{
			"bind_host": intent.BindHost,
			"port":      strconv.Itoa(port),
		}
		host := intent.BindHost
		if intent.Mode == fabric.PortHTTP {
			if serviceHost, ok := hosts.Services[name]; ok {
				host = serviceHost
			}
			fact["host"] = host
			fact["url"] = fmt.Sprintf("http://%s", host)
		} else {
			fact["host"] = host
		}

		ports[name] = PlannedPort{
			Intent:   intent,
			Port:     port,
			Occupied: occupied,
		}
		facts[name] = fact
	}
	return ports, facts, nil
}

func selectPort(intent fabric.EndpointIntent) (int, bool, error) {
	if intent.FixedPort > 0 {
		return intent.FixedPort, portInUse(intent.BindHost, intent.FixedPort), nil
	}
	for _, candidate := range intent.Candidates {
		if !portInUse(intent.BindHost, candidate) {
			return candidate, false, nil
		}
	}
	return 0, false, fmt.Errorf("no free port candidates in %d-%d", intent.Range.Start, intent.Range.End)
}

func portInUse(host string, port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func orderServices(cfg *fabric.Config, active []string) ([]string, error) {
	activeSet := map[string]struct{}{}
	for _, name := range active {
		activeSet[name] = struct{}{}
	}

	visiting := map[string]bool{}
	visited := map[string]bool{}
	var ordered []string

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("services: dependency cycle at %s", name)
		}
		visiting[name] = true

		deps := make([]string, 0, len(cfg.Services[name].Needs))
		for dep := range cfg.Services[name].Needs {
			if _, ok := activeSet[dep]; ok {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		ordered = append(ordered, name)
		return nil
	}

	names := append([]string(nil), active...)
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func validateRuntimePlan(plan *Plan) error {
	active := map[string]struct{}{}
	for _, name := range plan.Rendered.ActiveServices {
		active[name] = struct{}{}
	}

	for _, name := range plan.Rendered.ActiveServices {
		service := plan.Config.Services[name]
		if service.Endpoint != fabric.EndpointDeliveryEnvPort {
			return fmt.Errorf("services.%s.endpoint: only env_port runtime is implemented", name)
		}
		if _, _, ok := serviceHTTPPort(plan, name); !ok {
			if usesHTTPReady(service) {
				return fmt.Errorf("services.%s: http readiness requires a matching http port", name)
			}
		}
	}

	for routeName, route := range plan.Config.Routes {
		for path, target := range route.Routes {
			if _, ok := active[target.To]; !ok {
				continue
			}
			if _, _, ok := serviceHTTPPort(plan, target.To); !ok {
				return fmt.Errorf("routes.%s.%s: service %q needs a matching http port", routeName, path, target.To)
			}
		}
	}
	return nil
}

func usesHTTPReady(service fabric.ServiceConfig) bool {
	_, ok := service.Ready["http"]
	return ok
}

func serviceHTTPPort(plan *Plan, service string) (PlannedPort, fabric.PortFacts, bool) {
	port, ok := plan.Ports[service]
	if !ok || port.Intent.Mode != fabric.PortHTTP {
		return PlannedPort{}, nil, false
	}
	facts := plan.PortFacts[service]
	return port, facts, true
}

func injectServiceMap(plan *Plan) error {
	payload := map[string]any{
		"services": map[string]map[string]string{},
		"routes":   map[string]map[string]string{},
	}
	services := payload["services"].(map[string]map[string]string)
	for _, name := range plan.ServiceOrder {
		facts := map[string]string{}
		for key, value := range plan.PortFacts[name] {
			facts[key] = value
		}
		if host := plan.Hosts.Services[name]; host != "" {
			facts["service_host"] = host
		}
		services[name] = facts
	}

	routes := payload["routes"].(map[string]map[string]string)
	routeNames := make([]string, 0, len(plan.Hosts.Routes))
	for name := range plan.Hosts.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, name := range routeNames {
		fact := plan.Hosts.Routes[name]
		routes[name] = map[string]string{
			"host":       fact.Host,
			"visibility": string(fact.Visibility),
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for _, env := range plan.Rendered.Env {
		env["ZB_SERVICE_MAP"] = string(data)
	}
	return nil
}

func planHasHTTP(plan *Plan) bool {
	for _, port := range plan.Ports {
		if port.Intent.Mode == fabric.PortHTTP {
			return true
		}
	}
	return false
}

func startService(name string, plan *Plan, shell string) (*exec.Cmd, error) {
	service := plan.Config.Services[name]
	if reused, err := shouldReuseService(name, service, plan); err != nil {
		return nil, err
	} else if reused {
		return nil, nil
	}

	if port, _, ok := servicePrimaryPort(plan, name); ok && port.Occupied {
		return nil, fmt.Errorf("services.%s: port %d already in use and source reuse did not apply", name, port.Port)
	}

	cmd := exec.Command(resolveShell(shell), "-lc", service.Cmd)
	cmd.Env = append(os.Environ(), envList(plan.Rendered.Env[name])...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	if err := waitReady(name, service, plan); err != nil {
		killProcessGroup(cmd)
		return nil, err
	}
	return cmd, nil
}

func shouldReuseService(name string, service fabric.ServiceConfig, plan *Plan) (bool, error) {
	if service.Source == nil {
		return false, nil
	}
	ok, err := sourceAvailable(service.Source, plan.PortFacts)
	if err != nil {
		return false, fmt.Errorf("services.%s.source: %w", name, err)
	}
	if ok {
		switch service.Source.Found {
		case fabric.SourceActionUse:
			return true, nil
		case fabric.SourceActionStart:
			return false, nil
		case fabric.SourceActionFail:
			return false, fmt.Errorf("services.%s.source: existing endpoint found", name)
		}
	}
	switch service.Source.Missing {
	case fabric.SourceActionStart:
		return false, nil
	case fabric.SourceActionFail:
		return false, fmt.Errorf("services.%s.source: required existing endpoint missing", name)
	case fabric.SourceActionUse:
		return false, fmt.Errorf("services.%s.source: cannot use missing endpoint", name)
	default:
		return false, nil
	}
}

func sourceAvailable(source *fabric.SourceConfig, facts map[string]fabric.PortFacts) (bool, error) {
	for kind, probe := range source.Probe {
		portFacts, ok := facts[probe.Port]
		if !ok {
			return false, fmt.Errorf("unknown port %q", probe.Port)
		}
		bindHost := portFacts["bind_host"]
		port := portFacts["port"]
		if bindHost == "" || port == "" {
			return false, fmt.Errorf("port %q has incomplete facts", probe.Port)
		}
		switch kind {
		case "tcp":
			if !checkTCP(bindHost, port) {
				return false, nil
			}
		case "http":
			if !checkHTTP(bindHost, port, probe.Path, probe.ExpectStatus, probe.ExpectHeader) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unsupported probe %q", kind)
		}
	}
	return true, nil
}

func waitReady(name string, service fabric.ServiceConfig, plan *Plan) error {
	if len(service.Ready) == 0 {
		return nil
	}
	deadline := time.Now().Add(readyTimeout)
	for {
		ready, err := readyChecksPass(name, service, plan)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("services.%s: readiness timed out", name)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func readyChecksPass(name string, service fabric.ServiceConfig, plan *Plan) (bool, error) {
	for kind, value := range service.Ready {
		switch kind {
		case "tcp":
			portName := value
			if portName == "" {
				portName = name
			}
			facts, ok := plan.PortFacts[portName]
			if !ok {
				return false, fmt.Errorf("services.%s.ready.tcp: unknown port %q", name, portName)
			}
			if !checkTCP(facts["bind_host"], facts["port"]) {
				return false, nil
			}
		case "http":
			_, facts, ok := servicePrimaryPort(plan, name)
			if !ok {
				return false, fmt.Errorf("services.%s.ready.http: requires matching http port", name)
			}
			path := value
			if path == "" {
				path = "/"
			}
			if !checkHTTP(facts["bind_host"], facts["port"], path, 0, nil) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("services.%s.ready.%s: unsupported readiness check", name, kind)
		}
	}
	return true, nil
}

func checkTCP(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func checkHTTP(host, port, path string, expectStatus int, expectHeader map[string]string) bool {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if expectStatus > 0 && resp.StatusCode != expectStatus {
		return false
	}
	for key, want := range expectHeader {
		if resp.Header.Get(key) != want {
			return false
		}
	}
	return expectStatus == 0 || resp.StatusCode == expectStatus || len(expectHeader) > 0
}

func registerRoutes(cm *caddy.Manager, plan *Plan) ([]string, error) {
	var routeIDs []string

	serviceNames := append([]string(nil), plan.ServiceOrder...)
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		port, facts, ok := servicePrimaryPort(plan, name)
		if !ok {
			continue
		}
		upstream := net.JoinHostPort(port.Intent.BindHost, strconv.Itoa(port.Port))
		routeID := "zb-up-svc-" + name
		if err := cm.AddHTTPRoute(routeID, plan.Hosts.Services[name], upstream); err != nil {
			return nil, fmt.Errorf("register service route %s: %w", name, err)
		}
		routeIDs = append(routeIDs, routeID)
		_ = facts
	}

	routeNames := make([]string, 0, len(plan.Config.Routes))
	for name := range plan.Config.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, routeName := range routeNames {
		hostFact := plan.Hosts.Routes[routeName]
		group := "zb-up-route-" + routeName
		paths := make([]string, 0, len(plan.Config.Routes[routeName].Routes))
		for path := range plan.Config.Routes[routeName].Routes {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			target := plan.Config.Routes[routeName].Routes[path]
			port, _, ok := servicePrimaryPort(plan, target.To)
			if !ok {
				return nil, fmt.Errorf("routes.%s.%s: service %q missing matching http port", routeName, path, target.To)
			}
			upstream := net.JoinHostPort(port.Intent.BindHost, strconv.Itoa(port.Port))
			routeID := fmt.Sprintf("zb-up-route-%s-%s", routeName, safeID(path))
			if err := cm.AddPathRoute(routeID, hostFact.Host, normalizeRoutePath(path), upstream, group); err != nil {
				return nil, fmt.Errorf("register path route %s %s: %w", routeName, path, err)
			}
			routeIDs = append(routeIDs, routeID)
		}
	}

	return routeIDs, nil
}

func servicePrimaryPort(plan *Plan, service string) (PlannedPort, fabric.PortFacts, bool) {
	port, ok := plan.Ports[service]
	if !ok {
		return PlannedPort{}, nil, false
	}
	facts := plan.PortFacts[service]
	return port, facts, true
}

func normalizeRoutePath(path string) string {
	if path == "/*" {
		return "/"
	}
	if strings.HasSuffix(path, "/*") {
		return strings.TrimSuffix(path, "/*")
	}
	return strings.TrimSuffix(path, "*")
}

func safeID(path string) string {
	replacer := strings.NewReplacer("/", "-", "*", "star", ".", "-", ":", "-")
	return replacer.Replace(strings.Trim(path, "-"))
}

func printHosts(plan *Plan) {
	serviceNames := append([]string(nil), plan.ServiceOrder...)
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		if _, _, ok := servicePrimaryPort(plan, name); !ok {
			continue
		}
		fmt.Println("http://" + plan.Hosts.Services[name])
	}

	routeNames := make([]string, 0, len(plan.Config.Routes))
	for name := range plan.Config.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, routeName := range routeNames {
		host := plan.Hosts.Routes[routeName].Host
		paths := make([]string, 0, len(plan.Config.Routes[routeName].Routes))
		for path := range plan.Config.Routes[routeName].Routes {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			fmt.Println("http://" + host + displayPath(path))
		}
	}
}

func displayPath(path string) string {
	if path == "/*" {
		return "/"
	}
	if strings.HasSuffix(path, "/*") {
		return strings.TrimSuffix(path, "/*")
	}
	return path
}

func waitForShutdown(processes map[string]*exec.Cmd) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	type result struct {
		name string
		err  error
	}
	done := make(chan result, len(processes))
	for name, cmd := range processes {
		go func(service string, c *exec.Cmd) {
			done <- result{name: service, err: c.Wait()}
		}(name, cmd)
	}

	if len(processes) == 0 {
		<-sigs
		return nil
	}

	select {
	case sig := <-sigs:
		_ = sig
		return nil
	case res := <-done:
		if res.err != nil {
			return fmt.Errorf("service %s exited: %w", res.name, res.err)
		}
		return fmt.Errorf("service %s exited", res.name)
	}
}

func stopProcesses(processes map[string]*exec.Cmd) {
	for _, cmd := range processes {
		killProcessGroup(cmd)
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func envList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func resolveShell(shell string) string {
	if strings.TrimSpace(shell) != "" {
		return shell
	}
	if envShell := os.Getenv("SHELL"); envShell != "" {
		return envShell
	}
	return "/bin/sh"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
