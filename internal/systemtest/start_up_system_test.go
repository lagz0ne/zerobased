//go:build system

package systemtest

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartCreatesDaemonLockSocketAndHealth(t *testing.T) {
	bin := buildZerobased(t)
	home := t.TempDir()
	routeRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer routeRuntime.Close()

	daemon := startDaemon(t, home, bin, env{
		"ZEROBASED_ROUTE_RUNTIME_BACKEND":   "admin-url",
		"ZEROBASED_ROUTE_RUNTIME_ADMIN_URL": routeRuntime.URL,
	})
	defer daemon.stop(t)

	assertExists(t, filepath.Join(home, "daemon.lock"))
	assertExists(t, filepath.Join(home, "daemon.sock"))
	assertExists(t, filepath.Join(home, "health.json"))
}

func TestUpPublishesOnlyAfterProcessReadiness(t *testing.T) {
	bin := buildZerobased(t)
	home := t.TempDir()
	project := t.TempDir()
	readyFile := filepath.Join(project, "ready.txt")
	port := freeTCPPort(t)
	var routeApplyCount atomic.Int32

	routeRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/routes" {
			routeApplyCount.Add(1)
			if _, err := os.Stat(readyFile); err != nil {
				t.Fatalf("route runtime was called before process readiness: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == "/routes" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer routeRuntime.Close()

	daemon := startDaemon(t, home, bin, env{
		"ZEROBASED_ROUTE_RUNTIME_BACKEND":   "admin-url",
		"ZEROBASED_ROUTE_RUNTIME_ADMIN_URL": routeRuntime.URL,
	})
	defer daemon.stop(t)

	writeFile(t, filepath.Join(project, "server.go"), `package main

import (
	"net"
	"net/http"
	"os"
)

func main() {
	readyPath := os.Args[1]
	port := os.Args[2]
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(readyPath, []byte("ready"), 0o644); err != nil {
		panic(err)
	}
	if err := http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})); err != nil {
		panic(err)
	}
}
`)
	writeFile(t, filepath.Join(project, "zerobased.yaml"), fmt.Sprintf(`version: 1
name: systemtest
host: systemtest.localhost
processes:
  web:
    command: ["go", "run", "server.go", "%s", "%d"]
    readiness:
      type: file
      path: %s
routes:
  - path: /
    process: web
    port: %d
`, readyFile, port, readyFile, port))

	up := startProcess(t, home, project, bin, "up", nil)
	waitForCondition(t, "route runtime publication", up, func() bool {
		return routeApplyCount.Load() > 0
	})
	assertStillRunning(t, up)
	up.interrupt(t)

	if routeApplyCount.Load() == 0 {
		t.Fatalf("route runtime was not called")
	}
}

type env map[string]string

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type daemonProcess struct {
	cmd    *exec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	done   chan error
}

func buildZerobased(t *testing.T) string {
	t.Helper()

	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "zerobased")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/zerobased")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build zerobased: %v\n%s", err, output)
	}
	return bin
}

func startDaemon(t *testing.T, home string, bin string, extra env) daemonProcess {
	t.Helper()

	process := startProcess(t, home, "", bin, "start", extra)
	waitForExists(t, filepath.Join(home, "daemon.lock"), process)
	waitForExists(t, filepath.Join(home, "daemon.sock"), process)
	waitForExists(t, filepath.Join(home, "health.json"), process)
	return process
}

func startProcess(t *testing.T, home string, dir string, bin string, arg string, extra env) daemonProcess {
	t.Helper()

	cmd := exec.Command(bin, "start")
	cmd.Args = []string{bin, arg}
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "ZEROBASED_HOME="+home)
	for key, value := range extra {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s command: %v", arg, err)
	}

	process := daemonProcess{cmd: cmd, stdout: &stdout, stderr: &stderr, done: make(chan error, 1)}
	go func() {
		process.done <- cmd.Wait()
	}()
	return process
}

func (process daemonProcess) stop(t *testing.T) {
	t.Helper()

	if process.cmd.Process == nil {
		return
	}
	select {
	case <-process.done:
		return
	default:
	}
	if err := process.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill process: %v\nstdout:\n%s\nstderr:\n%s", err, process.stdout.String(), process.stderr.String())
	}
	<-process.done
}

func (process daemonProcess) interrupt(t *testing.T) {
	t.Helper()

	if err := process.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt process: %v\nstdout:\n%s\nstderr:\n%s", err, process.stdout.String(), process.stderr.String())
	}
	select {
	case err := <-process.done:
		if err != nil {
			t.Fatalf("process exited after interrupt with error: %v\nstdout:\n%s\nstderr:\n%s", err, process.stdout.String(), process.stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = process.cmd.Process.Kill()
		t.Fatalf("timed out waiting for interrupted process\nstdout:\n%s\nstderr:\n%s", process.stdout.String(), process.stderr.String())
	}
}

func assertStillRunning(t *testing.T, process daemonProcess) {
	t.Helper()

	select {
	case err := <-process.done:
		t.Fatalf("process exited before signal: %v\nstdout:\n%s\nstderr:\n%s", err, process.stdout.String(), process.stderr.String())
	case <-time.After(100 * time.Millisecond):
	}
}

func run(t *testing.T, home string, dir string, bin string, arg string, extra env) commandResult {
	t.Helper()

	cmd := exec.Command(bin, arg)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "ZEROBASED_HOME="+home)
	for key, value := range extra {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return commandResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func waitForExists(t *testing.T, path string, process daemonProcess) {
	t.Helper()

	for range 100 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s\nstdout:\n%s\nstderr:\n%s", path, process.stdout.String(), process.stderr.String())
}

func waitForCondition(t *testing.T, name string, process daemonProcess, ready func() bool) {
	t.Helper()

	for range 300 {
		if ready() {
			return
		}
		select {
		case err := <-process.done:
			t.Fatalf("process exited before %s: %v\nstdout:\n%s\nstderr:\n%s", name, err, process.stdout.String(), process.stderr.String())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s\nstdout:\n%s\nstderr:\n%s", name, process.stdout.String(), process.stderr.String())
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from cwd")
		}
		dir = parent
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate tcp port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}
