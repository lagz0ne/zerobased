package fabric

import "testing"

func TestPlanEndpointIntentsStableCandidateSequence(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
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

	first, err := PlanEndpointIntents(cfg, identity)
	if err != nil {
		t.Fatalf("PlanEndpointIntents() error = %v", err)
	}
	second, err := PlanEndpointIntents(cfg, identity)
	if err != nil {
		t.Fatalf("PlanEndpointIntents() second error = %v", err)
	}
	if first["api"].Candidates[0] != second["api"].Candidates[0] {
		t.Fatalf("candidate changed: %d != %d", first["api"].Candidates[0], second["api"].Candidates[0])
	}
	if first["api"].Candidates[0] < defaultPortRangeStart || first["api"].Candidates[0] > defaultPortRangeEnd {
		t.Fatalf("candidate %d outside default range", first["api"].Candidates[0])
	}
}

func TestPlanEndpointIntentsCustomRangeAndFixedPort(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
identity:
  port_range: 45000-45002
ports:
  api: http
  db:
    mode: tcp
    port: 5432
services:
  api:
    cmd: go run ./cmd/api
  db:
    cmd: ./start-db
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
	intents, err := PlanEndpointIntents(cfg, identity)
	if err != nil {
		t.Fatalf("PlanEndpointIntents() error = %v", err)
	}
	if got := len(intents["api"].Candidates); got != 3 {
		t.Fatalf("api candidate count = %d, want 3", got)
	}
	if got := intents["db"].Candidates; len(got) != 1 || got[0] != 5432 {
		t.Fatalf("db candidates = %#v, want fixed 5432", got)
	}
}

func TestPlanEndpointIntentsRejectsBadRange(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
identity:
  port_range: 5000-nope
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
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
	if _, err := PlanEndpointIntents(cfg, identity); err == nil {
		t.Fatal("PlanEndpointIntents() error = nil, want bad range")
	}
}
