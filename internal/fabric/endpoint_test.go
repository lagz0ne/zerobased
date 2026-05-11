package fabric

import "testing"

func TestEndpointPolicies(t *testing.T) {
	tests := []struct {
		policy EndpointPolicy
		class  EndpointPolicyClass
	}{
		{EndpointUnixSocket, EndpointClassNormal},
		{EndpointListenerHandoff, EndpointClassNormal},
		{EndpointEphemeralLoopback, EndpointClassNormal},
		{EndpointEnvPort, EndpointClassNormal},
		{EndpointEnvFlag, EndpointClassNormal},
	}

	policies := EndpointPolicies()
	seen := map[EndpointPolicy]EndpointPolicyModel{}
	for _, policy := range policies {
		seen[policy.Policy] = policy
	}

	for _, tt := range tests {
		t.Run(string(tt.policy), func(t *testing.T) {
			got, ok := seen[tt.policy]
			if !ok {
				t.Fatalf("missing endpoint policy %q", tt.policy)
			}
			if got.Class != tt.class {
				t.Fatalf("Class = %q, want %q", got.Class, tt.class)
			}
			if got.Policy.Class() != tt.class {
				t.Fatalf("Class() = %q, want %q", got.Policy.Class(), tt.class)
			}
			if got.Policy.IsNormalPath() != (tt.class == EndpointClassNormal) {
				t.Fatalf("IsNormalPath() = %v, want class %q", got.Policy.IsNormalPath(), tt.class)
			}
		})
	}
}

func TestEndpointPolicyUnknownIsUnsupported(t *testing.T) {
	unknown := EndpointPolicy("guessed_after_start")
	if unknown.IsNormalPath() {
		t.Fatal("unknown endpoint policy must not be normal allocation")
	}
	if unknown.IsPortless() {
		t.Fatal("unknown endpoint policy must not be portless")
	}
	if unknown.Class() != EndpointClassUnsupported {
		t.Fatalf("unknown class = %q, want %q", unknown.Class(), EndpointClassUnsupported)
	}
}
