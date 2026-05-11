package fabric

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// PortMode describes the generic kind of a named local connection point.
type PortMode string

const (
	PortHTTP   PortMode = "http"
	PortTCP    PortMode = "tcp"
	PortSocket PortMode = "socket"
)

// PortConfig declares a named connection point.
type PortConfig struct {
	Mode PortMode `yaml:"mode"`
	Port string   `yaml:"port"`
}

// UnmarshalYAML supports both `api: http` and `api: { mode: http }`.
func (p *PortConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		p.Mode = PortMode(value.Value)
		return nil
	}
	type portConfig PortConfig
	var decoded portConfig
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*p = PortConfig(decoded)
	return nil
}

// NeedKind declares whether a dependency is required.
type NeedKind string

const (
	NeedRequired NeedKind = "required"
	NeedOptional NeedKind = "optional"
)

// NeedConfig declares a service dependency.
type NeedConfig struct {
	Kind NeedKind
}

// EndpointDelivery declares how zerobased hands an endpoint to a service.
type EndpointDelivery string

const (
	EndpointDeliveryEnvPort         EndpointDelivery = "env_port"
	EndpointDeliveryListenerHandoff EndpointDelivery = "listener_handoff"
)

// HandoffFallback declares what to do when listener handoff is unavailable.
type HandoffFallback string

const (
	HandoffFallbackFail    HandoffFallback = "fail"
	HandoffFallbackEnvPort HandoffFallback = "env_port"
)

// SourceAction declares what to do after checking for an existing endpoint.
type SourceAction string

const (
	SourceActionUse   SourceAction = "use"
	SourceActionStart SourceAction = "start"
	SourceActionFail  SourceAction = "fail"
)

// SourceConfig declares how a service instance is obtained.
type SourceConfig struct {
	Probe                     map[string]SourceProbe
	Found                     SourceAction
	Missing                   SourceAction
	AllowUnidentifiedExisting bool
}

// UnmarshalYAML supports shorthand `source: { tcp: db }`.
func (s *SourceConfig) UnmarshalYAML(value *yaml.Node) error {
	s.Missing = SourceActionStart
	s.Probe = map[string]SourceProbe{}

	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("source must be a mapping")
	}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]
		switch key {
		case "probe":
			probes, err := decodeSourceProbes(val)
			if err != nil {
				return err
			}
			s.Probe = probes
		case "found":
			var action string
			if err := val.Decode(&action); err != nil {
				return err
			}
			s.Found = SourceAction(action)
		case "missing":
			var action string
			if err := val.Decode(&action); err != nil {
				return err
			}
			s.Missing = SourceAction(action)
		case "allow_unidentified_existing":
			if err := val.Decode(&s.AllowUnidentifiedExisting); err != nil {
				return err
			}
		default:
			probe, err := decodeSourceProbe(key, val)
			if err != nil {
				return err
			}
			s.Probe[key] = probe
		}
	}
	return nil
}

// SourceProbe declares how to check an existing endpoint before using it.
type SourceProbe struct {
	Kind         string
	Port         string
	Path         string            `yaml:"path"`
	ExpectStatus int               `yaml:"expect_status"`
	ExpectHeader map[string]string `yaml:"expect_header"`
}

// ServiceConfig declares an opaque command plus env and dependencies.
type ServiceConfig struct {
	Cmd             string                `yaml:"cmd"`
	Env             map[string]string     `yaml:"env"`
	Needs           map[string]NeedConfig `yaml:"needs"`
	Ready           map[string]string     `yaml:"ready"`
	Source          *SourceConfig         `yaml:"source"`
	Endpoint        EndpointDelivery      `yaml:"endpoint"`
	HandoffFallback HandoffFallback       `yaml:"handoff_fallback"`
	Tolerant        bool                  `yaml:"tolerant"`
}

// UnmarshalYAML supports `needs: [db]` and `needs: { db: required }`.
func (s *ServiceConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawService struct {
		Cmd             string            `yaml:"cmd"`
		Env             map[string]string `yaml:"env"`
		Needs           yaml.Node         `yaml:"needs"`
		Ready           map[string]string `yaml:"ready"`
		Source          *SourceConfig     `yaml:"source"`
		Endpoint        EndpointDelivery  `yaml:"endpoint"`
		HandoffFallback HandoffFallback   `yaml:"handoff_fallback"`
		Tolerant        bool              `yaml:"tolerant"`
	}
	var raw rawService
	if err := value.Decode(&raw); err != nil {
		return err
	}

	needs, err := decodeNeeds(raw.Needs)
	if err != nil {
		return err
	}
	*s = ServiceConfig{
		Cmd:             raw.Cmd,
		Env:             raw.Env,
		Needs:           needs,
		Ready:           raw.Ready,
		Source:          raw.Source,
		Endpoint:        raw.Endpoint,
		HandoffFallback: raw.HandoffFallback,
		Tolerant:        raw.Tolerant,
	}
	return nil
}

func decodeNeeds(node yaml.Node) (map[string]NeedConfig, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	needs := map[string]NeedConfig{}
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("needs sequence entries must be service names")
			}
			needs[item.Value] = NeedConfig{Kind: NeedRequired}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			val := node.Content[i+1].Value
			if val == "" {
				val = string(NeedRequired)
			}
			needs[key] = NeedConfig{Kind: NeedKind(val)}
		}
	default:
		return nil, fmt.Errorf("needs must be a sequence or mapping")
	}
	return needs, nil
}

func decodeSourceProbes(node *yaml.Node) (map[string]SourceProbe, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("source.probe must be a mapping")
	}
	probes := map[string]SourceProbe{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		probe, err := decodeSourceProbe(key, node.Content[i+1])
		if err != nil {
			return nil, err
		}
		probes[key] = probe
	}
	return probes, nil
}

func decodeSourceProbe(kind string, node *yaml.Node) (SourceProbe, error) {
	probe := SourceProbe{Kind: kind}
	if node.Kind == yaml.ScalarNode {
		probe.Port = node.Value
		return probe, nil
	}
	type rawProbe struct {
		Port         string      `yaml:"port"`
		Path         string      `yaml:"path"`
		ExpectStatus int         `yaml:"expect_status"`
		ExpectHeader interface{} `yaml:"expect_header"`
	}
	var raw rawProbe
	if err := node.Decode(&raw); err != nil {
		return SourceProbe{}, err
	}
	headers, err := decodeExpectHeader(raw.ExpectHeader)
	if err != nil {
		return SourceProbe{}, err
	}
	probe.Port = raw.Port
	probe.Path = raw.Path
	probe.ExpectStatus = raw.ExpectStatus
	probe.ExpectHeader = headers
	return probe, nil
}

func decodeExpectHeader(value interface{}) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case string:
		name, expected, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("expect_header must be name=value")
		}
		return map[string]string{strings.TrimSpace(name): expected}, nil
	case map[string]interface{}:
		headers := map[string]string{}
		for name, expected := range v {
			text, ok := expected.(string)
			if !ok {
				return nil, fmt.Errorf("expect_header.%s must be a string", name)
			}
			headers[name] = text
		}
		return headers, nil
	default:
		return nil, fmt.Errorf("expect_header must be a string or mapping")
	}
}

// ProfileConfig declares profile-specific values and service disabling.
type ProfileConfig struct {
	Required []string
	Disable  []string
	Values   map[string]string
}

// UnmarshalYAML keeps arbitrary dotted keys as profile values.
func (p *ProfileConfig) UnmarshalYAML(value *yaml.Node) error {
	p.Values = map[string]string{}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]
		switch key {
		case "required":
			if err := val.Decode(&p.Required); err != nil {
				return err
			}
		case "disable":
			if err := val.Decode(&p.Disable); err != nil {
				return err
			}
		default:
			var rendered string
			if err := val.Decode(&rendered); err != nil {
				return fmt.Errorf("profile value %q must be a string", key)
			}
			p.Values[key] = rendered
		}
	}
	return nil
}

// RouteTarget declares a route target service plus route options.
type RouteTarget struct {
	To    string
	Strip bool
}

// UnmarshalYAML supports `/api/*: backend` and `/api/*: { to: backend }`.
func (r *RouteTarget) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.To = value.Value
		return nil
	}
	type routeTarget RouteTarget
	var decoded routeTarget
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*r = RouteTarget(decoded)
	return nil
}

// RouteHost declares a host's user-facing route table.
type RouteHost struct {
	As            string
	Host          string
	Visibility    RouteVisibility
	Internal      bool
	Routes        map[string]RouteTarget
	internalSet   bool
	visibilitySet bool
}

// RouteVisibility declares whether a route is public or private to the local fabric.
type RouteVisibility string

const (
	RouteVisibilityPublic   RouteVisibility = "public"
	RouteVisibilityInternal RouteVisibility = "internal"
)

// UnmarshalYAML keeps path keys separate from host metadata.
func (r *RouteHost) UnmarshalYAML(value *yaml.Node) error {
	r.Routes = map[string]RouteTarget{}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]
		switch key {
		case "as":
			if err := val.Decode(&r.As); err != nil {
				return err
			}
		case "host":
			if err := val.Decode(&r.Host); err != nil {
				return err
			}
		case "internal":
			if err := val.Decode(&r.Internal); err != nil {
				return err
			}
			r.internalSet = true
		case "visibility":
			if err := val.Decode(&r.Visibility); err != nil {
				return err
			}
			r.visibilitySet = true
		default:
			var target RouteTarget
			if err := val.Decode(&target); err != nil {
				return err
			}
			r.Routes[key] = target
		}
	}
	return nil
}

// IdentityConfig declares optional naming overrides for generated hosts and ports.
type IdentityConfig struct {
	App       string `yaml:"app"`
	Repo      string `yaml:"repo"`
	Branch    string `yaml:"branch"`
	Worktree  string `yaml:"worktree"`
	Domain    string `yaml:"domain"`
	PortRange string `yaml:"port_range"`
}

// Config is the pure portless config contract.
type Config struct {
	Identity IdentityConfig           `yaml:"identity"`
	Ports    map[string]PortConfig    `yaml:"ports"`
	Services map[string]ServiceConfig `yaml:"services"`
	Profiles map[string]ProfileConfig `yaml:"profiles"`
	Routes   map[string]RouteHost     `yaml:"routes"`
}

// ParseConfig parses zerobased.yaml bytes.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Ports == nil {
		cfg.Ports = map[string]PortConfig{}
	}
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceConfig{}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	if cfg.Routes == nil {
		cfg.Routes = map[string]RouteHost{}
	}
	return &cfg, nil
}

// ValidateConfig rejects config errors before runtime side effects.
func ValidateConfig(cfg *Config) error {
	if len(cfg.Ports) == 0 {
		return fmt.Errorf("ports: at least one named port is required")
	}
	for name, port := range cfg.Ports {
		if !validName(name) {
			return fmt.Errorf("ports.%s: invalid name", name)
		}
		switch port.Mode {
		case PortHTTP, PortTCP, PortSocket:
		default:
			return fmt.Errorf("ports.%s: unsupported mode %q", name, port.Mode)
		}
		if port.Port != "" && port.Port != "auto" && !isNumeric(port.Port) {
			return fmt.Errorf("ports.%s.port: must be auto or numeric", name)
		}
	}
	for name, service := range cfg.Services {
		if !validName(name) {
			return fmt.Errorf("services.%s: invalid name", name)
		}
		if strings.TrimSpace(service.Cmd) == "" {
			return fmt.Errorf("services.%s.cmd: required", name)
		}
		if service.Endpoint == "" {
			service.Endpoint = EndpointDeliveryEnvPort
		}
		switch service.Endpoint {
		case EndpointDeliveryEnvPort, EndpointDeliveryListenerHandoff:
		default:
			return fmt.Errorf("services.%s.endpoint: invalid delivery %q", name, service.Endpoint)
		}
		if service.HandoffFallback == "" {
			service.HandoffFallback = HandoffFallbackFail
		}
		switch service.HandoffFallback {
		case HandoffFallbackFail, HandoffFallbackEnvPort:
		default:
			return fmt.Errorf("services.%s.handoff_fallback: invalid fallback %q", name, service.HandoffFallback)
		}
		for dep, need := range service.Needs {
			if _, ok := cfg.Services[dep]; !ok {
				return fmt.Errorf("services.%s.needs.%s: unknown service", name, dep)
			}
			if need.Kind != NeedRequired && need.Kind != NeedOptional {
				return fmt.Errorf("services.%s.needs.%s: invalid kind %q", name, dep, need.Kind)
			}
		}
		for key, tmpl := range service.Env {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("services.%s.env: empty key", name)
			}
			if err := validateTemplateRefs(tmpl, cfg); err != nil {
				return fmt.Errorf("services.%s.env.%s: %w", name, key, err)
			}
		}
		if service.Source != nil {
			if err := validateSourceConfig(cfg, name, service.Source); err != nil {
				return err
			}
		}
		cfg.Services[name] = service
	}
	for routeName, routeHost := range cfg.Routes {
		if strings.TrimSpace(routeName) == "" {
			return fmt.Errorf("routes: empty name")
		}
		if err := normalizeRouteVisibility(routeName, &routeHost); err != nil {
			return err
		}
		cfg.Routes[routeName] = routeHost
		seen := map[string]struct{}{}
		for path, target := range routeHost.Routes {
			if !strings.HasPrefix(path, "/") {
				return fmt.Errorf("routes.%s.%s: path must start with /", routeName, path)
			}
			if _, exists := seen[path]; exists {
				return fmt.Errorf("routes.%s.%s: duplicate route", routeName, path)
			}
			seen[path] = struct{}{}
			if _, ok := cfg.Services[target.To]; !ok {
				return fmt.Errorf("routes.%s.%s: unknown service %q", routeName, path, target.To)
			}
		}
	}
	for profileName, profile := range cfg.Profiles {
		for _, disabled := range profile.Disable {
			if _, ok := cfg.Services[disabled]; !ok {
				return fmt.Errorf("profiles.%s.disable.%s: unknown service", profileName, disabled)
			}
		}
		for key := range profile.Values {
			if !validDottedKey(key) {
				return fmt.Errorf("profiles.%s.%s: invalid key", profileName, key)
			}
		}
		for _, required := range profile.Required {
			if !validDottedKey(required) {
				return fmt.Errorf("profiles.%s.required.%s: invalid key", profileName, required)
			}
		}
	}
	return nil
}

// PortFacts are concrete values for one named port.
type PortFacts map[string]string

// RenderInput provides runtime facts without performing IO.
type RenderInput struct {
	Ports     map[string]PortFacts
	Profile   string
	Overrides map[string]string
	Run       map[string]string
}

// RenderedConfig is a side-effect-free execution plan.
type RenderedConfig struct {
	Env            map[string]map[string]string
	ActiveServices []string
	Values         map[string]string
}

// RenderConfig merges profile and overrides, checks required values, and renders service env.
func RenderConfig(cfg *Config, input RenderInput) (*RenderedConfig, error) {
	values := flattenPortFacts(input.Ports)
	for k, v := range input.Run {
		values["run."+k] = v
	}

	disabled := map[string]struct{}{}
	if input.Profile != "" {
		profile, ok := cfg.Profiles[input.Profile]
		if !ok {
			return nil, fmt.Errorf("profile %q not found", input.Profile)
		}
		for k, v := range profile.Values {
			rendered, err := renderTemplate(v, values)
			if err != nil {
				return nil, fmt.Errorf("profiles.%s.%s: %w", input.Profile, k, err)
			}
			values[k] = rendered
		}
		for _, name := range profile.Disable {
			disabled[name] = struct{}{}
		}
	}
	for k, v := range input.Overrides {
		values[k] = v
	}
	if input.Profile != "" {
		for _, required := range cfg.Profiles[input.Profile].Required {
			if values[required] == "" {
				return nil, fmt.Errorf("profile %q requires %s", input.Profile, required)
			}
		}
	}

	if err := validateDisabledRefs(cfg, disabled); err != nil {
		return nil, err
	}

	env := map[string]map[string]string{}
	var active []string
	for name, service := range cfg.Services {
		if _, off := disabled[name]; off {
			continue
		}
		active = append(active, name)
		renderedEnv := map[string]string{}
		for key, tmpl := range service.Env {
			rendered, err := renderTemplate(tmpl, values)
			if err != nil {
				return nil, fmt.Errorf("services.%s.env.%s: %w", name, key, err)
			}
			renderedEnv[key] = rendered
		}
		env[name] = renderedEnv
	}
	sort.Strings(active)
	return &RenderedConfig{Env: env, ActiveServices: active, Values: values}, nil
}

func flattenPortFacts(ports map[string]PortFacts) map[string]string {
	values := map[string]string{}
	for portName, facts := range ports {
		for key, value := range facts {
			values[portName+"."+key] = value
		}
	}
	return values
}

func validateDisabledRefs(cfg *Config, disabled map[string]struct{}) error {
	for routeName, routeHost := range cfg.Routes {
		for path, target := range routeHost.Routes {
			if _, off := disabled[target.To]; off {
				return fmt.Errorf("routes.%s.%s: target %q disabled by profile", routeName, path, target.To)
			}
		}
	}
	return nil
}

func validateSourceConfig(cfg *Config, serviceName string, source *SourceConfig) error {
	if len(source.Probe) == 0 {
		return fmt.Errorf("services.%s.source: probe required", serviceName)
	}
	if source.Found == "" {
		if sourceHasIdentityProof(source) {
			source.Found = SourceActionUse
		} else {
			source.Found = SourceActionFail
		}
	}
	if source.Missing == "" {
		source.Missing = SourceActionStart
	}
	if !validSourceAction(source.Found) {
		return fmt.Errorf("services.%s.source.found: invalid action %q", serviceName, source.Found)
	}
	if !validSourceAction(source.Missing) {
		return fmt.Errorf("services.%s.source.missing: invalid action %q", serviceName, source.Missing)
	}
	for probeKind, probe := range source.Probe {
		switch probeKind {
		case "tcp", "http":
		default:
			return fmt.Errorf("services.%s.source.%s: unsupported probe", serviceName, probeKind)
		}
		if probe.Kind == "" {
			probe.Kind = probeKind
			source.Probe[probeKind] = probe
		}
		if probe.Port == "" {
			return fmt.Errorf("services.%s.source.%s.port: required", serviceName, probeKind)
		}
		if _, ok := cfg.Ports[probe.Port]; !ok {
			return fmt.Errorf("services.%s.source.%s: unknown port %q", serviceName, probeKind, probe.Port)
		}
	}
	if source.Found == SourceActionUse && !source.AllowUnidentifiedExisting && !sourceHasIdentityProof(source) {
		return fmt.Errorf("services.%s.source.found: use requires identity proof or allow_unidentified_existing", serviceName)
	}
	return nil
}

func normalizeRouteVisibility(routeName string, route *RouteHost) error {
	if !route.visibilitySet {
		if route.internalSet && route.Internal {
			route.Visibility = RouteVisibilityInternal
		} else {
			route.Visibility = RouteVisibilityPublic
		}
		return nil
	}
	switch route.Visibility {
	case RouteVisibilityPublic, RouteVisibilityInternal:
	default:
		return fmt.Errorf("routes.%s.visibility: invalid value %q", routeName, route.Visibility)
	}
	if route.internalSet {
		want := RouteVisibilityPublic
		if route.Internal {
			want = RouteVisibilityInternal
		}
		if route.Visibility != want {
			return fmt.Errorf("routes.%s: route.visibility_conflict", routeName)
		}
	}
	return nil
}

func sourceHasIdentityProof(source *SourceConfig) bool {
	for _, probe := range source.Probe {
		if probe.Kind != "http" {
			continue
		}
		if probe.ExpectStatus > 0 && len(probe.ExpectHeader) > 0 {
			return true
		}
	}
	return false
}

func validSourceAction(action SourceAction) bool {
	switch action {
	case SourceActionUse, SourceActionStart, SourceActionFail:
		return true
	default:
		return false
	}
}

var templateRefPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func renderTemplate(tmpl string, values map[string]string) (string, error) {
	var missing []string
	result := templateRefPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		parts := templateRefPattern.FindStringSubmatch(match)
		key := parts[1]
		value, ok := values[key]
		if !ok {
			missing = append(missing, key)
			return match
		}
		return value
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("unknown template ref %s", strings.Join(missing, ", "))
	}
	return result, nil
}

func validateTemplateRefs(tmpl string, cfg *Config) error {
	refs := templateRefPattern.FindAllStringSubmatch(tmpl, -1)
	for _, ref := range refs {
		key := ref[1]
		if strings.HasPrefix(key, "run.") {
			continue
		}
		head, _, ok := strings.Cut(key, ".")
		if !ok {
			return fmt.Errorf("template ref %q must be dotted", key)
		}
		if _, ok := cfg.Ports[head]; ok {
			continue
		}
		return fmt.Errorf("unknown template ref %q", key)
	}
	return nil
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validDottedKey(key string) bool {
	if key == "" {
		return false
	}
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !validName(part) {
			return false
		}
	}
	return true
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
