package up

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lagz0ne/zerobased/internal/composebackend"
	"github.com/lagz0ne/zerobased/internal/controlplane"
	"github.com/lagz0ne/zerobased/internal/zberr"
)

type ControlPlane interface {
	Claim(context.Context, controlplane.Claim) (controlplane.ClaimReceipt, error)
	Publish(context.Context, controlplane.Publication) error
	Release(context.Context, controlplane.Release) error
}

type ComposeBackend interface {
	Start(context.Context, composebackend.Request) (composebackend.RunningStack, error)
}

type Options struct {
	ProjectDir       string
	ControlPlane     ControlPlane
	ComposeBackend   ComposeBackend
	ReadinessTimeout time.Duration
	CleanupTimeout   time.Duration
}

type configFile struct {
	Version   int                      `yaml:"version"`
	Name      string                   `yaml:"name"`
	Host      string                   `yaml:"host"`
	Profile   string                   `yaml:"profile"`
	Compose   composeConfig            `yaml:"compose"`
	Processes map[string]processConfig `yaml:"processes"`
	Routes    []routeConfig            `yaml:"routes"`
}

type composeConfig struct {
	Files     []string `yaml:"files"`
	Profiles  []string `yaml:"profiles"`
	Services  []string `yaml:"services"`
	Ownership string   `yaml:"ownership"`
}

type processConfig struct {
	Command   []string        `yaml:"command"`
	Readiness readinessConfig `yaml:"readiness"`
}

type readinessConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type routeConfig struct {
	Path    string `yaml:"path"`
	Process string `yaml:"process"`
	Port    int    `yaml:"port"`
}

type runningProcess struct {
	name string
	cmd  *exec.Cmd
	done chan error
}

const (
	defaultCleanupTimeout   = 30 * time.Second
	defaultReadinessTimeout = 60 * time.Second
)

func Run(ctx context.Context, options Options) (err error) {
	if options.ControlPlane == nil {
		return configInvalid("control plane client is required")
	}

	projectDir := options.ProjectDir
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(err))
		}
		projectDir = cwd
	}

	projectDir, err = findProjectRoot(projectDir)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(projectDir)
	if err != nil {
		return err
	}

	routes := routeClaims(cfg.Routes)
	sessionID := newSessionID()
	claim := controlplane.Claim{
		Name:       cfg.Name,
		Host:       cfg.Host,
		ProjectDir: projectDir,
		SessionID:  sessionID,
		Routes:     routes,
	}
	receipt, err := options.ControlPlane.Claim(ctx, claim)
	if err != nil {
		return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeClaimRejected, zberr.WithCause(err))
	}
	release := controlplane.Release{
		Name:       cfg.Name,
		Host:       cfg.Host,
		ProjectDir: projectDir,
		SessionID:  sessionID,
		ClaimToken: receipt.Token,
		Generation: receipt.Generation,
	}
	defer func() {
		cleanupTimeout := cleanupTimeoutOrDefault(options.CleanupTimeout)
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancelRelease()

		if releaseErr := options.ControlPlane.Release(releaseCtx, release); releaseErr != nil {
			if err != nil {
				releaseErr = fmt.Errorf("%w; primary error: %v", releaseErr, err)
			}
			err = zberr.New(zberr.LayerStackOrchestrator, zberr.CodeCleanupFailed, zberr.WithCause(releaseErr))
		}
	}()

	var composeStack composebackend.RunningStack
	if cfg.Compose.enabled() {
		backend := options.ComposeBackend
		if backend == nil {
			backend = composebackend.Backend{}
		}
		composeStack, err = backend.Start(ctx, composeRequest(projectDir, cfg, sessionID, cleanupTimeoutOrDefault(options.CleanupTimeout)))
		if err != nil {
			return mapComposeStartError(err)
		}
		defer func() {
			cleanupTimeout := cleanupTimeoutOrDefault(options.CleanupTimeout)
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancelCleanup()
			if cleanupErr := composeStack.Stop(cleanupCtx); cleanupErr != nil {
				if err != nil {
					cleanupErr = fmt.Errorf("%w; primary error: %v", cleanupErr, err)
				}
				err = zberr.New(zberr.LayerStackOrchestrator, zberr.CodeCleanupFailed, zberr.WithCause(cleanupErr))
			}
		}()
	}

	processCtx, stopProcesses := context.WithCancel(ctx)

	processes, err := startProcesses(processCtx, projectDir, cfg.Processes)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer cleanupProcesses(stopProcesses, processes)

	timeout := options.ReadinessTimeout
	timeout = readinessTimeoutOrDefault(timeout)
	if err := waitForReadiness(ctx, cfg.Processes, processes, timeout); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if ctx.Err() != nil {
		return nil
	}

	publication := controlplane.Publication{
		Name:       cfg.Name,
		Host:       cfg.Host,
		ProjectDir: projectDir,
		SessionID:  sessionID,
		Routes:     routes,
		ClaimToken: receipt.Token,
		Generation: receipt.Generation,
	}
	if err := options.ControlPlane.Publish(ctx, publication); err != nil {
		return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeGenerationCommitFailed, zberr.WithCause(err))
	}

	return waitForeground(ctx, processes)
}

func findProjectRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(err))
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "zerobased.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(fmt.Errorf("zerobased.yaml not found from %s", start)))
		}
		dir = parent
	}
}

func loadConfig(projectDir string) (configFile, error) {
	path := filepath.Join(projectDir, "zerobased.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		return configFile{}, zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(err))
	}
	if !firstConfigKeyIsVersion(string(content)) {
		return configFile{}, configInvalid("zerobased.yaml must start with version")
	}

	var cfg configFile
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return configFile{}, zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(err))
	}
	if cfg.Version != 1 {
		return configFile{}, configInvalid(fmt.Sprintf("unsupported zerobased.yaml version %d", cfg.Version))
	}
	if cfg.Name == "" {
		return configFile{}, configInvalid("name is required")
	}
	if cfg.Host == "" {
		cfg.Host = filepath.Base(projectDir) + ".localhost"
	}
	if cfg.Profile == "" {
		cfg.Profile = "dev"
	}
	if cfg.Compose.enabled() {
		if len(cfg.Compose.Files) == 0 {
			return configFile{}, configInvalid("compose.files is required")
		}
		if cfg.Compose.Ownership == "" {
			cfg.Compose.Ownership = string(composebackend.OwnershipOwned)
		}
		switch composebackend.Ownership(cfg.Compose.Ownership) {
		case composebackend.OwnershipOwned:
		default:
			return configFile{}, configInvalid(fmt.Sprintf("unsupported compose ownership %s", cfg.Compose.Ownership))
		}
	}
	if len(cfg.Processes) == 0 {
		return configFile{}, configInvalid("at least one process is required")
	}
	if len(cfg.Routes) == 0 {
		return configFile{}, configInvalid("at least one route is required")
	}
	for name, process := range cfg.Processes {
		if len(process.Command) == 0 {
			return configFile{}, configInvalid(fmt.Sprintf("process %s command is required", name))
		}
		if process.Readiness.Type != "file" {
			return configFile{}, configInvalid(fmt.Sprintf("process %s readiness type must be file", name))
		}
		if process.Readiness.Path == "" {
			return configFile{}, configInvalid(fmt.Sprintf("process %s readiness path is required", name))
		}
		if !filepath.IsAbs(process.Readiness.Path) {
			process.Readiness.Path = filepath.Join(projectDir, process.Readiness.Path)
			cfg.Processes[name] = process
		}
	}
	for _, route := range cfg.Routes {
		if route.Path == "" || route.Process == "" || route.Port <= 0 {
			return configFile{}, configInvalid("route path, process, and positive port are required")
		}
		if _, ok := cfg.Processes[route.Process]; !ok {
			return configFile{}, configInvalid(fmt.Sprintf("route references unknown process %s", route.Process))
		}
	}
	return cfg, nil
}

func firstConfigKeyIsVersion(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return strings.HasPrefix(trimmed, "version:")
	}
	return false
}

func startProcesses(_ context.Context, projectDir string, processes map[string]processConfig) ([]runningProcess, error) {
	names := make([]string, 0, len(processes))
	for name := range processes {
		names = append(names, name)
	}
	sort.Strings(names)

	running := make([]runningProcess, 0, len(names))
	for _, name := range names {
		process := processes[name]
		cmd := exec.Command(process.Command[0], process.Command[1:]...)
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			terminateProcesses(running)
			return nil, zberr.New(zberr.LayerStackOrchestrator, zberr.CodeDependencyStartFailed, zberr.WithCause(err))
		}

		item := runningProcess{name: name, cmd: cmd, done: make(chan error, 1)}
		done := item.done
		command := cmd
		go func() {
			done <- command.Wait()
		}()
		running = append(running, item)
	}
	return running, nil
}

func cleanupProcesses(cancel context.CancelFunc, processes []runningProcess) {
	terminateProcesses(processes)
	cancel()
}

func terminateProcesses(processes []runningProcess) {
	for _, process := range processes {
		if process.cmd.Process != nil {
			_ = syscall.Kill(-process.cmd.Process.Pid, syscall.SIGTERM)
		}
	}
	for _, process := range processes {
		select {
		case <-process.done:
			continue
		case <-time.After(200 * time.Millisecond):
			if process.cmd.Process != nil {
				_ = syscall.Kill(-process.cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}
}

func waitForReadiness(ctx context.Context, processes map[string]processConfig, running []runningProcess, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if allReady(processes) {
			return nil
		}
		for _, process := range running {
			select {
			case err := <-process.done:
				if err == nil {
					err = fmt.Errorf("process %s exited before readiness", process.name)
				}
				return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeReadinessFailed, zberr.WithCause(err))
			default:
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeReadinessFailed, zberr.WithCause(fmt.Errorf("readiness timeout")))
		case <-ticker.C:
		}
	}
}

func allReady(processes map[string]processConfig) bool {
	for _, process := range processes {
		if _, err := os.Stat(process.Readiness.Path); err != nil {
			return false
		}
	}
	return true
}

func waitForeground(ctx context.Context, processes []runningProcess) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		for _, process := range processes {
			select {
			case err := <-process.done:
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					return nil
				}
				if err == nil {
					err = fmt.Errorf("process %s exited while zerobased up was foreground", process.name)
				}
				return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeDependencyStartFailed, zberr.WithCause(err))
			default:
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func routeClaims(routes []routeConfig) []controlplane.Route {
	result := make([]controlplane.Route, 0, len(routes))
	for _, route := range routes {
		result = append(result, controlplane.Route{
			Path:    route.Path,
			Process: route.Process,
			Port:    route.Port,
		})
	}
	return result
}

func (config composeConfig) enabled() bool {
	return len(config.Files) > 0 || len(config.Profiles) > 0 || len(config.Services) > 0 || config.Ownership != ""
}

func cleanupTimeoutOrDefault(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return defaultCleanupTimeout
	}
	return timeout
}

func readinessTimeoutOrDefault(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return defaultReadinessTimeout
	}
	return timeout
}

func composeRequest(projectDir string, cfg configFile, sessionID string, rollbackTimeout time.Duration) composebackend.Request {
	return composebackend.Request{
		ProjectDir:      projectDir,
		StackName:       cfg.Name,
		Profile:         cfg.Profile,
		SessionID:       sessionID,
		Files:           append([]string(nil), cfg.Compose.Files...),
		ComposeProfiles: append([]string(nil), cfg.Compose.Profiles...),
		Services:        append([]string(nil), cfg.Compose.Services...),
		Ownership:       composebackend.Ownership(cfg.Compose.Ownership),
		RollbackTimeout: rollbackTimeout,
	}
}

func mapComposeStartError(err error) error {
	switch {
	case zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeModelInvalid),
		zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeIsolationRejected):
		return zberr.New(zberr.LayerStackOrchestrator, zberr.CodePreflightRejected, zberr.WithCause(err))
	case zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeLifecycleFailed):
		return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeDependencyStartFailed, zberr.WithCause(err))
	case zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeCritical):
		return zberr.Critical(zberr.LayerStackOrchestrator, err)
	default:
		return zberr.Critical(zberr.LayerStackOrchestrator, err)
	}
}

func configInvalid(message string) error {
	return zberr.New(zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid, zberr.WithCause(fmt.Errorf("%s", message)))
}

func IsConfigInvalid(err error) bool {
	return zberr.Is(err, zberr.LayerStackOrchestrator, zberr.CodeConfigInvalid)
}

func newSessionID() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(token[:])
}
