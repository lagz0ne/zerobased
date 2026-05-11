---
id: adr-20260511-service-level-reuse-contract
c3-seal: 2ba715ab6ea110e081d1de46017f8a7a838e616d3be4eafac454cb7a92b98da0
title: service-level-source-contract
type: adr
goal: Move existing-service acquisition semantics from port declarations to service behavior in the pure portless config contract.
status: implemented
date: "2026-05-11"
---

## Goal

Move existing-service acquisition semantics from port declarations to service behavior in the pure portless config contract.

## Context

Current config contract had `ports.*.reuse`, but user corrected the model: using an existing service is not a property of a named port. A service chooses whether to probe first, start first, and what to do on found/missing conditions. Ports only define endpoint constraints such as mode and fixed/auto port.

## Decision

Remove `PortConfig.Reuse`. Add service-level `source` config with probe target and found/missing actions. Keep shorthand `source: { tcp: db }` as probe-first data, with safe validation added by later ADRs. Validate source probe references a known port. Runtime behavior remains deferred; this layer is parse/validate contract only.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns CLI internal config contracts. | Keep pure; focused tests. |
| N.A - internal/fabric codemap gap | N.A - reason | C3 lookup lacks mapping. | Treat as cli-owned child surface. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Source probe references named ports. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no active rule entities listed | No C3 rule entities available. | N.A - no active rules |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| internal/fabric/config.go | Remove port reuse; add service source parse/validate. | go test ./internal/fabric |
| internal/fabric/config_test.go | Update tests to service-level source and invalid probe target. | go test ./internal/fabric |
| README.md | Align example and prose. | inspection plus tests |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add this ADR. | focused C3 check |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| Go tests | Port reuse removed; service source validated. | go test ./internal/fabric |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Keep reuse under ports | Existing-service acquisition changes process ownership and start behavior, not endpoint definition. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Source config becomes too complex | Preserve simple shorthand and add explicit actions only as data. | tests |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/fabric | Required before implemented |
| go test ./... | Required before implemented |
| c3 check --include-adr --only adr-20260511-service-level-reuse-contract | Required before implemented |
