package route_runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lagz0ne/zerobased/internal/controlplane"
)

func TestAdminURLPublishesRoutesWithPut(t *testing.T) {
	var method string
	var path string
	var publication controlplane.Publication

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&publication); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	adapter := AdminURL{URL: server.URL}
	err := adapter.Publish(context.Background(), controlplane.Publication{
		Name: "example",
		Host: "example.localhost",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if method != http.MethodPut {
		t.Fatalf("method = %q, want %q", method, http.MethodPut)
	}
	if path != "/routes" {
		t.Fatalf("path = %q, want /routes", path)
	}
	if publication.Host != "example.localhost" {
		t.Fatalf("host = %q, want example.localhost", publication.Host)
	}
}

func TestDockerCaddyBootstrapsOwnedContainerAndPublishesCaddyfile(t *testing.T) {
	home := t.TempDir()
	runner := &recordingCommandRunner{}
	runtime := NewDockerCaddy(DockerCaddyOptions{
		Home:   home,
		Runner: runner,
	})

	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if !runner.hasCommand("docker", "run", "-d", "--name", "zerobased-route-runtime-caddy") {
		t.Fatalf("bootstrap commands = %#v, want docker run for zerobased-route-runtime-caddy", runner.commands)
	}

	err := runtime.Publish(context.Background(), controlplane.Publication{
		Name: "acountee",
		Host: "acountee.localhost",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(home, "caddy", "Caddyfile"))
	if err != nil {
		t.Fatalf("read Caddyfile: %v", err)
	}
	caddyfile := string(content)
	if !strings.Contains(caddyfile, "acountee.localhost") {
		t.Fatalf("Caddyfile = %q, want host", caddyfile)
	}
	if !strings.Contains(caddyfile, "reverse_proxy host.docker.internal:3000") {
		t.Fatalf("Caddyfile = %q, want host-gateway reverse proxy", caddyfile)
	}
	if !runner.hasCommand("docker", "exec", "zerobased-route-runtime-caddy", "caddy", "reload") {
		t.Fatalf("publish commands = %#v, want caddy reload", runner.commands)
	}

	if err := runtime.Unpublish(context.Background(), controlplane.Publication{Host: "acountee.localhost"}); err != nil {
		t.Fatalf("Unpublish returned error: %v", err)
	}
	content, err = os.ReadFile(filepath.Join(home, "caddy", "Caddyfile"))
	if err != nil {
		t.Fatalf("read Caddyfile after unpublish: %v", err)
	}
	if strings.Contains(string(content), "acountee.localhost") {
		t.Fatalf("Caddyfile after unpublish = %q, want host removed", content)
	}
}

func TestDockerCaddyRejectsExistingContainerFromDifferentHome(t *testing.T) {
	home := t.TempDir()
	runner := &recordingCommandRunner{
		inspectLabels: map[string]string{
			"dev.zerobased.runtime": "caddy",
			"dev.zerobased.owner":   "zerobased",
			"dev.zerobased.home":    "/tmp/other-zerobased",
		},
	}
	runtime := NewDockerCaddy(DockerCaddyOptions{
		Home:   home,
		Runner: runner,
	})

	err := runtime.Bootstrap(context.Background())
	if err == nil {
		t.Fatalf("Bootstrap returned nil error, want stale container rejection")
	}
	if !strings.Contains(err.Error(), "different ZEROBASED_HOME") {
		t.Fatalf("Bootstrap error = %v, want different ZEROBASED_HOME", err)
	}
	if runner.hasCommand("docker", "start", "zerobased-route-runtime-caddy") {
		t.Fatalf("bootstrap commands = %#v, started stale container", runner.commands)
	}
	if runner.hasCommand("docker", "run") {
		t.Fatalf("bootstrap commands = %#v, replaced stale container instead of failing fast", runner.commands)
	}
}

func TestDockerCaddyDoesNotCommitRouteStateWhenReloadFails(t *testing.T) {
	home := t.TempDir()
	runner := &recordingCommandRunner{failReload: true}
	runtime := NewDockerCaddy(DockerCaddyOptions{
		Home:   home,
		Runner: runner,
	})
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}

	err := runtime.Publish(context.Background(), controlplane.Publication{
		Name: "acountee",
		Host: "acountee.localhost",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	})
	if err == nil {
		t.Fatalf("Publish returned nil error, want reload failure")
	}

	runner.failReload = false
	err = runtime.Publish(context.Background(), controlplane.Publication{
		Name: "other",
		Host: "other.localhost",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    4000,
		}},
	})
	if err != nil {
		t.Fatalf("second Publish returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(home, "caddy", "Caddyfile"))
	if err != nil {
		t.Fatalf("read Caddyfile: %v", err)
	}
	caddyfile := string(content)
	if strings.Contains(caddyfile, "acountee.localhost") {
		t.Fatalf("Caddyfile = %q, failed publish leaked into later reload", caddyfile)
	}
	if !strings.Contains(caddyfile, "other.localhost") {
		t.Fatalf("Caddyfile = %q, want successful later route", caddyfile)
	}
}

type recordingCommandRunner struct {
	commands      [][]string
	failReload    bool
	inspectLabels map[string]string
}

func (runner *recordingCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := append([]string{name}, args...)
	runner.commands = append(runner.commands, command)
	if name == "docker" && len(args) >= 1 && args[0] == "inspect" {
		if runner.inspectLabels != nil {
			payload, err := json.Marshal(runner.inspectLabels)
			if err != nil {
				return nil, err
			}
			return payload, nil
		}
		return nil, errors.New("not found")
	}
	if name == "docker" && len(args) >= 2 && args[0] == "exec" && runner.failReload {
		return nil, errors.New("reload failed")
	}
	return []byte("ok"), nil
}

func (runner *recordingCommandRunner) hasCommand(parts ...string) bool {
	for _, command := range runner.commands {
		if len(command) < len(parts) {
			continue
		}
		matches := true
		for i, part := range parts {
			if command[i] != part {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
