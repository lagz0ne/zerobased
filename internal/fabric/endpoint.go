package fabric

// EndpointPolicy describes how a service endpoint is discovered or provided.
type EndpointPolicy string

const (
	EndpointUnixSocket        EndpointPolicy = "unix_socket"
	EndpointListenerHandoff   EndpointPolicy = "listener_handoff"
	EndpointEphemeralLoopback EndpointPolicy = "ephemeral_loopback"
	EndpointEnvPort           EndpointPolicy = "env_port"
	EndpointEnvFlag           EndpointPolicy = "env_flag"
)

// EndpointPolicyClass captures where a policy sits in the allocation order.
type EndpointPolicyClass string

const (
	EndpointClassNormal      EndpointPolicyClass = "normal"
	EndpointClassUnsupported EndpointPolicyClass = "unsupported"
)

// EndpointPolicyModel classifies endpoint policies for portless fabric routing.
type EndpointPolicyModel struct {
	Policy EndpointPolicy
	Class  EndpointPolicyClass
}

var endpointPolicies = []EndpointPolicyModel{
	{Policy: EndpointUnixSocket, Class: EndpointClassNormal},
	{Policy: EndpointListenerHandoff, Class: EndpointClassNormal},
	{Policy: EndpointEphemeralLoopback, Class: EndpointClassNormal},
	{Policy: EndpointEnvPort, Class: EndpointClassNormal},
	{Policy: EndpointEnvFlag, Class: EndpointClassNormal},
}

// EndpointPolicies returns the known policy classifications.
func EndpointPolicies() []EndpointPolicyModel {
	out := make([]EndpointPolicyModel, len(endpointPolicies))
	copy(out, endpointPolicies)
	return out
}

// IsNormalPath reports whether policy is part of normal portless allocation.
func (p EndpointPolicy) IsNormalPath() bool {
	return p.Class() == EndpointClassNormal
}

// Class returns the allocation class for policy.
func (p EndpointPolicy) Class() EndpointPolicyClass {
	for _, known := range endpointPolicies {
		if known.Policy == p {
			return known.Class
		}
	}
	return EndpointClassUnsupported
}

// IsPortless reports whether policy avoids user-selected numeric ports.
func (p EndpointPolicy) IsPortless() bool {
	return p.Class() == EndpointClassNormal
}
