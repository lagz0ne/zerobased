package routes

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const Filename = "zerobased.routes"
const YAMLFilename = "zerobased.routes.yaml"

// Entry represents one line in a routefile: path prefix → target.
type Entry struct {
	Path    string // e.g., "/api", "/ws", "/"
	Service string // docker-compose service name (empty for external targets)
	Target  Target // full target info (External=true for URLs)
}

// File represents a parsed routefile.
type File struct {
	Entries []Entry
	Gateway string // project.localhost
}

// Load reads a routefile from the given directory. Returns nil if no routefile exists.
// Checks for YAML format first (zerobased.routes.yaml), falls back to text format.
func Load(dir string) (*File, error) {
	return LoadWithProfile(dir, nil)
}

// LoadWithProfile loads a routefile with profile selection.
// If profiles is nil/empty, uses "default" for YAML or ignores for text format.
func LoadWithProfile(dir string, profiles []string) (*File, error) {
	yamlPath := filepath.Join(dir, YAMLFilename)
	data, yamlErr := os.ReadFile(yamlPath)
	if yamlErr == nil {
		rf, targets, err := ResolveProfile(data, profiles)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", YAMLFilename, err)
		}
		for i := range rf.Entries {
			if t, ok := targets[rf.Entries[i].Path]; ok {
				rf.Entries[i].Target = t
			}
		}
		return rf, nil
	}
	if !os.IsNotExist(yamlErr) {
		return nil, fmt.Errorf("read %s: %w", YAMLFilename, yamlErr)
	}

	// Fall back to text format
	if len(profiles) > 0 {
		return nil, fmt.Errorf("--profile requires %s (text format does not support profiles)", YAMLFilename)
	}
	return loadText(dir)
}

// loadText reads the old two-column text format.
func loadText(dir string) (*File, error) {
	path := filepath.Join(dir, Filename)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected 'path service', got %q", Filename, lineNum, line)
		}

		path, service := fields[0], fields[1]
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("%s:%d: path must start with /, got %q", Filename, lineNum, path)
		}

		entries = append(entries, Entry{
			Path:    path,
			Service: service,
			Target:  Target{Raw: service, Service: service},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", Filename, err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no routes defined", Filename)
	}

	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].Path) > len(entries[j].Path)
	})

	return &File{Entries: entries}, nil
}

// FindService looks up the service name for a given route name (used by `run`).
func (f *File) FindService(name string) *Entry {
	for i := range f.Entries {
		if f.Entries[i].Service == name {
			return &f.Entries[i]
		}
	}
	return nil
}

// FindByDir tries to load a routefile from the given directory.
// Convenience wrapper around Load.
func FindByDir(dir string) *File {
	rf, _ := Load(dir)
	return rf
}
