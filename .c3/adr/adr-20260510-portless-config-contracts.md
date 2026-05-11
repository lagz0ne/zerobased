---
id: adr-20260510-portless-config-contracts
c3-seal: 11e24b3ab8653102d139654fd2b4489cd7db40f7d450d6a6a801e929d30861ef
title: portless-config-contracts
type: adr
goal: 'Implement the first pure config contract for zerobased portless connection management: parse, validate, and render the compact `ports`/`services`/`wiring`/`routes` model without starting processes, binding ports, or touching Caddy.'
status: implemented
date: "2026-05-10"
---

## Goal

Implement the first pure config contract for zerobased portless connection management: parse, validate, and render the compact `ports`/`services`/`wiring`/`routes` model without starting processes, binding ports, or touching Caddy.

## Context

The existing repo has pure `internal/fabric` contracts for namespace, endpoint policy, lifecycle, and lease cleanup, plus existing routefile parsing under `internal/routes`. The new user direction is to use `ports` as named local connection points, `services` as opaque commands, `wiring` for environment-specific values and required CLI inputs, and `routes` as final app surface. This layer must remain zero-IO except YAML parsing from bytes, so runtime orchestration can be built later on stable semantics.

## Decision

Add a pure `internal/fabric/config` contract package or equivalent internal fabric config files that parse YAML bytes into typed config, validate references, merge wiring profile plus inline overrides, and render service env templates from provided port facts/run metadata. This wins over wiring directly into CLI because config errors must fail before any process starts.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns CLI internals and new config parsing contract. | Keep change pure and do not mutate daemon/run runtime yet. |
| N.A - internal/fabric is uncharted in C3 codemap | N.A - reason | C3 lookup reports codemap coverage gap for internal/fabric paths. | Treat as cli-owned child surface and record focused tests. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Config references named ports/services/routes; names must stay deterministic and collision-safe. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no active rule entities listed | c3 list has no rule entities. | N.A - no active C3 rules to apply |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| internal/fabric config | Add pure parse/validate/render contract for compact config. | focused Go tests pass |
| README/ADR alignment | No doc update required unless contract names drift from README concept. | scan/inspection |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add proposed work order. | focused C3 check |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| Go tests | invalid config, missing required wiring, bad template refs, route target validation, env render. | go test ./internal/fabric/... |
| Full Go suite | Ensure no existing router behavior breaks. | go test ./... |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Implement CLI up directly | Would mix parser errors with process/proxy side effects and violate fail-before-start goal. |
| Extend old routefile parser only | Old routefile only maps paths to services; it cannot express env wiring or service commands. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Config grows Kubernetes-like | Keep contract to ports/services/wiring/routes and tests for shorthand forms. | review examples in tests |
| Templates become programming language | Only support simple {{key.path}} replacement and fail unknown refs. | template tests |
| Staging wiring accidentally starts local dependency | Model disabled services in wiring validation. | test disabled service behavior |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/fabric/... | Required before implemented |
| go test ./... | Required before implemented |
| c3 check --include-adr --only adr-20260510-portless-config-contracts | Required before implemented |
