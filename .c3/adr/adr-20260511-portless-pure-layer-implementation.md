---
id: adr-20260511-portless-pure-layer-implementation
c3-seal: 90183ddba2ab1052d88eb2e7417749efd4dcbe93345024c74aa9b68b54a9edd8
title: portless-pure-layer-implementation
type: adr
goal: Implement the reviewed pure portless fabric layers that define config safety, identity/host planning, and endpoint candidate planning before any runtime process, lease allocator, or proxy side effects are introduced.
status: implemented
date: "2026-05-11"
---

## Goal

Implement the reviewed pure portless fabric layers that define config safety, identity/host planning, and endpoint candidate planning before any runtime process, lease allocator, or proxy side effects are introduced.

## Context

The portless fabric plan was reviewed through multiple Codex/Claude rounds. The remaining approved implementation slice is pure: config contracts, route visibility semantics, source safety schema, endpoint capability declarations, host graph generation, and deterministic endpoint candidate planning. Existing legacy run/docker/daemon paths remain separate and must not constrain or be modified by this slice.

## Decision

Add pure fabric contracts and tests only. Route exposure uses canonical `visibility: internal|public`; legacy `internal: true` is shorthand only when `visibility` is absent and conflicting declarations fail. Service source `found: use` requires identity-capable proof unless the explicit unidentified escape hatch is set. Endpoint capability and deterministic candidate ports are data/planning contracts only. Runtime lease allocation, process starting, Caddy mutation, and CLI up/down remain out of scope.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns CLI and internal fabric contracts. | Keep changes pure and verified by tests. |
| N.A - internal/fabric codemap gap | N.A - reason | C3 lookup does not map new fabric files yet. | Treat as CLI-owned child surface. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Host and route names must remain URL-safe and deterministic. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no active rule entities listed | No C3 rule entities available. | N.A - no active rules |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| internal/fabric/config.go | Add visibility, endpoint capability, and safe source validation. | go test ./internal/fabric |
| internal/fabric/namespace.go | Add host graph planning using repo/scope/domain defaults and overrides. | go test ./internal/fabric |
| internal/fabric/endpoint_plan.go | Add deterministic endpoint intent/candidate planning. | go test ./internal/fabric |
| internal/fabric/*_test.go | Cover contract conflicts, source safety, host uniqueness, and candidate determinism. | go test ./internal/fabric |
| README.md | Align user manual with implemented config shape. | inspection |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add this ADR for the pure implementation slice. | focused C3 check |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| Go tests | Config, identity, and endpoint planner contracts reject unsafe states before runtime. | go test ./internal/fabric |
| Full suite | Existing legacy behavior still compiles and tests pass. | go test ./... |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Implement runtime first | Runtime would freeze unclear contracts and leak errors across layers. |
| Keep internal boolean as authoritative | Conflicts with visibility create security-adjacent ambiguity. |
| Allow TCP source use by default | Bare TCP proves reachability, not service identity. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Pure contracts drift from runtime later | Keep runtime out of scope and create fixtures/data structures for later layers. | focused tests |
| Config shape becomes too long | Preserve shorthand forms and only require detail for unsafe reuse cases. | README example review |
| Legacy paths regress | Avoid touching runtime packages and run full suite. | go test ./... |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/fabric | Required before implemented |
| go test ./... | Required before implemented |
| c3 check --include-adr --only <this-adr> | Required before implemented |
