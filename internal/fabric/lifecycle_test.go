package fabric

import "testing"

func TestLifecycleConstants(t *testing.T) {
	tests := []struct {
		state LifecycleState
		want  string
	}{
		{StateWaitingForDeps, "waiting_for_deps"},
		{StateStarting, "starting"},
		{StateProbing, "probing"},
		{StateReady, "ready"},
		{StateDegraded, "degraded"},
		{StateFailed, "failed"},
		{StateRestarting, "restarting"},
		{StateStopping, "stopping"},
		{StateReleased, "released"},
		{StateOrphaned, "orphaned"},
		{StatePruned, "pruned"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Fatalf("state = %q, want %q", tt.state, tt.want)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from LifecycleState
		to   LifecycleState
		want bool
	}{
		{"wait to start", StateWaitingForDeps, StateStarting, true},
		{"start to probe", StateStarting, StateProbing, true},
		{"probe to ready", StateProbing, StateReady, true},
		{"ready to degraded", StateReady, StateDegraded, true},
		{"degraded recovers", StateDegraded, StateReady, true},
		{"restart can probe directly", StateRestarting, StateProbing, true},
		{"waiting deleted source can orphan", StateWaitingForDeps, StateOrphaned, true},
		{"failed can orphan", StateFailed, StateOrphaned, true},
		{"stopping can orphan", StateStopping, StateOrphaned, true},
		{"orphan can release", StateOrphaned, StateReleased, true},
		{"orphan can prune", StateOrphaned, StatePruned, true},
		{"ready to released", StateReady, StateReleased, true},
		{"released to pruned", StateReleased, StatePruned, true},
		{"false ready from waiting rejected", StateWaitingForDeps, StateReady, false},
		{"false ready from starting rejected", StateStarting, StateReady, false},
		{"ready cannot go back to probing", StateReady, StateProbing, false},
		{"pruned terminal", StatePruned, StateReady, false},
		{"unknown source rejected", LifecycleState("unknown"), StateReady, false},
		{"unknown target rejected", StateReady, LifecycleState("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
