package routes

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Target represents a parsed route target — either a local service name or an external URL.
type Target struct {
	Raw      string // original value
	Scheme   string // "", "https", "wss", "postgres", "nats", "redis"
	Host     string // for external
	Port     string // for external (defaulted per scheme)
	Service  string // for bare names
	External bool
}

// Profile represents a named routing profile in the YAML file.
type Profile struct {
	Extends []string          `yaml:"extends"`
	Routes  map[string]string `yaml:"routes"`
}

// YAMLFile represents the top-level structure of a zerobased YAML routefile.
type YAMLFile struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

// defaultPorts maps supported schemes to their default port.
var defaultPorts = map[string]string{
	"https":    "443",
	"wss":      "443",
	"postgres": "5432",
	"nats":     "4222",
	"redis":    "6379",
}

// ParseTarget parses a route target string into a Target.
// Bare names (no scheme) become local service references.
func ParseTarget(raw string) (Target, error) {
	if !strings.Contains(raw, "://") {
		return Target{Raw: raw, Service: raw}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("parse target %q: %w", raw, err)
	}

	port, ok := defaultPorts[u.Scheme]
	if !ok {
		return Target{}, fmt.Errorf("unsupported scheme %q in target %q", u.Scheme, raw)
	}
	if p := u.Port(); p != "" {
		port = p
	}

	return Target{
		Raw:      raw,
		Scheme:   u.Scheme,
		Host:     u.Hostname(),
		Port:     port,
		External: true,
	}, nil
}

// ParseYAML unmarshals YAML data into a YAMLFile. Returns an error if no profiles are defined.
func ParseYAML(data []byte) (*YAMLFile, error) {
	var yf YAMLFile
	if err := yaml.Unmarshal(data, &yf); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if len(yf.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles defined")
	}
	return &yf, nil
}

// MergeProfiles merges the given profile names in order, recursively resolving extends chains.
// Last-write-wins for duplicate route paths. Detects circular extends.
func MergeProfiles(profiles map[string]Profile, names []string) (map[string]string, error) {
	merged := make(map[string]string)

	for _, name := range names {
		if err := resolveExtends(profiles, name, merged, nil); err != nil {
			return nil, err
		}
	}

	return merged, nil
}

// resolveExtends recursively resolves a profile's extends chain and overlays its routes.
func resolveExtends(profiles map[string]Profile, name string, merged map[string]string, visited []string) error {
	// Cycle detection
	for _, v := range visited {
		if v == name {
			return fmt.Errorf("circular extends: %s -> %s", strings.Join(visited, " -> "), name)
		}
	}

	p, ok := profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	visited = append(visited, name)

	// Resolve extends first (base routes applied before this profile's routes)
	for _, ext := range p.Extends {
		if err := resolveExtends(profiles, ext, merged, visited); err != nil {
			return err
		}
	}

	// Overlay this profile's routes (last-write-wins)
	for path, target := range p.Routes {
		merged[path] = target
	}

	return nil
}

// ResolveProfile orchestrates: ParseYAML → MergeProfiles → ParseTarget for each entry.
// Returns a *File with Entries sorted by path length descending, plus a map of parsed Targets keyed by path.
// If profileNames is empty, defaults to ["default"].
func ResolveProfile(data []byte, profileNames []string) (*File, map[string]Target, error) {
	if len(profileNames) == 0 {
		profileNames = []string{"default"}
	}

	yf, err := ParseYAML(data)
	if err != nil {
		return nil, nil, err
	}

	merged, err := MergeProfiles(yf.Profiles, profileNames)
	if err != nil {
		return nil, nil, err
	}

	var entries []Entry
	targets := make(map[string]Target)
	for path, raw := range merged {
		tgt, err := ParseTarget(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("route %q: %w", path, err)
		}

		entries = append(entries, Entry{
			Path:    path,
			Service: tgt.Service, // empty for external targets
		})
		targets[path] = tgt
	}

	// Sort by path length descending — most specific first.
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].Path) > len(entries[j].Path)
	})

	return &File{Entries: entries}, targets, nil
}
