package env

import (
	"fmt"
	"path/filepath"
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

// Hostname returns the HTTP hostname for a service port.
func Hostname(project, service string, containerPort uint16) string {
	return fmt.Sprintf("%s-%d.%s.localhost", service, containerPort, project)
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
