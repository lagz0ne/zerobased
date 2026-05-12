package up

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lagz0ne/zerobased/internal/fabric"
)

func TestBuildPlanRendersPortsHostsAndServiceOrder(t *testing.T) {
	cfg, err := fabric.ParseConfig([]byte(`
identity:
  repo: zerobased
  branch: feature/fabric
ports:
  db: tcp
  api: http
  web: http
profiles:
  local:
    db.url: "postgres://localhost:{{db.port}}/app"
services:
  db:
    cmd: ./start-db
    ready:
      tcp: db
  api:
    cmd: ./start-api
    needs: [db]
    env:
      HOST: "{{api.bind_host}}"
      PORT: "{{api.port}}"
      DATABASE_URL: "{{db.url}}"
      API_URL: "{{api.url}}"
    ready:
      http: /health
  web:
    cmd: ./start-web
    needs: [api]
    env:
      PORT: "{{web.port}}"
      API_URL: "{{api.url}}"
routes:
  app:
    /api/*: api
    /*: web
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := fabric.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	plan, err := BuildPlan(cfg, fabric.IdentityInput{
		Repo:     "zerobased",
		Branch:   "feature/fabric",
		Worktree: "/tmp/zerobased",
	}, "local", nil)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if got, want := strings.Join(plan.ServiceOrder, ","), "db,api,web"; got != want {
		t.Fatalf("service order = %q, want %q", got, want)
	}

	apiFacts := plan.PortFacts["api"]
	if apiFacts["bind_host"] != "127.0.0.1" {
		t.Fatalf("api bind_host = %q", apiFacts["bind_host"])
	}
	if want := "http://" + plan.Hosts.Services["api"]; apiFacts["url"] != want {
		t.Fatalf("api url = %q, want %q", apiFacts["url"], want)
	}

	wantDBURL := fmt.Sprintf("postgres://localhost:%s/app", plan.PortFacts["db"]["port"])
	if got := plan.Rendered.Env["api"]["DATABASE_URL"]; got != wantDBURL {
		t.Fatalf("DATABASE_URL = %q, want %q", got, wantDBURL)
	}
	if got := plan.Rendered.Env["web"]["API_URL"]; got != apiFacts["url"] {
		t.Fatalf("web API_URL = %q, want %q", got, apiFacts["url"])
	}

	if plan.Hosts.Routes["app"].Host == "" {
		t.Fatal("route host should be generated")
	}
}

func TestBuildPlanRejectsRoutedServiceWithoutMatchingHTTPPort(t *testing.T) {
	cfg, err := fabric.ParseConfig([]byte(`
ports:
  frontend: http
services:
  api:
    cmd: ./start-api
routes:
  app:
    /api/*: api
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := fabric.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	_, err = BuildPlan(cfg, fabric.IdentityInput{
		Repo:     "zerobased",
		Branch:   "main",
		Worktree: "/tmp/zerobased",
	}, "", nil)
	if err == nil {
		t.Fatal("BuildPlan() error = nil, want missing service port rejection")
	}
	if !strings.Contains(err.Error(), "matching http port") {
		t.Fatalf("unexpected error: %v", err)
	}
}
