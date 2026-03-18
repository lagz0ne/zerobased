package classifier

import (
	"fmt"
	"strconv"
)

// ExposeMethod determines how a container port is exposed to the host.
type ExposeMethod string

const (
	Socket   ExposeMethod = "socket"
	HTTP     ExposeMethod = "http"
	Port     ExposeMethod = "port"
	Internal ExposeMethod = "internal" // never exposed — cluster/inter-node ports
)

// wellKnownInternal maps ports that are internal-only and should never be exposed.
var wellKnownInternal = map[uint16]bool{
	6222:  true, // NATS cluster/routing
	9222:  true, // NATS leafnode
	7946:  true, // Docker Swarm gossip
	2377:  true, // Docker Swarm management
	2380:  true, // etcd peer
}

// wellKnownSockets maps container ports to socket-based exposure.
var wellKnownSockets = map[uint16]bool{
	5432:  true, // Postgres
	3306:  true, // MySQL
	6379:  true, // Redis
	27017: true, // MongoDB
	4317:  true, // OTEL gRPC
}

// wellKnownHTTP maps container ports to HTTP/WS-based exposure.
var wellKnownHTTP = map[uint16]bool{
	80:   true,
	443:  true,
	3000: true,
	3001: true,
	4318: true, // OTEL HTTP
	5173: true, // Vite
	8000: true,
	8025: true, // Mailhog
	8080: true,
	8222: true, // NATS monitoring
	8443: true,
	8888: true,
	9090: true,
}

// Classify returns the expose method for a given container port.
// If a Docker label override is provided, it takes precedence.
func Classify(containerPort uint16, labelOverride string) ExposeMethod {
	if labelOverride != "" {
		switch labelOverride {
		case "socket":
			return Socket
		case "http":
			return HTTP
		case "port":
			return Port
		}
	}

	if wellKnownInternal[containerPort] {
		return Internal
	}
	if wellKnownSockets[containerPort] {
		return Socket
	}
	if wellKnownHTTP[containerPort] {
		return HTTP
	}
	return Port
}

// ClassifyFromLabels classifies a port using Docker labels for override lookup.
func ClassifyFromLabels(containerPort uint16, labels map[string]string) ExposeMethod {
	return Classify(containerPort, labels[fmt.Sprintf("zerobased.%d", containerPort)])
}

// SocketFilename returns the conventional socket filename for a service/port.
// For Postgres, uses the .s.PGSQL.5432 convention.
func SocketFilename(service string, containerPort uint16) string {
	if containerPort == 5432 {
		return ".s.PGSQL.5432"
	}
	return service + "-" + strconv.FormatUint(uint64(containerPort), 10) + ".sock"
}
