package fabric

import (
	"fmt"
	"sort"
	"strings"
)

const defaultDomain = "localhost"

// IdentityInput provides discovered and overridden project identity facts.
type IdentityInput struct {
	Repo     string
	Branch   string
	Worktree string
	App      string
	Domain   string
}

// IdentityFacts are canonical labels used by downstream planners.
type IdentityFacts struct {
	Repo     Label
	Scope    Label
	App      Label
	Domain   string
	RawRepo  string
	RawScope string
}

// HostGraph owns all generated service and route hosts for one run.
type HostGraph struct {
	Identity IdentityFacts
	Services map[string]string
	Routes   map[string]RouteHostFact
}

// RouteHostFact is the generated host and visibility for one logical route.
type RouteHostFact struct {
	Name       string
	Host       string
	Visibility RouteVisibility
}

// ResolveIdentity merges config overrides over discovered git/worktree facts.
func ResolveIdentity(cfg IdentityConfig, input IdentityInput) (IdentityFacts, error) {
	repoRaw := firstNonEmpty(cfg.Repo, input.Repo)
	scopeRaw := firstNonEmpty(cfg.Branch, input.Branch, cfg.Worktree, input.Worktree)
	appRaw := firstNonEmpty(cfg.App, input.App, repoRaw)
	domain := firstNonEmpty(cfg.Domain, input.Domain, defaultDomain)

	repo, err := canonicalLabel("repo", repoRaw)
	if err != nil {
		return IdentityFacts{}, err
	}
	scope, err := canonicalLabel("scope", scopeRaw)
	if err != nil {
		return IdentityFacts{}, err
	}
	app, err := canonicalLabel("app", appRaw)
	if err != nil {
		return IdentityFacts{}, err
	}
	if strings.TrimSpace(domain) == "" || strings.ContainsAny(domain, "/:@#*?& \t\n\\") {
		return IdentityFacts{}, fmt.Errorf("identity.domain: invalid domain %q", domain)
	}
	return IdentityFacts{
		Repo:     repo,
		Scope:    scope,
		App:      app,
		Domain:   strings.TrimSpace(domain),
		RawRepo:  repoRaw,
		RawScope: scopeRaw,
	}, nil
}

// BuildHostGraph creates deterministic route and service hosts and rejects conflicts.
func BuildHostGraph(cfg *Config, identity IdentityFacts) (*HostGraph, error) {
	graph := &HostGraph{
		Identity: identity,
		Services: map[string]string{},
		Routes:   map[string]RouteHostFact{},
	}
	owners := map[string]string{}

	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		label, err := canonicalLabel("service", name)
		if err != nil {
			return nil, err
		}
		host := joinHost(label.Value, identity.Scope.Value, identity.Repo.Value, identity.Domain)
		if err := claimHost(owners, host, "service."+name); err != nil {
			return nil, err
		}
		graph.Services[name] = host
	}

	routeNames := make([]string, 0, len(cfg.Routes))
	for name := range cfg.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, name := range routeNames {
		route := cfg.Routes[name]
		host := route.Host
		if host == "" {
			labelRaw := firstNonEmpty(route.As, name)
			label, err := canonicalLabel("route", labelRaw)
			if err != nil {
				return nil, err
			}
			host = joinHost(label.Value, identity.Scope.Value, identity.Repo.Value, identity.Domain)
		}
		if err := claimHost(owners, host, "route."+name); err != nil {
			return nil, err
		}
		graph.Routes[name] = RouteHostFact{
			Name:       name,
			Host:       host,
			Visibility: route.Visibility,
		}
	}

	return graph, nil
}

func claimHost(owners map[string]string, host, owner string) error {
	if previous, ok := owners[host]; ok {
		return fmt.Errorf("host %q owned by both %s and %s", host, previous, owner)
	}
	owners[host] = owner
	return nil
}

func joinHost(parts ...string) string {
	return strings.Join(parts, ".")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
