package fabric

// LeaseOwnerKind names the authority that owns a private endpoint lease.
type LeaseOwnerKind string

const (
	LeaseOwnerDaemonSession LeaseOwnerKind = "daemon_session"
	LeaseOwnerWrappedPID    LeaseOwnerKind = "wrapped_pid"
	LeaseOwnerDocker        LeaseOwnerKind = "docker_container"
	LeaseOwnerCompose       LeaseOwnerKind = "compose_service"
)

// LeaseStopReason explains why a lease is being evaluated for cleanup.
type LeaseStopReason string

const (
	LeaseStopClean          LeaseStopReason = "clean_stop"
	LeaseStopCrash          LeaseStopReason = "crash"
	LeaseStopExpired        LeaseStopReason = "expired"
	LeaseStopDeletedSource  LeaseStopReason = "deleted_source"
	LeaseStopGatewayRebuild LeaseStopReason = "gateway_rebuild"
)

// LeasePruneDecision is the reviewable cleanup decision for a lease.
type LeasePruneDecision string

const (
	LeaseKeep         LeasePruneDecision = "keep"
	LeaseRelease      LeasePruneDecision = "release"
	LeaseMarkOrphan   LeasePruneDecision = "mark_orphan"
	LeasePrune        LeasePruneDecision = "prune"
	LeaseRebuildRoute LeasePruneDecision = "rebuild_route"
)

// LeaseRecord is the pure contract for tracking a private endpoint owner.
type LeaseRecord struct {
	ID        string
	Namespace ServiceNamespace
	OwnerKind LeaseOwnerKind
	OwnerID   string
	Endpoint  EndpointPolicy
	State     LifecycleState
}

// ValidLeaseOwner reports whether kind can own a lease.
func ValidLeaseOwner(kind LeaseOwnerKind) bool {
	switch kind {
	case LeaseOwnerDaemonSession, LeaseOwnerWrappedPID, LeaseOwnerDocker, LeaseOwnerCompose:
		return true
	default:
		return false
	}
}

// ReviewLeaseCleanup returns the non-destructive cleanup decision a dry run should show.
func ReviewLeaseCleanup(state LifecycleState, reason LeaseStopReason) LeasePruneDecision {
	if state == StatePruned {
		return LeaseKeep
	}

	switch reason {
	case LeaseStopClean:
		if state == StateReleased || state == StateOrphaned {
			return LeasePrune
		}
		return LeaseRelease
	case LeaseStopCrash, LeaseStopExpired, LeaseStopDeletedSource:
		if state == StateOrphaned || state == StateReleased {
			return LeasePrune
		}
		return LeaseMarkOrphan
	case LeaseStopGatewayRebuild:
		if state == StateReady || state == StateDegraded {
			return LeaseRebuildRoute
		}
		return LeaseKeep
	default:
		return LeaseKeep
	}
}
