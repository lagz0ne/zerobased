package route_runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lagz0ne/zerobased/internal/controlplane"
	"github.com/lagz0ne/zerobased/internal/zberr"
)

type DockerCaddyOptions struct {
	Home          string
	Runner        commandRunner
	ContainerName string
	Image         string
	UpstreamHost  string
}

type DockerCaddy struct {
	home          string
	configPath    string
	runner        commandRunner
	containerName string
	image         string
	upstreamHost  string
	mu            sync.Mutex
	publications  map[string]controlplane.Publication
}

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execCommandRunner struct{}

const (
	defaultCaddyContainerName = "zerobased-route-runtime-caddy"
	defaultCaddyImage         = "caddy:2-alpine"
)

var validCaddyHost = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)

func NewDockerCaddy(options DockerCaddyOptions) *DockerCaddy {
	home := options.Home
	if home != "" {
		home = filepath.Clean(home)
	}
	runner := options.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	containerName := options.ContainerName
	if containerName == "" {
		containerName = defaultCaddyContainerName
	}
	image := options.Image
	if image == "" {
		image = defaultCaddyImage
	}
	upstreamHost := options.UpstreamHost
	if upstreamHost == "" {
		upstreamHost = "host.docker.internal"
	}
	configPath := filepath.Join(home, "caddy", "Caddyfile")
	return &DockerCaddy{
		home:          home,
		configPath:    configPath,
		runner:        runner,
		containerName: containerName,
		image:         image,
		upstreamHost:  upstreamHost,
		publications:  make(map[string]controlplane.Publication),
	}
}

func (runtime *DockerCaddy) Bootstrap(ctx context.Context) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.home == "" {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(fmt.Errorf("home is required")))
	}
	if err := runtime.writeConfigLocked(runtime.publications); err != nil {
		return err
	}

	labels, err := runtime.inspectLabels(ctx)
	if err == nil {
		if labels["dev.zerobased.runtime"] != "caddy" || labels["dev.zerobased.owner"] != "zerobased" {
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(fmt.Errorf("container %s exists but is not owned by zerobased", runtime.containerName)))
		}
		if labels["dev.zerobased.home"] != runtime.home {
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(fmt.Errorf("container %s belongs to a different ZEROBASED_HOME (%s)", runtime.containerName, labels["dev.zerobased.home"])))
		}
		if _, err := runtime.runner.Run(ctx, "docker", "start", runtime.containerName); err != nil {
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(err))
		}
		return runtime.reloadLocked(ctx)
	}

	args := []string{
		"run", "-d",
		"--name", runtime.containerName,
		"--label", "dev.zerobased.runtime=caddy",
		"--label", "dev.zerobased.owner=zerobased",
		"--label", "dev.zerobased.home=" + runtime.home,
		"-v", runtime.configPath + ":/etc/caddy/Caddyfile:ro",
		"-p", "127.0.0.1:80:80",
		"--add-host", "host.docker.internal:host-gateway",
	}
	args = append(args, runtime.image, "caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
	if _, err := runtime.runner.Run(ctx, "docker", args...); err != nil {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(err))
	}
	return nil
}

func (runtime *DockerCaddy) inspectLabels(ctx context.Context) (map[string]string, error) {
	output, err := runtime.runner.Run(ctx, "docker", "inspect", "-f", "{{ json .Config.Labels }}", runtime.containerName)
	if err != nil {
		return nil, err
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &labels); err != nil {
		return nil, err
	}
	if labels == nil {
		labels = map[string]string{}
	}
	return labels, nil
}

func (runtime *DockerCaddy) Publish(ctx context.Context, publication controlplane.Publication) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	publication.Host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publication.Host)), ".")
	if err := validatePublication(publication); err != nil {
		return err
	}
	next := copyPublications(runtime.publications)
	next[publication.Host] = publication
	if err := runtime.writeConfigLocked(next); err != nil {
		return err
	}
	if err := runtime.reloadLocked(ctx); err != nil {
		_ = runtime.writeConfigLocked(runtime.publications)
		return err
	}
	runtime.publications = next
	return nil
}

func (runtime *DockerCaddy) Unpublish(ctx context.Context, publication controlplane.Publication) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publication.Host)), ".")
	next := copyPublications(runtime.publications)
	delete(next, host)
	if err := runtime.writeConfigLocked(next); err != nil {
		return err
	}
	if err := runtime.reloadLocked(ctx); err != nil {
		_ = runtime.writeConfigLocked(runtime.publications)
		return err
	}
	runtime.publications = next
	return nil
}

func (runtime *DockerCaddy) Close(ctx context.Context) error {
	if _, err := runtime.runner.Run(ctx, "docker", "rm", "-f", runtime.containerName); err != nil {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(err))
	}
	return nil
}

func (runtime *DockerCaddy) writeConfigLocked(publications map[string]controlplane.Publication) error {
	if err := os.MkdirAll(filepath.Dir(runtime.configPath), 0o755); err != nil {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeReloadFailed, zberr.WithCause(err))
	}
	content, err := renderCaddyfile(publications, runtime.upstreamHost)
	if err != nil {
		return err
	}
	if err := os.WriteFile(runtime.configPath, []byte(content), 0o644); err != nil {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeReloadFailed, zberr.WithCause(err))
	}
	return nil
}

func copyPublications(publications map[string]controlplane.Publication) map[string]controlplane.Publication {
	next := make(map[string]controlplane.Publication, len(publications))
	for host, publication := range publications {
		copied := publication
		copied.Routes = append([]controlplane.Route(nil), publication.Routes...)
		next[host] = copied
	}
	return next
}

func (runtime *DockerCaddy) reloadLocked(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		_, err := runtime.runner.Run(ctx, "docker", "exec", runtime.containerName, "caddy", "reload", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeReloadFailed, zberr.WithCause(ctx.Err()))
		case <-time.After(100 * time.Millisecond):
		}
	}
	return zberr.New(zberr.LayerRouteRuntime, zberr.CodeReloadFailed, zberr.WithCause(lastErr))
}

func renderCaddyfile(publications map[string]controlplane.Publication, upstreamHost string) (string, error) {
	var builder strings.Builder
	builder.WriteString("{\n\tauto_https off\n")
	builder.WriteString("}\n\n")
	builder.WriteString(":80 {\n\trespond \"zerobased: route not found\" 404\n}\n")

	hosts := make([]string, 0, len(publications))
	for host := range publications {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		publication := publications[host]
		if err := validatePublication(publication); err != nil {
			return "", err
		}
		routes := append([]controlplane.Route(nil), publication.Routes...)
		sort.SliceStable(routes, func(i, j int) bool {
			return len(routes[i].Path) > len(routes[j].Path)
		})

		builder.WriteString("\n")
		builder.WriteString("http://")
		builder.WriteString(host)
		builder.WriteString(" {\n")
		for _, route := range routes {
			if route.Path == "/" {
				builder.WriteString("\thandle {\n")
			} else {
				builder.WriteString("\thandle ")
				builder.WriteString(caddyPathMatcher(route.Path))
				builder.WriteString(" {\n")
			}
			builder.WriteString(fmt.Sprintf("\t\treverse_proxy %s:%d\n", upstreamHost, route.Port))
			builder.WriteString("\t}\n")
		}
		builder.WriteString("}\n")
	}
	return builder.String(), nil
}

func validatePublication(publication controlplane.Publication) error {
	if !validCaddyHost.MatchString(publication.Host) {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeApplyRejected, zberr.WithCause(fmt.Errorf("invalid route host %q", publication.Host)))
	}
	for _, route := range publication.Routes {
		if route.Port <= 0 || route.Port > 65535 {
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeApplyRejected, zberr.WithCause(fmt.Errorf("invalid route port %d", route.Port)))
		}
		if !strings.HasPrefix(route.Path, "/") || strings.ContainsAny(route.Path, " \t\r\n{}") {
			return zberr.New(zberr.LayerRouteRuntime, zberr.CodeApplyRejected, zberr.WithCause(fmt.Errorf("invalid route path %q", route.Path)))
		}
	}
	return nil
}

func caddyPathMatcher(path string) string {
	if strings.HasSuffix(path, "*") {
		return path
	}
	if strings.HasSuffix(path, "/") {
		return path + "*"
	}
	return path + "*"
}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		return output, err
	}
	return output, nil
}
