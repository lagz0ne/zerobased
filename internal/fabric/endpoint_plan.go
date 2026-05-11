package fabric

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultPortRangeStart = 42000
	defaultPortRangeEnd   = 49999
)

// PortRange is the dynamic candidate range for env-port endpoints.
type PortRange struct {
	Start int
	End   int
}

// EndpointIntent is a side-effect-free endpoint allocation plan.
type EndpointIntent struct {
	Name       string
	Mode       PortMode
	BindHost   string
	FixedPort  int
	Candidates []int
	Range      PortRange
}

// PlanEndpointIntents creates deterministic endpoint candidates without binding.
func PlanEndpointIntents(cfg *Config, identity IdentityFacts) (map[string]EndpointIntent, error) {
	portRange, err := parsePortRange(cfg.Identity.PortRange)
	if err != nil {
		return nil, err
	}
	intents := map[string]EndpointIntent{}
	for name, port := range cfg.Ports {
		intent := EndpointIntent{
			Name:     name,
			Mode:     port.Mode,
			BindHost: "127.0.0.1",
			Range:    portRange,
		}
		if port.Port != "" && port.Port != "auto" {
			fixed, err := strconv.Atoi(port.Port)
			if err != nil || fixed < 1 || fixed > 65535 {
				return nil, fmt.Errorf("ports.%s.port: invalid tcp port", name)
			}
			intent.FixedPort = fixed
			intent.Candidates = []int{fixed}
		} else if port.Mode != PortSocket {
			intent.Candidates = candidateSequence(identity, name, portRange)
		}
		intents[name] = intent
	}
	return intents, nil
}

func parsePortRange(value string) (PortRange, error) {
	if value == "" {
		return PortRange{Start: defaultPortRangeStart, End: defaultPortRangeEnd}, nil
	}
	startText, endText, ok := strings.Cut(value, "-")
	if !ok {
		return PortRange{}, fmt.Errorf("identity.port_range: must be start-end")
	}
	start, err := strconv.Atoi(strings.TrimSpace(startText))
	if err != nil {
		return PortRange{}, fmt.Errorf("identity.port_range: invalid start")
	}
	end, err := strconv.Atoi(strings.TrimSpace(endText))
	if err != nil {
		return PortRange{}, fmt.Errorf("identity.port_range: invalid end")
	}
	if start < 1 || end > 65535 || start > end {
		return PortRange{}, fmt.Errorf("identity.port_range: invalid range")
	}
	return PortRange{Start: start, End: end}, nil
}

func candidateSequence(identity IdentityFacts, endpoint string, portRange PortRange) []int {
	size := portRange.End - portRange.Start + 1
	offset := int(hashUint32(identity.Repo.Raw+"\x00"+identity.Scope.Raw+"\x00"+endpoint) % uint32(size))
	candidates := make([]int, size)
	for i := 0; i < size; i++ {
		candidates[i] = portRange.Start + ((offset + i) % size)
	}
	return candidates
}
