package fabric

import "testing"

func TestValidLeaseOwner(t *testing.T) {
	tests := []struct {
		kind LeaseOwnerKind
		want bool
	}{
		{LeaseOwnerDaemonSession, true},
		{LeaseOwnerWrappedPID, true},
		{LeaseOwnerDocker, true},
		{LeaseOwnerCompose, true},
		{LeaseOwnerKind("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := ValidLeaseOwner(tt.kind); got != tt.want {
				t.Fatalf("ValidLeaseOwner(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestReviewLeaseCleanup(t *testing.T) {
	tests := []struct {
		name   string
		state  LifecycleState
		reason LeaseStopReason
		want   LeasePruneDecision
	}{
		{"clean stop releases", StateReady, LeaseStopClean, LeaseRelease},
		{"clean stop released residue prunes", StateReleased, LeaseStopClean, LeasePrune},
		{"clean stop orphan residue prunes", StateOrphaned, LeaseStopClean, LeasePrune},
		{"crash marks live lease orphan", StateReady, LeaseStopCrash, LeaseMarkOrphan},
		{"expired marks live lease orphan", StateDegraded, LeaseStopExpired, LeaseMarkOrphan},
		{"deleted source marks starting lease orphan", StateStarting, LeaseStopDeletedSource, LeaseMarkOrphan},
		{"orphan crash prunes", StateOrphaned, LeaseStopCrash, LeasePrune},
		{"released expired prunes", StateReleased, LeaseStopExpired, LeasePrune},
		{"gateway rebuild restores ready route", StateReady, LeaseStopGatewayRebuild, LeaseRebuildRoute},
		{"gateway rebuild restores degraded route", StateDegraded, LeaseStopGatewayRebuild, LeaseRebuildRoute},
		{"gateway rebuild leaves failed route alone", StateFailed, LeaseStopGatewayRebuild, LeaseKeep},
		{"gateway rebuild leaves orphan alone", StateOrphaned, LeaseStopGatewayRebuild, LeaseKeep},
		{"unknown reason keeps lease", StateReady, LeaseStopReason("unknown"), LeaseKeep},
		{"pruned lease stays pruned", StatePruned, LeaseStopCrash, LeaseKeep},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReviewLeaseCleanup(tt.state, tt.reason); got != tt.want {
				t.Fatalf("ReviewLeaseCleanup(%q, %q) = %q, want %q", tt.state, tt.reason, got, tt.want)
			}
		})
	}
}
