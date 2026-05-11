package fabric

import "testing"

const configExample = `
ports:
  db:
    mode: tcp
    port: 5432
  backend: http
  frontend: http

profiles:
  local:
    db.url: "postgres://localhost:{{db.port}}/app"
  staging:
    disable: [db]
    required: [db.url]

services:
  db:
    cmd: docker compose up db
    env:
      DB_PORT: "{{db.port}}"
    source:
      tcp: db
    ready:
      tcp: db
  backend:
    cmd: go run ./cmd/api
    needs:
      db: required
    env:
      HOST: "{{backend.bind_host}}"
      PORT: "{{backend.port}}"
      DATABASE_URL: "{{db.url}}"
      AUTH_CALLBACK_URL: "{{backend.url}}/auth/callback"
    ready:
      http: /health
  frontend:
    cmd: pnpm dev
    needs: [backend]
    env:
      HOST: "{{frontend.bind_host}}"
      PORT: "{{frontend.port}}"
      API_URL: "{{backend.url}}"

routes:
  app:
    as: app
    /api/*: backend
    /ws/*: backend
    /*: frontend
  internal:
    internal: true
    /metrics: backend
`

func TestParseConfigCompactShape(t *testing.T) {
	cfg, err := ParseConfig([]byte(configExample))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	if cfg.Ports["backend"].Mode != PortHTTP {
		t.Fatalf("backend mode = %q, want %q", cfg.Ports["backend"].Mode, PortHTTP)
	}
	if cfg.Ports["db"].Port != "5432" {
		t.Fatalf("db port config = %#v, want fixed 5432", cfg.Ports["db"])
	}
	if cfg.Services["db"].Source == nil || cfg.Services["db"].Source.Probe["tcp"].Port != "db" {
		t.Fatalf("db source config = %#v, want tcp db probe", cfg.Services["db"].Source)
	}
	if cfg.Services["frontend"].Needs["backend"].Kind != NeedRequired {
		t.Fatalf("frontend needs backend = %q, want required", cfg.Services["frontend"].Needs["backend"].Kind)
	}
	if !cfg.Routes["internal"].Internal {
		t.Fatal("internal route should be internal")
	}
	if cfg.Routes["app"].Routes["/api/*"].To != "backend" {
		t.Fatalf("/api/* target = %q, want backend", cfg.Routes["app"].Routes["/api/*"].To)
	}
}

func TestValidateConfigRejectsUnknownRouteTarget(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
routes:
  app:
    /api/*: missing
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() error = nil, want unknown route target")
	}
}

func TestValidateConfigRejectsBadTemplateRef(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
    env:
      PORT: "{{missing.port}}"
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() error = nil, want bad template ref")
	}
}

func TestRenderConfigMergesProfileAndOverrides(t *testing.T) {
	cfg, err := ParseConfig([]byte(configExample))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	rendered, err := RenderConfig(cfg, RenderInput{
		Ports: map[string]PortFacts{
			"db": {
				"port": "55432",
			},
			"backend": {
				"bind_host": "127.0.0.1",
				"port":      "48001",
				"url":       "http://backend.localhost",
			},
			"frontend": {
				"bind_host": "127.0.0.1",
				"port":      "48002",
				"url":       "http://frontend.localhost",
			},
		},
		Profile: "local",
		Run: map[string]string{
			"name": "dev",
		},
	})
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	if got := rendered.Env["backend"]["DATABASE_URL"]; got != "postgres://localhost:55432/app" {
		t.Fatalf("DATABASE_URL = %q", got)
	}
	if got := rendered.Env["frontend"]["API_URL"]; got != "http://backend.localhost" {
		t.Fatalf("API_URL = %q", got)
	}
}

func TestRenderConfigRequiresProfileInputBeforeStart(t *testing.T) {
	cfg, err := ParseConfig([]byte(configExample))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	_, err = RenderConfig(cfg, RenderInput{
		Profile: "staging",
		Ports: map[string]PortFacts{
			"backend": {"url": "http://backend.localhost"},
		},
	})
	if err == nil {
		t.Fatal("RenderConfig() error = nil, want missing required db.url")
	}
}

func TestRenderConfigDisablesLocalServiceForExternalProfile(t *testing.T) {
	cfg, err := ParseConfig([]byte(configExample))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	rendered, err := RenderConfig(cfg, RenderInput{
		Profile: "staging",
		Overrides: map[string]string{
			"db.url": "postgres://staging/app",
		},
		Ports: map[string]PortFacts{
			"backend": {
				"bind_host": "127.0.0.1",
				"port":      "48001",
				"url":       "http://backend.localhost",
			},
			"frontend": {
				"bind_host": "127.0.0.1",
				"port":      "48002",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if _, ok := rendered.Env["db"]; ok {
		t.Fatal("db service should be disabled in staging profile")
	}
	if got := rendered.Env["backend"]["DATABASE_URL"]; got != "postgres://staging/app" {
		t.Fatalf("DATABASE_URL = %q", got)
	}
}

func TestValidateConfigRejectsSourceUnknownPort(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  db:
    mode: tcp
services:
  db:
    cmd: docker compose up db
    source:
      tcp: missing
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() error = nil, want unknown source port rejection")
	}
}

func TestValidateConfigAcceptsExplicitSourceActions(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  db: tcp
services:
  db:
    cmd: ./start-db
    source:
      probe:
        http:
          port: db
          path: /health
          expect_status: 200
          expect_header: X-ZB-Service=db
      found: use
      missing: start
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if cfg.Services["db"].Source.Found != SourceActionUse || cfg.Services["db"].Source.Missing != SourceActionStart {
		t.Fatalf("source actions = %#v", cfg.Services["db"].Source)
	}
}

func TestValidateConfigRejectsSourceUseWithoutIdentityProof(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  db: tcp
services:
  db:
    cmd: ./start-db
    source:
      tcp: db
      found: use
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() error = nil, want source use identity proof rejection")
	}
}

func TestValidateConfigAcceptsUnidentifiedSourceUseEscapeHatch(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  db: tcp
services:
  db:
    cmd: ./start-db
    source:
      tcp: db
      found: use
      allow_unidentified_existing: true
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigVisibilityConflict(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
routes:
  metrics:
    internal: true
    visibility: public
    /metrics: api
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() error = nil, want route visibility conflict")
	}
}

func TestValidateConfigNormalizesInternalVisibility(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
routes:
  metrics:
    internal: true
    /metrics: api
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if got := cfg.Routes["metrics"].Visibility; got != RouteVisibilityInternal {
		t.Fatalf("visibility = %q, want internal", got)
	}
}

func TestRenderConfigRejectsRouteToDisabledService(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
profiles:
  external:
    disable: [api]
    required: [api.url]
services:
  api:
    cmd: go run ./cmd/api
routes:
  app:
    /api/*: api
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	_, err = RenderConfig(cfg, RenderInput{
		Profile: "external",
		Overrides: map[string]string{
			"api.url": "https://api.staging.example.com",
		},
	})
	if err == nil {
		t.Fatal("RenderConfig() error = nil, want disabled route target rejection")
	}
}

func TestRenderConfigRejectsUnknownTemplateAtRender(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
ports:
  api: http
services:
  api:
    cmd: go run ./cmd/api
    env:
      PORT: "{{api.port}}"
`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	_, err = RenderConfig(cfg, RenderInput{
		Ports: map[string]PortFacts{
			"api": {},
		},
	})
	if err == nil {
		t.Fatal("RenderConfig() error = nil, want missing api.port fact")
	}
}
