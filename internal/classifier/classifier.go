package classifier

// ExposeMethod determines how a container port is exposed to the host.
type ExposeMethod string

const (
	Socket ExposeMethod = "socket"
	HTTP   ExposeMethod = "http"
	Port   ExposeMethod = "port"
)

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

	if wellKnownSockets[containerPort] {
		return Socket
	}
	if wellKnownHTTP[containerPort] {
		return HTTP
	}
	return Port
}

// SocketFilename returns the conventional socket filename for a service/port.
// For Postgres, uses the .s.PGSQL.5432 convention.
func SocketFilename(service string, containerPort uint16) string {
	if containerPort == 5432 {
		return ".s.PGSQL.5432"
	}
	return service + "-" + uitoa(containerPort) + ".sock"
}

func uitoa(n uint16) string {
	if n == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
