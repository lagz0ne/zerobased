package routes

import (
	"fmt"
	"os"
	"path/filepath"
)

const legacyFilename = "zerobased.routes"
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

// Load reads the supported YAML routefile from the given directory.
// Returns nil if no routefile exists.
func Load(dir string) (*File, error) {
	return LoadWithProfile(dir, nil)
}

// LoadWithProfile loads the supported YAML routefile with profile selection.
// If profiles is nil/empty, "default" is used.
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

	legacyPath := filepath.Join(dir, legacyFilename)
	if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
		return nil, fmt.Errorf("%s is no longer supported; use %s", legacyFilename, YAMLFilename)
	} else if !os.IsNotExist(legacyErr) {
		return nil, fmt.Errorf("stat %s: %w", legacyFilename, legacyErr)
	}

	if len(profiles) > 0 {
		return nil, fmt.Errorf("--profile requires %s", YAMLFilename)
	}
	return nil, nil
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
