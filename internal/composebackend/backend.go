package composebackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"gopkg.in/yaml.v3"

	"github.com/lagz0ne/zerobased/internal/zberr"
)

type Ownership string

const (
	OwnershipOwned  Ownership = "owned"
	OwnershipReused Ownership = "reused"
	OwnershipShared Ownership = "shared"
)

type Request struct {
	ProjectDir      string
	StackName       string
	Profile         string
	SessionID       string
	Files           []string
	ComposeProfiles []string
	Services        []string
	Ownership       Ownership
}

type EffectiveProject struct {
	Project *types.Project
	Files   []string
}

type Project = types.Project

type Lifecycle interface {
	Up(context.Context, *Project) error
	Down(context.Context, *Project) error
}

type RunningStack interface {
	Stop(context.Context) error
}

type Backend struct {
	lifecycle Lifecycle
}

func NewBackend(lifecycle Lifecycle) Backend {
	return Backend{lifecycle: lifecycle}
}

func (backend Backend) Start(ctx context.Context, request Request) (RunningStack, error) {
	loaded, err := Load(ctx, request)
	if err != nil {
		return nil, err
	}
	lifecycle, err := backend.lifecycleOrDefault()
	if err != nil {
		return nil, lifecycleFailed(err.Error())
	}
	if err := lifecycle.Up(ctx, loaded.Project); err != nil {
		_ = lifecycle.Down(context.Background(), loaded.Project)
		return nil, lifecycleFailed(err.Error())
	}
	return loadedStack{project: loaded.Project, lifecycle: lifecycle}, nil
}

type loadedStack struct {
	project   *types.Project
	lifecycle Lifecycle
}

func (stack loadedStack) Stop(ctx context.Context) error {
	if stack.lifecycle == nil {
		return nil
	}
	if err := stack.lifecycle.Down(ctx, stack.project); err != nil {
		return lifecycleFailed(err.Error())
	}
	return nil
}

func (backend Backend) lifecycleOrDefault() (Lifecycle, error) {
	if backend.lifecycle != nil {
		return backend.lifecycle, nil
	}
	return newDefaultLifecycle()
}

func Load(ctx context.Context, request Request) (EffectiveProject, error) {
	if len(request.Files) == 0 {
		return EffectiveProject{}, modelInvalid("compose.files is required")
	}
	if request.Ownership == "" {
		request.Ownership = OwnershipOwned
	}
	projectDir, err := filepath.Abs(request.ProjectDir)
	if err != nil {
		return EffectiveProject{}, modelInvalid(fmt.Sprintf("resolve project dir: %v", err))
	}
	if request.StackName == "" {
		request.StackName = filepath.Base(projectDir)
	}
	if request.Profile == "" {
		request.Profile = "dev"
	}

	files, err := resolveFiles(projectDir, request.Files)
	if err != nil {
		return EffectiveProject{}, err
	}
	rawPolicies, err := loadRawPolicies(files)
	if err != nil {
		return EffectiveProject{}, err
	}

	projectName := ProjectName(request.StackName, request.Profile, projectDir)
	options, err := composecli.NewProjectOptions(files,
		composecli.WithWorkingDirectory(projectDir),
		composecli.WithEnvFiles(),
		composecli.WithOsEnv,
		composecli.WithDotEnv,
		composecli.WithName(projectName),
		composecli.WithProfiles(request.ComposeProfiles),
	)
	if err != nil {
		return EffectiveProject{}, modelInvalid(err.Error())
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return EffectiveProject{}, modelInvalid(err.Error())
	}
	project.Name = projectName
	project.WorkingDir = projectDir

	if len(request.Services) > 0 {
		project, err = project.WithSelectedServices(request.Services, types.IncludeDependencies)
		if err != nil {
			return EffectiveProject{}, modelInvalid(err.Error())
		}
	}
	project = project.WithoutUnnecessaryResources()
	project.Name = projectName
	project.WorkingDir = projectDir

	if err := validateIsolation(project, rawPolicies, request.Ownership); err != nil {
		return EffectiveProject{}, err
	}

	project, err = injectLabels(project, request, projectDir, projectName)
	if err != nil {
		return EffectiveProject{}, zberr.Critical(zberr.LayerComposeBackend, err)
	}
	project.Name = projectName
	project.WorkingDir = projectDir

	return EffectiveProject{Project: project, Files: files}, nil
}

func ProjectName(stackName string, profile string, projectDir string) string {
	stackName = sanitizeName(stackName)
	profile = sanitizeName(profile)
	if stackName == "" {
		stackName = "stack"
	}
	if profile == "" {
		profile = "dev"
	}
	sum := sha256.Sum256([]byte(projectDir + "\x00" + stackName + "\x00" + profile))
	return fmt.Sprintf("zb_%s_%s_%s", stackName, profile, hex.EncodeToString(sum[:])[:12])
}

func resolveFiles(projectDir string, files []string) ([]string, error) {
	resolved := make([]string, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file) == "" {
			return nil, modelInvalid("compose file path is empty")
		}
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, modelInvalid(fmt.Sprintf("resolve compose file %s: %v", file, err))
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, modelInvalid(fmt.Sprintf("compose file %s: %v", file, err))
		}
		resolved = append(resolved, abs)
	}
	return resolved, nil
}

type rawComposePolicies struct {
	networkNames    map[string]string
	networkExternal map[string]bool
	volumeNames     map[string]string
	volumeExternal  map[string]bool
	secretNames     map[string]string
	secretExternal  map[string]bool
	configNames     map[string]string
	configExternal  map[string]bool
}

type rawComposeFile struct {
	Networks map[string]rawResource `yaml:"networks"`
	Volumes  map[string]rawResource `yaml:"volumes"`
	Secrets  map[string]rawResource `yaml:"secrets"`
	Configs  map[string]rawResource `yaml:"configs"`
}

type rawResource struct {
	Name     string      `yaml:"name"`
	External rawExternal `yaml:"external"`
}

type rawExternal struct {
	set    bool
	active bool
}

func (external *rawExternal) UnmarshalYAML(node *yaml.Node) error {
	external.set = true
	switch node.Kind {
	case yaml.ScalarNode:
		var value bool
		if err := node.Decode(&value); err != nil {
			return err
		}
		external.active = value
	case yaml.MappingNode:
		external.active = true
	default:
		external.active = false
	}
	return nil
}

func loadRawPolicies(files []string) (rawComposePolicies, error) {
	policies := rawComposePolicies{
		networkNames:    make(map[string]string),
		networkExternal: make(map[string]bool),
		volumeNames:     make(map[string]string),
		volumeExternal:  make(map[string]bool),
		secretNames:     make(map[string]string),
		secretExternal:  make(map[string]bool),
		configNames:     make(map[string]string),
		configExternal:  make(map[string]bool),
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return policies, modelInvalid(fmt.Sprintf("read compose file %s: %v", file, err))
		}
		var raw rawComposeFile
		if err := yaml.Unmarshal(content, &raw); err != nil {
			return policies, modelInvalid(fmt.Sprintf("parse compose file %s: %v", file, err))
		}
		for name, network := range raw.Networks {
			if network.Name != "" {
				policies.networkNames[name] = network.Name
			}
			if network.External.active {
				policies.networkExternal[name] = true
			}
		}
		for name, volume := range raw.Volumes {
			if volume.Name != "" {
				policies.volumeNames[name] = volume.Name
			}
			if volume.External.active {
				policies.volumeExternal[name] = true
			}
		}
		for name, secret := range raw.Secrets {
			if secret.Name != "" {
				policies.secretNames[name] = secret.Name
			}
			if secret.External.active {
				policies.secretExternal[name] = true
			}
		}
		for name, config := range raw.Configs {
			if config.Name != "" {
				policies.configNames[name] = config.Name
			}
			if config.External.active {
				policies.configExternal[name] = true
			}
		}
	}
	return policies, nil
}

func validateIsolation(project *types.Project, policies rawComposePolicies, ownership Ownership) error {
	if ownership != OwnershipOwned {
		return nil
	}

	serviceNames := project.ServiceNames()
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		service, err := project.GetService(name)
		if err != nil {
			return modelInvalid(err.Error())
		}
		if service.ContainerName != "" {
			return isolationRejected(fmt.Sprintf("service %s uses container_name", name))
		}
		if len(service.Ports) > 0 {
			return isolationRejected(fmt.Sprintf("service %s publishes host ports", name))
		}
		if service.NetworkMode == "host" || service.Net == "host" {
			return isolationRejected(fmt.Sprintf("service %s uses host networking", name))
		}
		if service.Pid == "host" || service.Ipc == "host" || service.Cgroup == "host" {
			return isolationRejected(fmt.Sprintf("service %s uses host namespace sharing", name))
		}
		if service.Provider != nil {
			return isolationRejected(fmt.Sprintf("service %s uses provider-managed service", name))
		}
	}

	for name := range project.Networks {
		if customName := policies.networkNames[name]; customName != "" {
			return isolationRejected(fmt.Sprintf("network %s uses custom name %s", name, customName))
		}
		if policies.networkExternal[name] {
			return isolationRejected(fmt.Sprintf("network %s is external", name))
		}
	}
	for name := range project.Volumes {
		if customName := policies.volumeNames[name]; customName != "" {
			return isolationRejected(fmt.Sprintf("volume %s uses custom name %s", name, customName))
		}
		if policies.volumeExternal[name] {
			return isolationRejected(fmt.Sprintf("volume %s is external", name))
		}
	}
	for name := range project.Secrets {
		if customName := policies.secretNames[name]; customName != "" {
			return isolationRejected(fmt.Sprintf("secret %s uses custom name %s", name, customName))
		}
		if policies.secretExternal[name] {
			return isolationRejected(fmt.Sprintf("secret %s is external", name))
		}
	}
	for name := range project.Configs {
		if customName := policies.configNames[name]; customName != "" {
			return isolationRejected(fmt.Sprintf("config %s uses custom name %s", name, customName))
		}
		if policies.configExternal[name] {
			return isolationRejected(fmt.Sprintf("config %s is external", name))
		}
	}

	return nil
}

func injectLabels(project *types.Project, request Request, projectDir string, projectName string) (*types.Project, error) {
	return project.WithServicesTransform(func(_ string, service types.ServiceConfig) (types.ServiceConfig, error) {
		if service.Labels == nil {
			service.Labels = types.Labels{}
		}
		labels := map[string]string{
			"dev.zerobased.project":         projectName,
			"dev.zerobased.stack":           request.StackName,
			"dev.zerobased.profile":         request.Profile,
			"dev.zerobased.session":         request.SessionID,
			"dev.zerobased.ownership":       string(request.Ownership),
			"dev.zerobased.project_root":    projectDir,
			"dev.zerobased.compose_backend": "true",
		}
		for key, value := range labels {
			service.Labels[key] = value
		}
		return service, nil
	})
}

var invalidNameChars = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = invalidNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	return value
}

func modelInvalid(message string) error {
	return zberr.New(zberr.LayerComposeBackend, zberr.CodeModelInvalid, zberr.WithCause(fmt.Errorf("%s", message)))
}

func isolationRejected(message string) error {
	return zberr.New(zberr.LayerComposeBackend, zberr.CodeIsolationRejected, zberr.WithCause(fmt.Errorf("%s", message)))
}

func lifecycleFailed(message string) error {
	return zberr.New(zberr.LayerComposeBackend, zberr.CodeLifecycleFailed, zberr.WithCause(fmt.Errorf("%s", message)))
}
