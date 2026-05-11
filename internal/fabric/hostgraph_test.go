package fabric

import (
	"strings"
	"testing"
)

func TestBuildHostGraphGeneratesBranchScopedHosts(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
  web: http
services:
  api:
    cmd: go run ./cmd/api
  web:
    cmd: pnpm dev
routes:
  app:
    /api/*: api
    /*: web
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	identity, err := ResolveIdentity(cfg.Identity, IdentityInput{
		Repo:     "zerobased",
		Branch:   "feature/auth",
		Worktree: "/tmp/agent-a",
	})
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	graph, err := BuildHostGraph(cfg, identity)
	if err != nil {
		t.Fatalf("BuildHostGraph() error = %v", err)
	}

	for _, host := range []string{graph.Services["api"], graph.Services["web"], graph.Routes["app"].Host} {
		if !strings.HasSuffix(host, "."+identity.Scope.Value+"."+identity.Repo.Value+".localhost") {
			t.Fatalf("host %q does not include scope/repo/domain", host)
		}
	}
}

func TestBuildHostGraphRejectsHostConflict(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
routes:
  app:
    host: same.localhost
    /api/*: api
  admin:
    host: same.localhost
    /admin/*: api
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	identity, err := ResolveIdentity(cfg.Identity, IdentityInput{Repo: "repo", Branch: "main"})
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if _, err := BuildHostGraph(cfg, identity); err == nil {
		t.Fatal("BuildHostGraph() error = nil, want host conflict")
	}
}

func TestResolveIdentityConfigOverridesGitFacts(t *testing.T) {
	identity, err := ResolveIdentity(IdentityConfig{
		Repo:   "configured-repo",
		Branch: "configured-branch",
		Domain: "dev.localhost",
	}, IdentityInput{
		Repo:   "git-repo",
		Branch: "git-branch",
	})
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.Repo.Raw != "configured-repo" {
		t.Fatalf("repo raw = %q, want config override", identity.Repo.Raw)
	}
	if identity.Scope.Raw != "configured-branch" {
		t.Fatalf("scope raw = %q, want config override", identity.Scope.Raw)
	}
	if identity.Domain != "dev.localhost" {
		t.Fatalf("domain = %q, want config override", identity.Domain)
	}
}
