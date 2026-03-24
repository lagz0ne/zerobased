package routes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	content := `# comment
/api     api
/ws      ws
/        frontend
`
	os.WriteFile(filepath.Join(dir, Filename), []byte(content), 0644)

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

func TestLoadInvalidPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, Filename), []byte("noslash api\n"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for path without /")
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

func TestLoad_TextPopulatesTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, Filename), []byte("/api api\n"), 0644)

	rf, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := rf.Entries[0]
	if e.Target.Service != "api" || e.Target.External {
		t.Errorf("text entry Target = %+v, want bare service", e.Target)
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

func TestLoad_YAMLPrecedence(t *testing.T) {
	dir := t.TempDir()
	// Text file with different content
	os.WriteFile(filepath.Join(dir, Filename), []byte("/old old-service\n"), 0644)
	// YAML file takes precedence
	yaml := `profiles:
  default:
    routes:
      /new: new-service
`
	os.WriteFile(filepath.Join(dir, YAMLFilename), []byte(yaml), 0644)

	rf, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rf.Entries[0].Service != "new-service" {
		t.Errorf("YAML should take precedence, got %q", rf.Entries[0].Service)
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

func TestLoadWithProfile_TextRejectsProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, Filename), []byte("/api api\n"), 0644)

	_, err := LoadWithProfile(dir, []string{"staging"})
	if err == nil {
		t.Fatal("text format should reject --profile")
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
