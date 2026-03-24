package routes

import (
	"testing"
)

// --- ParseTarget tests ---

func TestParseTarget_BareService(t *testing.T) {
	tgt, err := ParseTarget("frontend")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Service != "frontend" {
		t.Errorf("Service = %q, want %q", tgt.Service, "frontend")
	}
	if tgt.External {
		t.Error("bare service should not be external")
	}
	if tgt.Scheme != "" {
		t.Errorf("Scheme = %q, want empty", tgt.Scheme)
	}
	if tgt.Raw != "frontend" {
		t.Errorf("Raw = %q, want %q", tgt.Raw, "frontend")
	}
}

func TestParseTarget_HTTPS(t *testing.T) {
	tgt, err := ParseTarget("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.External {
		t.Error("HTTPS target should be external")
	}
	if tgt.Scheme != "https" {
		t.Errorf("Scheme = %q, want %q", tgt.Scheme, "https")
	}
	if tgt.Host != "example.com" {
		t.Errorf("Host = %q, want %q", tgt.Host, "example.com")
	}
	if tgt.Port != "443" {
		t.Errorf("Port = %q, want %q", tgt.Port, "443")
	}
	if tgt.Service != "" {
		t.Errorf("Service = %q, want empty for external", tgt.Service)
	}
}

func TestParseTarget_WSS(t *testing.T) {
	tgt, err := ParseTarget("wss://ws.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.External {
		t.Error("WSS target should be external")
	}
	if tgt.Scheme != "wss" {
		t.Errorf("Scheme = %q, want %q", tgt.Scheme, "wss")
	}
	if tgt.Host != "ws.example.com" {
		t.Errorf("Host = %q, want %q", tgt.Host, "ws.example.com")
	}
	if tgt.Port != "443" {
		t.Errorf("Port = %q, want %q", tgt.Port, "443")
	}
}

func TestParseTarget_Postgres(t *testing.T) {
	tgt, err := ParseTarget("postgres://db.internal")
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.External {
		t.Error("postgres target should be external")
	}
	if tgt.Scheme != "postgres" {
		t.Errorf("Scheme = %q, want %q", tgt.Scheme, "postgres")
	}
	if tgt.Host != "db.internal" {
		t.Errorf("Host = %q, want %q", tgt.Host, "db.internal")
	}
	if tgt.Port != "5432" {
		t.Errorf("Port = %q, want %q", tgt.Port, "5432")
	}
}

func TestParseTarget_NATS(t *testing.T) {
	tgt, err := ParseTarget("nats://nats.cluster")
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.External {
		t.Error("nats target should be external")
	}
	if tgt.Scheme != "nats" {
		t.Errorf("Scheme = %q, want %q", tgt.Scheme, "nats")
	}
	if tgt.Host != "nats.cluster" {
		t.Errorf("Host = %q, want %q", tgt.Host, "nats.cluster")
	}
	if tgt.Port != "4222" {
		t.Errorf("Port = %q, want %q", tgt.Port, "4222")
	}
}

func TestParseTarget_Redis(t *testing.T) {
	tgt, err := ParseTarget("redis://cache.internal")
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.External {
		t.Error("redis target should be external")
	}
	if tgt.Scheme != "redis" {
		t.Errorf("Scheme = %q, want %q", tgt.Scheme, "redis")
	}
	if tgt.Host != "cache.internal" {
		t.Errorf("Host = %q, want %q", tgt.Host, "cache.internal")
	}
	if tgt.Port != "6379" {
		t.Errorf("Port = %q, want %q", tgt.Port, "6379")
	}
}

func TestParseTarget_CustomPort(t *testing.T) {
	tgt, err := ParseTarget("https://example.com:8443")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Port != "8443" {
		t.Errorf("Port = %q, want %q", tgt.Port, "8443")
	}
	if tgt.Host != "example.com" {
		t.Errorf("Host = %q, want %q", tgt.Host, "example.com")
	}
}

func TestParseTarget_Invalid(t *testing.T) {
	_, err := ParseTarget("ftp://files.example.com")
	if err == nil {
		t.Fatal("expected error for unsupported scheme ftp")
	}
}

// --- ParseYAML tests ---

func TestParseYAML_SingleProfile(t *testing.T) {
	data := []byte(`
profiles:
  default:
    routes:
      /api: api
      /: frontend
`)
	yf, err := ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := yf.Profiles["default"]
	if !ok {
		t.Fatal("missing default profile")
	}
	if len(p.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(p.Routes))
	}
	if p.Routes["/api"] != "api" {
		t.Errorf("route /api = %q, want %q", p.Routes["/api"], "api")
	}
	if p.Routes["/"] != "frontend" {
		t.Errorf("route / = %q, want %q", p.Routes["/"], "frontend")
	}
}

func TestParseYAML_Invalid(t *testing.T) {
	data := []byte(`{{{not yaml`)
	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseYAML_Empty(t *testing.T) {
	data := []byte(`
profiles:
`)
	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error for empty profiles")
	}
}

// --- MergeProfiles tests ---

func TestMergeProfiles_NoExtends(t *testing.T) {
	profiles := map[string]Profile{
		"default": {
			Routes: map[string]string{
				"/api": "api",
				"/":    "frontend",
			},
		},
	}
	merged, err := MergeProfiles(profiles, []string{"default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(merged))
	}
	if merged["/api"] != "api" {
		t.Errorf("/api = %q, want %q", merged["/api"], "api")
	}
}

func TestMergeProfiles_SingleExtend(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Routes: map[string]string{
				"/api": "api",
				"/":    "frontend",
			},
		},
		"dev": {
			Extends: []string{"base"},
			Routes: map[string]string{
				"/debug": "debugger",
			},
		},
	}
	merged, err := MergeProfiles(profiles, []string{"dev"})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(merged))
	}
	if merged["/api"] != "api" {
		t.Errorf("/api = %q, want %q", merged["/api"], "api")
	}
	if merged["/debug"] != "debugger" {
		t.Errorf("/debug = %q, want %q", merged["/debug"], "debugger")
	}
}

func TestMergeProfiles_MultiExtend(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Routes: map[string]string{
				"/": "frontend",
			},
		},
		"monitoring": {
			Routes: map[string]string{
				"/metrics": "prometheus",
			},
		},
		"full": {
			Extends: []string{"base", "monitoring"},
			Routes: map[string]string{
				"/api": "api",
			},
		},
	}
	merged, err := MergeProfiles(profiles, []string{"full"})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(merged))
	}
	if merged["/"] != "frontend" {
		t.Errorf("/ = %q, want %q", merged["/"], "frontend")
	}
	if merged["/metrics"] != "prometheus" {
		t.Errorf("/metrics = %q, want %q", merged["/metrics"], "prometheus")
	}
	if merged["/api"] != "api" {
		t.Errorf("/api = %q, want %q", merged["/api"], "api")
	}
}

func TestMergeProfiles_LastWriteWins(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Routes: map[string]string{
				"/api": "api-v1",
			},
		},
		"override": {
			Extends: []string{"base"},
			Routes: map[string]string{
				"/api": "api-v2",
			},
		},
	}
	merged, err := MergeProfiles(profiles, []string{"override"})
	if err != nil {
		t.Fatal(err)
	}
	if merged["/api"] != "api-v2" {
		t.Errorf("/api = %q, want %q (last write wins)", merged["/api"], "api-v2")
	}
}

func TestMergeProfiles_Circular(t *testing.T) {
	profiles := map[string]Profile{
		"a": {
			Extends: []string{"b"},
			Routes:  map[string]string{"/a": "a"},
		},
		"b": {
			Extends: []string{"a"},
			Routes:  map[string]string{"/b": "b"},
		},
	}
	_, err := MergeProfiles(profiles, []string{"a"})
	if err == nil {
		t.Fatal("expected error for circular extends")
	}
}

func TestMergeProfiles_Missing(t *testing.T) {
	profiles := map[string]Profile{
		"default": {
			Extends: []string{"nonexistent"},
			Routes:  map[string]string{"/": "frontend"},
		},
	}
	_, err := MergeProfiles(profiles, []string{"default"})
	if err == nil {
		t.Fatal("expected error for missing profile reference")
	}
}

// --- ResolveProfile tests ---

func TestResolveProfile_Default(t *testing.T) {
	data := []byte(`
profiles:
  default:
    routes:
      /api: api
      /: frontend
`)
	f, targets, err := ResolveProfile(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(f.Entries))
	}
	// Should find api and frontend services
	found := map[string]bool{}
	for _, e := range f.Entries {
		found[e.Service] = true
	}
	if !found["api"] {
		t.Error("missing service api")
	}
	if !found["frontend"] {
		t.Error("missing service frontend")
	}
	// Targets map should have entries for both paths
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets["/api"].Service != "api" {
		t.Errorf("target /api Service = %q, want %q", targets["/api"].Service, "api")
	}
	if targets["/"].Service != "frontend" {
		t.Errorf("target / Service = %q, want %q", targets["/"].Service, "frontend")
	}
}

func TestResolveProfile_Named(t *testing.T) {
	data := []byte(`
profiles:
  staging:
    routes:
      /api: https://staging-api.example.com
      /: frontend
`)
	f, targets, err := ResolveProfile(data, []string{"staging"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(f.Entries))
	}
	// /api is external so Service should be empty; / is bare so Service = "frontend"
	for _, e := range f.Entries {
		if e.Path == "/" && e.Service != "frontend" {
			t.Errorf("/ Service = %q, want %q", e.Service, "frontend")
		}
		if e.Path == "/api" && e.Service != "" {
			t.Errorf("/api Service = %q, want empty for external", e.Service)
		}
	}
	// External target should have parsed URL info
	apiTarget := targets["/api"]
	if !apiTarget.External {
		t.Error("/api target should be external")
	}
	if apiTarget.Host != "staging-api.example.com" {
		t.Errorf("/api target Host = %q, want %q", apiTarget.Host, "staging-api.example.com")
	}
	if apiTarget.Port != "443" {
		t.Errorf("/api target Port = %q, want %q", apiTarget.Port, "443")
	}
}

func TestResolveProfile_CLIMerge(t *testing.T) {
	data := []byte(`
profiles:
  base:
    routes:
      /: frontend
  extra:
    routes:
      /api: api
`)
	// Passing multiple profile names simulates CLI merge: base + extra
	f, _, err := ResolveProfile(data, []string{"base", "extra"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(f.Entries))
	}
	found := map[string]bool{}
	for _, e := range f.Entries {
		found[e.Service] = true
	}
	if !found["frontend"] {
		t.Error("missing service frontend")
	}
	if !found["api"] {
		t.Error("missing service api")
	}
}

func TestResolveProfile_SortsByLen(t *testing.T) {
	data := []byte(`
profiles:
  default:
    routes:
      /: frontend
      /api/v2: api-v2
      /api: api
`)
	f, _, err := ResolveProfile(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(f.Entries))
	}
	// Sorted by path length descending: /api/v2 (7), /api (4), / (1)
	if f.Entries[0].Path != "/api/v2" {
		t.Errorf("entry[0].Path = %q, want %q", f.Entries[0].Path, "/api/v2")
	}
	if f.Entries[1].Path != "/api" {
		t.Errorf("entry[1].Path = %q, want %q", f.Entries[1].Path, "/api")
	}
	if f.Entries[2].Path != "/" {
		t.Errorf("entry[2].Path = %q, want %q", f.Entries[2].Path, "/")
	}
}
