---
id: adr-20260508-portless-service-fabric
c3-seal: 14ab2919c60c356f984135b97bbc7167a1099abfdf3bd636d02f4c1519035914
title: portless-service-fabric
type: adr
goal: 'Introduce a portless service fabric direction for zerobased: users and agents name services, while zerobased owns private conflict-free endpoints, readiness state, stable local URLs, and lease-backed cleanup. This ADR authorizes the first implementation layer: docs plus pure contract models for namespace, lifecycle, lease, and endpoint policy before CLI/daemon wiring.'
status: implemented
date: "2026-05-08"
---

## Goal

Introduce a portless service fabric direction for zerobased: users and agents name services, while zerobased owns private conflict-free endpoints, readiness state, stable local URLs, and lease-backed cleanup. This ADR authorizes the first implementation layer: docs plus pure contract models for namespace, lifecycle, lease, and endpoint policy before CLI/daemon wiring.

## Context

Current zerobased is a Go CLI and daemon that watches Docker events, classifies container ports, creates Unix socket bridges or Caddy HTTP routes, and cleans them when containers stop. Current run-wrapper can wrap one dev server but still relies on explicit `-p` or stdout port detection. The product pressure is to remove normal user-facing port management entirely, including parallel worktree/agent collisions and stale process residue. Affected topology is the cli container and future components under it. C3 lookup shows `internal/fabric/**` and `docs/adr/**` are uncharted, so this ADR explicitly treats them as new parent-owned cli surfaces.

## Decision

Add an impact-first ADR/documentation artifact and a pure internal service-fabric contract package. Keep this layer free of daemon, Docker, Caddy, or process IO so tests can validate semantics before runtime wiring. The chosen approach is lower risk than wiring directly into run-wrapper because it makes the no-port promise, lifecycle states, and endpoint policy falsifiable before touching side-effectful code.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns Go binary, daemon, run-wrapper, env, caddy, Docker orchestration, and new service-fabric contracts. | Review container boundary and naming ref before adding new internal package/docs. |
| N.A - ref-naming-convention is a reference, not topology | N.A - reference | Naming stability and collision avoidance govern service names and URLs. | Comply with deterministic collision-free naming intent; future ref update may be needed after contracts settle. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Service identity and generated URLs must be deterministic, URL-safe, and collision-aware. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no C3 rule entities listed | c3 list shows refs/components but no rule entities. | N.A - no active C3 rules to comply with |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| docs/adr/0001-portless-service-fabric.md | Human-facing ADR with impact promise, lifecycle, lease owners, endpoint policy, trust boundaries, MVP, and verification matrix. | File exists and contains ADR-level verification matrix. |
| internal/fabric | Pure Go contracts for namespace identity, lifecycle transitions, lease state, and endpoint policy. | go test ./internal/fabric passes. |
| coordinator tasks | Keep ADR as feature-level verification root; leaf tasks carry local proof only. | Coordination note already records graph invariant. |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add this ADR as proposed work order. | c3 read adr-portless-service-fabric --full after creation. |
| C3 check | Existing c3 check reports no documents found in .c3; this ADR does not change C3 validators. | Record blocker in final evidence; rerun c3 check after mutation. |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| Go tests | Validate namespace collision behavior, lifecycle transition rejection, endpoint policy normal/legacy classification. | go test ./internal/fabric |
| ADR verification matrix | Defines feature-level proof scenarios such as two worktrees, host+Docker routing, orphan prune, gateway restart, dependency readiness. | docs ADR section |
| Coordinator graph | Prevents leaf tasks from claiming full feature verification. | ADR task note and linked tasks |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Wire directly into run-wrapper first | Current run-wrapper has IO/process/Caddy side effects; changing it before pure contracts makes failures harder to attribute. |
| Keep deterministic port hashes as primary solution | Hashes can collide and still expose numeric port semantics; they do not satisfy no-port normal workflow. |
| Adopt Portless model wholesale | Portless is HTTP-dev-server centered; zerobased must also cover Docker, Unix sockets, and non-HTTP connection strings. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| New contracts drift from current Docker behavior | Keep package pure and separately map Docker later through dedicated task. | Docker task remains blocked on namespace/lease/health contracts. |
| Users still see numeric ports as primary UX | Endpoint policy marks fixed ports legacy and names/URLs primary. | Fabric tests assert fixed legacy is not normal path. |
| C3 docs coverage gap for new paths | ADR records new surfaces as cli-owned until codemap update. | c3 lookup internal/fabric/** and c3 lookup docs/adr/** outputs recorded. |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/fabric | Required before marking implemented. |
| go test ./... | Required regression before marking implemented. |
| c3 check --include-adr | Required; current pre-existing check issue must be reported if still present. |
| ADR matrix review | Required named artifact: docs/adr/0001-portless-service-fabric.md Verification Matrix. |
