package env

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/ports"
)

// ServiceEndpoint describes how to connect to one port of a service.
type ServiceEndpoint struct {
	Project       string
	Service       string
	ContainerPort uint16
	Method        classifier.ExposeMethod
	ConnString    string // the actual connection string/path

	// Intermediate values (populated by ForPort, used by TemplateVars)
	Socket    string // full socket path (socket types only)
	SocketDir string // socket directory (socket types only)
	Host      string // hostname or "localhost"
	HostPort  uint16 // resolved host port (deterministic hash or container port)
}

// ForPort generates the connection string for a single port exposure.
func ForPort(baseDir, project, service string, containerPort uint16, method classifier.ExposeMethod) ServiceEndpoint {
	ep := ServiceEndpoint{
		Project:       project,
		Service:       service,
		ContainerPort: containerPort,
		Method:        method,
	}

	switch method {
	case classifier.Socket:
		ep.SocketDir = filepath.Join(baseDir, project)
		filename := classifier.SocketFilename(service, containerPort)
		ep.Socket = filepath.Join(ep.SocketDir, filename)

		switch containerPort {
		case 5432:
			ep.ConnString = fmt.Sprintf("postgresql://postgres@/%s?host=%s", "postgres", ep.SocketDir)
		case 3306:
			ep.ConnString = fmt.Sprintf("mysql://root@unix(%s)/", ep.Socket)
		case 6379:
			ep.ConnString = fmt.Sprintf("redis+unix://%s", ep.Socket)
		case 27017:
			ep.ConnString = fmt.Sprintf("mongodb://localhost/%s?socketPath=%s", "admin", ep.Socket)
		default:
			ep.ConnString = fmt.Sprintf("unix://%s", ep.Socket)
		}

	case classifier.HTTP:
		ep.Host = Hostname(project, service, containerPort)
		ep.HostPort = containerPort
		ep.ConnString = fmt.Sprintf("http://%s", ep.Host)

	case classifier.Port:
		ep.HostPort = ports.DeterministicPort(project, service, containerPort)
		ep.Host = "localhost"
		ep.ConnString = fmt.Sprintf("localhost:%d", ep.HostPort)
	}

	return ep
}

// EndpointsFromContainers resolves all endpoints from a list of containers.
// If project is non-empty, only that project's containers are included.
func EndpointsFromContainers(baseDir string, containers []*docker.ContainerInfo, project string) []ServiceEndpoint {
	var endpoints []ServiceEndpoint
	for _, c := range containers {
		if project != "" && c.Project != project {
			continue
		}
		for _, pb := range c.Ports {
			if pb.Proto != "tcp" {
				continue
			}
			method := classifier.ClassifyFromLabels(pb.ContainerPort, c.Labels)
			if method == classifier.Internal {
				continue
			}
			endpoints = append(endpoints, ForPort(baseDir, c.Project, c.Service, pb.ContainerPort, method))
		}
	}
	return endpoints
}

// TemplateVars returns all discoverable variables for an endpoint.
// Reads from pre-computed fields — no re-derivation.
func TemplateVars(ep ServiceEndpoint) map[string]string {
	vars := map[string]string{
		"project":        ep.Project,
		"service":        ep.Service,
		"container_port": strconv.FormatUint(uint64(ep.ContainerPort), 10),
		"method":         string(ep.Method),
		"conn":           ep.ConnString,
	}

	if ep.Socket != "" {
		vars["socket"] = ep.Socket
		vars["socket_dir"] = ep.SocketDir
	}
	if ep.Host != "" {
		vars["host"] = ep.Host
	}
	if ep.HostPort > 0 {
		vars["port"] = strconv.FormatUint(uint64(ep.HostPort), 10)
	}
	if ep.ConnString != "" {
		vars["url"] = ep.ConnString
	}

	return vars
}

// RenderTemplate replaces {{key}} placeholders with values from vars.
func RenderTemplate(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// Hostname returns the HTTP hostname for a service port.
func Hostname(project, service string, containerPort uint16) string {
	return fmt.Sprintf("%s-%d.%s.localhost", service, containerPort, project)
}

// EnvKey returns the environment variable name for an endpoint.
// prefix "ZB" → ZB_POSTGRES_5432, prefix "" → POSTGRES_5432
func EnvKey(prefix string, ep ServiceEndpoint) string {
	name := fmt.Sprintf("%s_%d", strings.ToUpper(strings.ReplaceAll(ep.Service, "-", "_")), ep.ContainerPort)
	if prefix == "" {
		return name
	}
	return prefix + "_" + name
}

// AsEnvVars returns endpoints as KEY=VALUE pairs suitable for os.Environ / exec.Cmd.Env.
func AsEnvVars(prefix string, endpoints []ServiceEndpoint) []string {
	vars := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		vars = append(vars, EnvKey(prefix, ep)+"="+ep.ConnString)
	}
	return vars
}

// PrintEndpoints formats a slice of endpoints for display.
func PrintEndpoints(endpoints []ServiceEndpoint) string {
	var b strings.Builder
	for _, ep := range endpoints {
		fmt.Fprintf(&b, "  %-12s %-8s %s/%d → %s\n",
			ep.Project, ep.Service, ep.Method, ep.ContainerPort, ep.ConnString)
	}
	return b.String()
}

// PrintExport formats endpoints as shell export statements.
func PrintExport(prefix string, endpoints []ServiceEndpoint) string {
	var b strings.Builder
	for _, ep := range endpoints {
		fmt.Fprintf(&b, "export %s=%q\n", EnvKey(prefix, ep), ep.ConnString)
	}
	return b.String()
}
