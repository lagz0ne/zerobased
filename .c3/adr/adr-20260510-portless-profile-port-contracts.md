---
id: adr-20260510-portless-profile-port-contracts
c3-seal: 21c8aca9fb35de7a7aeb4c10e33f7fc696666a3ab07570b6a96904dd282cdb4b
title: portless-profile-port-contracts
type: adr
goal: Refine the pure portless config contract by replacing `wiring` with `profiles`, adding fixed port/reuse constraints under `ports`, and validating disabled profile effects before runtime.
status: implemented
date: "2026-05-10"
---

## Goal

Refine the pure portless config contract by replacing `wiring` with `profiles`, adding fixed port/reuse constraints under `ports`, and validating disabled profile effects before runtime.

## Context

The first config contract parses compact `ports/services/wiring/routes` and renders service env. User feedback refined the language: `profiles` is clearer than `wiring`, fixed ports are explicit constraints under `ports`, and `reuse` must be safe and validated before starting commands.

## Decision

Update the pure config layer only. Keep no runtime process/proxy side effects. `profiles` replaces `wiring`; `ports` accepts scalar shorthand or object form with `mode`, `port`, and `reuse`; render input selects a profile and applies CLI-style overrides. Render validation rejects disabled services used by active required dependencies or routes.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns CLI internal config contracts. | Keep change pure and covered by focused tests. |
| N.A - internal/fabric codemap gap | N.A - reason | C3 lookup has no component mapping for internal/fabric. | Treat as cli-owned child surface. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Config names ports/services/routes/profiles. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no active rule entities listed | No C3 rule entities available. | N.A - no active rules |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| internal/fabric/config.go | Rename wiring to profiles; add port object fields; render validation for disabled services. | go test ./internal/fabric |
| internal/fabric/config_test.go | Update and extend contract tests. | go test ./internal/fabric |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add this ADR. | focused C3 check |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| Go tests | profile required inputs, disabled service validation, fixed port/reuse validation. | go test ./internal/fabric |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Keep wiring alias | Keeps two names for one concept and confuses CLI surface. |
| Put fixed port under service | Port allocation belongs to named port facts, not command behavior. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Breaking earlier internal tests | Update tests to current contract; no runtime callers yet. | go test ./... |
| Reuse accepts wrong listener later | This layer validates declaration only; runtime probe validation deferred. | tests ensure reuse requires fixed port |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/fabric | Required before implemented |
| go test ./... | Required before implemented |
| c3 check --include-adr --only adr-20260510-portless-profile-port-contracts | Required before implemented |
