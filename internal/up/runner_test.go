package up

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/lagz0ne/zerobased/internal/composebackend"
	"github.com/lagz0ne/zerobased/internal/controlplane"
	"github.com/lagz0ne/zerobased/internal/zberr"
)

func TestRunClaimsBeforeProcessStartAndPublishesAfterReadiness(t *testing.T) {
	project := t.TempDir()
	startFile := filepath.Join(project, "started.txt")
	readyFile := filepath.Join(project, "ready.txt")
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{
		startFile: startFile,
		readyFile: readyFile,
		cancel:    cancel,
	}

	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
host: example.localhost
processes:
  web:
    command: ["sh", "-c", "printf started > %s; printf ready > %s; sleep 10"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, startFile, readyFile, readyFile))

	err := Run(ctx, Options{
		ProjectDir:   project,
		ControlPlane: plane,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if plane.claims != 1 {
		t.Fatalf("claim calls = %d, want 1", plane.claims)
	}
	if plane.publishes != 1 {
		t.Fatalf("publish calls = %d, want 1", plane.publishes)
	}
	if plane.publication.Host != "example.localhost" {
		t.Fatalf("published host = %q, want example.localhost", plane.publication.Host)
	}
	if plane.release.ClaimToken == "" {
		t.Fatalf("release was not called")
	}
}

func TestRunStaysForegroundUntilContextCancelled(t *testing.T) {
	project := t.TempDir()
	readyFile := filepath.Join(project, "ready.txt")
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{
		readyFile: readyFile,
		published: make(chan struct{}),
	}

	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
host: example.localhost
processes:
  web:
    command: ["sh", "-c", "printf ready > %s; sleep 10"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, readyFile, readyFile))

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ProjectDir:   project,
			ControlPlane: plane,
		})
	}()

	select {
	case <-plane.published:
	case err := <-done:
		t.Fatalf("Run exited before publishing: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for publish")
	}

	select {
	case err := <-done:
		t.Fatalf("Run exited before context cancellation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error after cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Run to stop")
	}
	if plane.release.ClaimToken == "" {
		t.Fatalf("release was not called")
	}
}

func TestRunFailsIfChildExitsAfterPublicationBeforeContextCancellation(t *testing.T) {
	project := t.TempDir()
	readyFile := filepath.Join(project, "ready.txt")
	plane := &recordingControlPlane{
		readyFile: readyFile,
		published: make(chan struct{}),
	}

	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
processes:
  web:
    command: ["sh", "-c", "printf ready > %s; sleep 0.1"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, readyFile, readyFile))

	err := Run(context.Background(), Options{
		ProjectDir:   project,
		ControlPlane: plane,
	})
	if err == nil {
		t.Fatalf("Run returned nil error after child exited")
	}
	if plane.publishes != 1 {
		t.Fatalf("publish calls = %d, want 1", plane.publishes)
	}
}

func TestRunStartsComposeAfterClaimBeforeProcessStart(t *testing.T) {
	project := t.TempDir()
	startFile := filepath.Join(project, "started.txt")
	readyFile := filepath.Join(project, "ready.txt")
	composeStartedFile := filepath.Join(project, "compose-started.txt")
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{
		startFile: startFile,
		cancel:    cancel,
	}
	backend := &recordingComposeBackend{
		startFile:           composeStartedFile,
		processStartFile:    startFile,
		requireClaimedPlane: plane,
	}

	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
host: example.localhost
profile: backend
compose:
  files:
    - compose.yaml
  services:
    - postgres
  ownership: owned
processes:
  web:
    command: ["sh", "-c", "test -f %s; printf started > %s; printf ready > %s; sleep 10"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, composeStartedFile, startFile, readyFile, readyFile))

	err := Run(ctx, Options{
		ProjectDir:       project,
		ControlPlane:     plane,
		ComposeBackend:   backend,
		CleanupTimeout:   time.Second,
		ReadinessTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if backend.starts != 1 {
		t.Fatalf("compose starts = %d, want 1", backend.starts)
	}
	if backend.request.ProjectDir != project {
		t.Fatalf("compose project dir = %q, want %q", backend.request.ProjectDir, project)
	}
	if backend.request.StackName != "example" {
		t.Fatalf("compose stack name = %q, want example", backend.request.StackName)
	}
	if backend.request.Profile != "backend" {
		t.Fatalf("compose profile = %q, want backend", backend.request.Profile)
	}
}

func TestRunStopsComposeBeforeReleaseOnReadinessFailure(t *testing.T) {
	project := t.TempDir()
	stopFile := filepath.Join(project, "compose-stopped.txt")
	plane := &recordingControlPlane{releaseRequiresFile: stopFile}
	backend := &recordingComposeBackend{stopFile: stopFile}

	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
compose:
  files:
    - compose.yaml
  services:
    - postgres
  ownership: owned
processes:
  web:
    command: ["sh", "-c", "sleep 10"]
    readiness:
      type: file
      path: never-ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	err := Run(context.Background(), Options{
		ProjectDir:       project,
		ControlPlane:     plane,
		ComposeBackend:   backend,
		ReadinessTimeout: 20 * time.Millisecond,
		CleanupTimeout:   time.Second,
	})
	if err == nil {
		t.Fatalf("Run returned nil error")
	}
	if backend.stops != 1 {
		t.Fatalf("compose stops = %d, want 1", backend.stops)
	}
	if plane.releases != 1 {
		t.Fatalf("release calls = %d, want 1", plane.releases)
	}
}

func TestRunDoesNotStartComposeWhenOnlyDefaultComposeFileExists(t *testing.T) {
	project := t.TempDir()
	readyFile := filepath.Join(project, "ready.txt")
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{cancel: cancel}
	backend := &recordingComposeBackend{}

	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
processes:
  web:
    command: ["sh", "-c", "printf ready > %s; sleep 10"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, readyFile, readyFile))

	err := Run(ctx, Options{
		ProjectDir:     project,
		ControlPlane:   plane,
		ComposeBackend: backend,
		CleanupTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if backend.starts != 0 {
		t.Fatalf("compose starts = %d, want 0", backend.starts)
	}
}

func TestRunMapsComposeModelInvalidToPreflightRejected(t *testing.T) {
	err := runWithComposeStartError(t, zberr.New(zberr.LayerComposeBackend, zberr.CodeModelInvalid))
	if !zberr.Is(err, zberr.LayerStackOrchestrator, zberr.CodePreflightRejected) {
		t.Fatalf("error = %v, want stack orchestrator PreflightRejected", err)
	}
}

func TestRunMapsComposeIsolationRejectedToPreflightRejected(t *testing.T) {
	err := runWithComposeStartError(t, zberr.New(zberr.LayerComposeBackend, zberr.CodeIsolationRejected))
	if !zberr.Is(err, zberr.LayerStackOrchestrator, zberr.CodePreflightRejected) {
		t.Fatalf("error = %v, want stack orchestrator PreflightRejected", err)
	}
}

func TestRunMapsComposeLifecycleFailedToDependencyStartFailed(t *testing.T) {
	err := runWithComposeStartError(t, zberr.New(zberr.LayerComposeBackend, zberr.CodeLifecycleFailed))
	if !zberr.Is(err, zberr.LayerStackOrchestrator, zberr.CodeDependencyStartFailed) {
		t.Fatalf("error = %v, want stack orchestrator DependencyStartFailed", err)
	}
}

func TestRunFindsProjectRootFromSubdirectoryAndNormalizesReadiness(t *testing.T) {
	project := t.TempDir()
	subdir := filepath.Join(project, "nested", "cmd")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{cancel: cancel}

	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
processes:
  web:
    command: ["sh", "-c", "printf ready > ready.txt; sleep 10"]
    readiness:
      type: file
      path: ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	err := Run(ctx, Options{
		ProjectDir:   subdir,
		ControlPlane: plane,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if plane.claim.ProjectDir != project {
		t.Fatalf("claim project dir = %q, want %q", plane.claim.ProjectDir, project)
	}
	if plane.claim.Host != filepath.Base(project)+".localhost" {
		t.Fatalf("default host = %q, want %q", plane.claim.Host, filepath.Base(project)+".localhost")
	}
}

func TestRunReleaseUsesBoundedCleanupTimeout(t *testing.T) {
	project := t.TempDir()
	readyFile := filepath.Join(project, "ready.txt")
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{
		readyFile:    readyFile,
		published:    make(chan struct{}),
		blockRelease: true,
	}

	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: example
processes:
  web:
    command: ["sh", "-c", "printf ready > %s; sleep 10"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: 3000
`, readyFile, readyFile))

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ProjectDir:     project,
			ControlPlane:   plane,
			CleanupTimeout: 20 * time.Millisecond,
		})
	}()

	select {
	case <-plane.published:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for publish")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run returned nil error after cleanup timeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not respect cleanup timeout")
	}
}

func TestTerminateProcessesStopsProcessGroups(t *testing.T) {
	project := t.TempDir()
	pidFile := filepath.Join(project, "pid.txt")

	cmd := exec.Command("sh", "-c", fmt.Sprintf("printf $$ > %s; sleep 10", pidFile))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	pid := waitForPID(t, pidFile)
	terminateProcesses([]runningProcess{{name: "web", cmd: cmd, done: done}})
	for range 100 {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d still alive after partial start failure", pid)
}

func TestRunCancelsDuringReadinessWithoutPublishing(t *testing.T) {
	project := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	plane := &recordingControlPlane{claimed: make(chan struct{})}
	claimed := plane.claimed

	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
processes:
  web:
    command: ["sh", "-c", "sleep 10"]
    readiness:
      type: file
      path: ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ProjectDir:       project,
			ControlPlane:     plane,
			ReadinessTimeout: 5 * time.Second,
		})
	}()

	select {
	case <-claimed:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for claim")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error after cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Run")
	}
	if plane.publishes != 0 {
		t.Fatalf("publish calls = %d, want 0", plane.publishes)
	}
}

func TestRunRejectsUnsupportedConfigVersion(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 2
name: example
`)

	err := Run(context.Background(), Options{
		ProjectDir:   project,
		ControlPlane: &recordingControlPlane{},
	})
	if err == nil {
		t.Fatalf("Run returned nil error")
	}
	if !IsConfigInvalid(err) {
		t.Fatalf("error = %v, want config invalid", err)
	}
}

func TestRunRejectsUnsupportedComposeReusedOwnership(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
compose:
  files:
    - compose.yaml
  ownership: reused
processes:
  web:
    command: ["sh", "-c", "sleep 10"]
    readiness:
      type: file
      path: ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	err := Run(context.Background(), Options{
		ProjectDir:   project,
		ControlPlane: &recordingControlPlane{},
	})
	if err == nil {
		t.Fatalf("Run returned nil error")
	}
	if !IsConfigInvalid(err) {
		t.Fatalf("error = %v, want config invalid", err)
	}
}

func TestRunRejectsUnsupportedProfileConfigurationShape(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
profiles:
  backend:
    compose:
      files:
        - compose.yaml
processes:
  web:
    command: ["sh", "-c", "sleep 10"]
    readiness:
      type: file
      path: ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	err := Run(context.Background(), Options{
		ProjectDir:   project,
		ControlPlane: &recordingControlPlane{},
	})
	if err == nil {
		t.Fatalf("Run returned nil error")
	}
	if !IsConfigInvalid(err) {
		t.Fatalf("error = %v, want config invalid", err)
	}
}

type recordingControlPlane struct {
	mu                  sync.Mutex
	startFile           string
	readyFile           string
	claims              int
	publishes           int
	releases            int
	claim               controlplane.Claim
	publication         controlplane.Publication
	release             controlplane.Release
	cancel              context.CancelFunc
	claimed             chan struct{}
	published           chan struct{}
	blockRelease        bool
	releaseRequiresFile string
}

func (plane *recordingControlPlane) Claim(ctx context.Context, claim controlplane.Claim) (controlplane.ClaimReceipt, error) {
	plane.mu.Lock()
	defer plane.mu.Unlock()

	if plane.startFile != "" {
		if _, err := os.Stat(plane.startFile); err == nil {
			return controlplane.ClaimReceipt{}, fmt.Errorf("claim after process start")
		}
	}
	plane.claims++
	plane.claim = claim
	if plane.claimed != nil {
		close(plane.claimed)
		plane.claimed = nil
	}
	return controlplane.ClaimReceipt{Token: "token", Generation: 1}, nil
}

func (plane *recordingControlPlane) Publish(ctx context.Context, publication controlplane.Publication) error {
	plane.mu.Lock()
	defer plane.mu.Unlock()

	if plane.readyFile != "" {
		if _, err := os.Stat(plane.readyFile); err != nil {
			return fmt.Errorf("publish before readiness: %w", err)
		}
	}
	plane.publishes++
	plane.publication = publication
	if plane.published != nil {
		close(plane.published)
	}
	if plane.cancel != nil {
		plane.cancel()
	}
	return nil
}

func (plane *recordingControlPlane) Release(ctx context.Context, release controlplane.Release) error {
	plane.mu.Lock()
	plane.releases++
	plane.release = release
	block := plane.blockRelease
	plane.mu.Unlock()

	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if plane.releaseRequiresFile != "" {
		if _, err := os.Stat(plane.releaseRequiresFile); err != nil {
			return fmt.Errorf("release before required file %s: %w", plane.releaseRequiresFile, err)
		}
	}
	return nil
}

type recordingComposeBackend struct {
	starts              int
	stops               int
	request             composebackend.Request
	startFile           string
	stopFile            string
	processStartFile    string
	requireClaimedPlane *recordingControlPlane
	err                 error
}

func (backend *recordingComposeBackend) Start(ctx context.Context, request composebackend.Request) (composebackend.RunningStack, error) {
	if backend.err != nil {
		return nil, backend.err
	}
	if backend.requireClaimedPlane != nil && backend.requireClaimedPlane.claims == 0 {
		return nil, fmt.Errorf("compose start before claim")
	}
	if backend.processStartFile != "" {
		if _, err := os.Stat(backend.processStartFile); err == nil {
			return nil, fmt.Errorf("compose start after process start")
		}
	}
	backend.starts++
	backend.request = request
	if backend.startFile != "" {
		if err := os.WriteFile(backend.startFile, []byte("started"), 0o644); err != nil {
			return nil, err
		}
	}
	return &recordingComposeStack{backend: backend}, nil
}

type recordingComposeStack struct {
	backend *recordingComposeBackend
}

func (stack *recordingComposeStack) Stop(ctx context.Context) error {
	stack.backend.stops++
	if stack.backend.stopFile != "" {
		return os.WriteFile(stack.backend.stopFile, []byte("stopped"), 0o644)
	}
	return nil
}

func runWithComposeStartError(t *testing.T, composeErr error) error {
	t.Helper()

	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), `version: 1
name: example
compose:
  files:
    - compose.yaml
  services:
    - postgres
  ownership: owned
processes:
  web:
    command: ["sh", "-c", "sleep 10"]
    readiness:
      type: file
      path: ready.txt
routes:
  - path: /
    process: web
    port: 3000
`)

	return Run(context.Background(), Options{
		ProjectDir:     project,
		ControlPlane:   &recordingControlPlane{},
		ComposeBackend: &recordingComposeBackend{err: composeErr},
		CleanupTimeout: time.Second,
	})
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()

	var content []byte
	for range 100 {
		var err error
		content, err = os.ReadFile(path)
		if err == nil && len(content) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var pid int
	if _, err := fmt.Sscanf(string(content), "%d", &pid); err != nil {
		t.Fatalf("read pid from %s: %v content=%q", path, err, content)
	}
	return pid
}

func waitUntil(t *testing.T, ready func() bool) {
	t.Helper()

	for range 100 {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition did not become ready")
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
