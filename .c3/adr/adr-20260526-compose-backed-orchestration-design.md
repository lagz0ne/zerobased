---
id: adr-20260526-compose-backed-orchestration-design
c3-seal: 1d206db5fc5f5ab6e4df9a3d7098f6bad745739199c6b1f5c5857e30b6632a60
title: compose-backed-orchestration-design
type: adr
goal: 'Design Compose-backed orchestration for zerobased up: load explicit zerobased.yaml v1 compose declarations through compose-go, validate and inject zerobased runtime identity and routing policy, then drive Docker Compose through the Go API while preserving daemon-owned claims, route publication, readiness, and foreground cleanup.'
status: proposed
date: "2026-05-26"
---

## Goal

Design Compose-backed orchestration for zerobased up: load explicit zerobased.yaml v1 compose declarations through compose-go, validate and inject zerobased runtime identity and routing policy, then drive Docker Compose through the Go API while preserving daemon-owned claims, route publication, readiness, and foreground cleanup.

## Context

Zerobased is in a rewrite from legacy Docker autodiscovery to explicit zerobased.yaml v1 control-plane orchestration. Current C3 topology still describes legacy autodiscovery components, while the checked-in rewrite files now center on start, up, daemon IPC, control-plane claims, route runtime, typed errors, and system tests. The next product slice is orchestration for project dependencies such as databases, NATS, and OTEL collector without returning to branch-agnostic docker compose up behavior. The design must use Docker Compose as a backend model, not as user-facing hidden magic, and must remain fail-fast instead of policing or silently resolving user environment conflicts.

## Decision

Adopt an explicit Compose backend design: zerobased.yaml v1 names Compose files, profiles, services, readiness, and endpoint claims; zerobased loads those files with compose-go, validates escape hatches that break integrity, injects scoped project identity and zerobased labels/extensions into the in-memory model, and drives the effective project through the Docker Compose Go API. The repo-local up process remains foreground and owns the lifecycle for the declared stack instance, while the daemon remains the sole owner of global host/domain claims and route publication.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-0 | system | Product behavior changes from process-only up to explicit project stack orchestration. | Reconcile stale goal from legacy autodiscovery to explicit control plane. |
| c3-1 | container | Current container still names legacy CLI internals; Compose orchestration belongs under the CLI/control-plane runtime boundary. | Update container purpose and child components before implementation. |
| c3-107 | component | Existing daemon component is stale but daemon claim ownership remains a governing boundary. | Preserve daemon-owned claim and route integrity in spec and tests. |
| N.A - new component pending | N.A - C3 topology is stale | Compose backend does not have a current component id yet. | Create or repurpose component through c3x once design names the boundary. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Runtime project names, hostnames, labels, and claim keys must be deterministic and explain conflicts. | Review and update for rewrite naming if stale. |
| N.A - explicit config rewrite guardrails | Guardrails live in AGENTS.md, CLAUDE.md, and RFC docs instead of a current C3 ref. | Create or map a C3 ref if implementation proceeds. |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no C3 rule currently mapped | Installed C3 topology has no rewrite-specific rules. | Capture enforcement in tests, specs, and C3 component contract. |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| Compose API research | Confirm compose-go loader and Docker Compose v2 API surfaces from primary sources. | Source links in design spec. |
| Product spec | Draft dependency graph, lifecycle, config contract, not-covered list, operational readiness, troubleshooting, and performance. | docs/superpowers/specs/2026-05-26-compose-backed-orchestration-design.md |
| C3 reconciliation | Map new spec and runtime boundary into C3 without direct .c3 file edits. | c3x list, c3x lookup, c3x check output. |
| Review | Use subagents to review edge cases, performance, and realism until no medium-or-higher issue remains. | Review transcript summary in task handoff. |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| C3 project topology | Add this ADR and reconcile stale legacy topology enough to name the Compose backend boundary. | c3x list --json --include-adr |
| C3 validation | Do not change the c3x engine or schema; use existing schema. | c3x schema adr and c3x check output. |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| zerobased.yaml v1 loader | Reject missing version and implicit Compose discovery. | Planned unit tests in config/compose loader layer. |
| Compose model validator | Reject or explicitly require opt-in for container_name, host port binds, external resources, and fixed project names that break zerobased ownership. | Planned loader validation tests. |
| up orchestrator | Claim before Compose start, wait readiness, publish routes, then cleanup on exit/failure. | Planned fake backend orchestration tests and system tests. |
| daemon IPC | Maintain claim-token-gated publication and owner records. | Existing daemon tests plus new route/stack ownership tests. |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Shell out to docker compose with generated YAML | Harder to inspect, test, and inject policy safely; errors become CLI text parsing instead of typed contracts. |
| Keep user docker compose up as the primary path | Too branch-agnostic; it can share one database across incompatible branches and obscure ownership. |
| Reimplement Compose semantics directly | High maintenance and unrealistic; Compose already owns dependency graph, networks, volumes, and container lifecycle semantics. |
| Return to Docker autodiscovery | Violates rewrite guardrails and hides the explicit config the user requested. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Compose API churn | Isolate Compose behind a backend interface and pin module versions. | go test ./... plus module API compile checks. |
| Integrity escape hatches | Validate known global/shared Compose fields before creating containers. | Unit tests for rejected model cases. |
| Slow stack startup | Load model once, create/start as a batch, readiness wait per declared service only. | System timing notes and fake backend ordering tests. |
| Cleanup failure leaves containers | Record stack owner and project name, perform best-effort down on foreground exit, expose troubleshooting command output. | System test with interrupt/cleanup path. |

## Verification

| Check | Result |
| --- | --- |
| c3x list --json --include-adr | Must show this ADR and affected topology. |
| c3x check --json | Existing stale C3 errors must be documented; no new direct .c3 edits allowed. |
| go test ./... | Required before implementation is marked done. |
| go test -tags system ./internal/systemtest -count=1 | Required after runtime behavior is implemented. |
| Subagent review summary | Required before implementation plan starts; no medium-or-higher unresolved issues. |
