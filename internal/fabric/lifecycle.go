package fabric

// LifecycleState names a pure service lifecycle state.
type LifecycleState string

const (
	StateWaitingForDeps LifecycleState = "waiting_for_deps"
	StateStarting       LifecycleState = "starting"
	StateProbing        LifecycleState = "probing"
	StateReady          LifecycleState = "ready"
	StateDegraded       LifecycleState = "degraded"
	StateFailed         LifecycleState = "failed"
	StateRestarting     LifecycleState = "restarting"
	StateStopping       LifecycleState = "stopping"
	StateReleased       LifecycleState = "released"
	StateOrphaned       LifecycleState = "orphaned"
	StatePruned         LifecycleState = "pruned"
)

var transitionTable = map[LifecycleState]map[LifecycleState]struct{}{
	StateWaitingForDeps: {
		StateStarting: {},
		StateFailed:   {},
		StateOrphaned: {},
		StateStopping: {},
	},
	StateStarting: {
		StateProbing:    {},
		StateFailed:     {},
		StateRestarting: {},
		StateStopping:   {},
	},
	StateProbing: {
		StateReady:      {},
		StateDegraded:   {},
		StateFailed:     {},
		StateRestarting: {},
		StateStopping:   {},
	},
	StateReady: {
		StateDegraded:   {},
		StateFailed:     {},
		StateRestarting: {},
		StateStopping:   {},
		StateReleased:   {},
		StateOrphaned:   {},
	},
	StateDegraded: {
		StateReady:      {},
		StateFailed:     {},
		StateRestarting: {},
		StateStopping:   {},
		StateReleased:   {},
		StateOrphaned:   {},
	},
	StateFailed: {
		StateRestarting: {},
		StateStopping:   {},
		StateReleased:   {},
		StateOrphaned:   {},
	},
	StateRestarting: {
		StateWaitingForDeps: {},
		StateStarting:       {},
		StateProbing:        {},
		StateFailed:         {},
		StateStopping:       {},
	},
	StateStopping: {
		StateReleased: {},
		StateFailed:   {},
		StateOrphaned: {},
	},
	StateReleased: {
		StatePruned: {},
	},
	StateOrphaned: {
		StateReleased: {},
		StatePruned:   {},
	},
	StatePruned: {},
}

// CanTransition reports whether the lifecycle state change is valid.
func CanTransition(from, to LifecycleState) bool {
	next, ok := transitionTable[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}
