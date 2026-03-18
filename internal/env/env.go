package env

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/ports"
)

// ServiceEndpoint describes how to connect to one port of a service.
type ServiceEndpoint struct {
	Project       string
	Service       string
	ContainerPort uint16
	Method        classifier.ExposeMethod
	ConnString    string // the actual connection string/path
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
		sockDir := filepath.Join(baseDir, project)
		filename := classifier.SocketFilename(service, containerPort)
		sockPath := filepath.Join(sockDir, filename)

		switch containerPort {
		case 5432:
			// Postgres uses directory, not file path in connection string
			ep.ConnString = fmt.Sprintf("postgresql://postgres@/%s?host=%s", "postgres", sockDir)
		case 3306:
			ep.ConnString = fmt.Sprintf("mysql://root@unix(%s)/", sockPath)
		case 6379:
			ep.ConnString = fmt.Sprintf("redis+unix://%s", sockPath)
		case 27017:
			ep.ConnString = fmt.Sprintf("mongodb://localhost/%s?socketPath=%s", "admin", sockPath)
		default:
			ep.ConnString = fmt.Sprintf("unix://%s", sockPath)
		}

	case classifier.HTTP:
		hostname := Hostname(project, service, containerPort)
		ep.ConnString = fmt.Sprintf("http://%s", hostname)

	case classifier.Port:
		p := ports.DeterministicPort(project, service, containerPort)
		ep.ConnString = fmt.Sprintf("localhost:%d", p)
	}

	return ep
}

// TemplateVars returns all discoverable variables for an endpoint.
func TemplateVars(baseDir string, ep ServiceEndpoint) map[string]string {
	vars := map[string]string{
		"project":        ep.Project,
		"service":        ep.Service,
		"container_port": strconv.FormatUint(uint64(ep.ContainerPort), 10),
		"method":         string(ep.Method),
		"conn":           ep.ConnString,
	}

	switch ep.Method {
	case classifier.Socket:
		sockDir := filepath.Join(baseDir, ep.Project)
		filename := classifier.SocketFilename(ep.Service, ep.ContainerPort)
		vars["socket"] = filepath.Join(sockDir, filename)
		vars["socket_dir"] = sockDir
	case classifier.HTTP:
		hostname := Hostname(ep.Project, ep.Service, ep.ContainerPort)
		vars["host"] = hostname
		vars["url"] = "http://" + hostname
		vars["port"] = strconv.FormatUint(uint64(ep.ContainerPort), 10)
	case classifier.Port:
		p := ports.DeterministicPort(ep.Project, ep.Service, ep.ContainerPort)
		vars["host"] = "localhost"
		vars["port"] = strconv.FormatUint(uint64(p), 10)
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
