package routes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	content := `profiles:
  default:
    routes:
      /api: api
      /ws: ws
      /: frontend
`
	os.WriteFile(filepath.Join(dir, YAMLFilename), []byte(content), 0644)

	rf, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rf == nil {
		t.Fatal("expected non-nil routefile")
	}
	if len(rf.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(rf.Entries))
	}

	// Sorted by path length descending: /api (4), /ws (3), / (1)
	if rf.Entries[0].Path != "/api" || rf.Entries[0].Service != "api" {
		t.Errorf("entry[0] = %+v, want /api → api", rf.Entries[0])
	}
	if rf.Entries[1].Path != "/ws" || rf.Entries[1].Service != "ws" {
		t.Errorf("entry[1] = %+v, want /ws → ws", rf.Entries[1])
	}
	if rf.Entries[2].Path != "/" || rf.Entries[2].Service != "frontend" {
		t.Errorf("entry[2] = %+v, want / → frontend", rf.Entries[2])
	}
}

func TestLoadNoFile(t *testing.T) {
	rf, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if rf != nil {
		t.Fatal("expected nil for missing routefile")
	}
}

func TestLoadRejectsLegacyRoutefileWithInvalidContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "zerobased.routes"), []byte("noslash api\n"), 0644)

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected legacy routefile rejection, got %v", err)
	}
}

func TestFindService(t *testing.T) {
	rf := &File{
		Entries: []Entry{
			{Path: "/api", Service: "api"},
			{Path: "/", Service: "frontend"},
		},
	}

	e := rf.FindService("api")
	if e == nil || e.Path != "/api" {
		t.Errorf("FindService(api) = %+v, want /api", e)
	}
	if rf.FindService("nonexistent") != nil {
		t.Error("FindService(nonexistent) should be nil")
	}
}

func TestLoadRejectsLegacyTextRoutefile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "zerobased.routes"), []byte("/api api\n"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected legacy routefile to be rejected")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected unsupported legacy error, got %v", err)
	}
}

func TestLoad_DetectsYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `profiles:
  default:
    routes:
      /api: api
      /: frontend
`
	os.WriteFile(filepath.Join(dir, YAMLFilename), []byte(yaml), 0644)

	rf, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rf == nil {
		t.Fatal("expected non-nil")
	}
	if len(rf.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(rf.Entries))
	}
	// /api is longer, should be first
	if rf.Entries[0].Path != "/api" || rf.Entries[0].Service != "api" {
		t.Errorf("entry[0] = %+v, want /api → api", rf.Entries[0])
	}
}

func TestLoadWithProfile(t *testing.T) {
	dir := t.TempDir()
	yaml := `profiles:
  default:
    routes:
      /api: api
      /: frontend
  staging:
    extends: [default]
    routes:
      /api: https://api.staging.com
`
	os.WriteFile(filepath.Join(dir, YAMLFilename), []byte(yaml), 0644)

	rf, err := LoadWithProfile(dir, []string{"staging"})
	if err != nil {
		t.Fatal(err)
	}
	// /api should be external (from staging override)
	for _, e := range rf.Entries {
		if e.Path == "/api" {
			if !e.Target.External {
				t.Error("/api should be external in staging profile")
			}
			if e.Service != "" {
				t.Error("/api Service should be empty for external target")
			}
			return
		}
	}
	t.Error("/api entry not found")
}

func TestLoadWithProfile_RequiresYAMLRoutefile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "zerobased.routes"), []byte("/api api\n"), 0644)

	_, err := LoadWithProfile(dir, []string{"staging"})
	if err == nil || !strings.Contains(err.Error(), YAMLFilename) {
		t.Fatalf("expected YAML-only routefile error, got %v", err)
	}
}

func TestFindService_SkipsExternal(t *testing.T) {
	rf := &File{
		Entries: []Entry{
			{Path: "/api", Service: "", Target: Target{External: true, Scheme: "https", Host: "api.com"}},
			{Path: "/", Service: "frontend", Target: Target{Service: "frontend"}},
		},
	}
	// Should not find "api" because external targets have empty Service
	if rf.FindService("api") != nil {
		t.Error("FindService should skip external targets")
	}
	if rf.FindService("frontend") == nil {
		t.Error("FindService should find bare service")
	}
}
