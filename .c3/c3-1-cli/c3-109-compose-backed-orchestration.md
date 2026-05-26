---
id: c3-109
c3-seal: 5c8ff132e731ef7172f7924cc6ef3a308eb8626fcb11e3c593d0c96f8e9e96eb
title: compose-backed-orchestration
type: component
category: foundation
parent: c3-1
goal: Load explicit zerobased.yaml Compose declarations, validate and inject zerobased runtime identity, and drive Docker Compose as a typed backend for foreground stack orchestration.
---

## Goal

Load explicit zerobased.yaml Compose declarations, validate and inject zerobased runtime identity, and drive Docker Compose as a typed backend for foreground stack orchestration.

## Parent Fit

| Field | Value |
| --- | --- |
| Parent | c3-1 CLI/runtime container owns repo-local zerobased start and up behavior. |
| Boundary | This component sits behind stack orchestration and before Docker Compose lifecycle calls. |
| Upstream | It receives selected profile and Compose declarations from the stack orchestrator. |
| Downstream | It produces an effective Compose project and lifecycle operations for Docker Compose. |

## Purpose

This component owns Compose-backed dependency orchestration for the rewrite. It does not autodiscover Docker containers, scan arbitrary Compose files, mutate Caddy, or publish routes directly.

## Foundational Flow

| Aspect | Detail | Reference |
| --- | --- | --- |
| Input | Explicit zerobased.yaml v1 compose block names files, profiles, services, readiness, and endpoints. | adr-20260526-compose-backed-orchestration-design |
| Loader | compose-go loads declared files into a typed project model. | adr-20260526-compose-backed-orchestration-design |
| Identity | Generated Compose project name derives from canonical stack identity. | ref-naming-convention |
| Isolation | Global escape hatches are rejected before resource creation. | adr-20260526-compose-backed-orchestration-design |

## Business Flow

| Aspect | Detail | Reference |
| --- | --- | --- |
| Primary path | Stack orchestrator claims through daemon, then starts Compose dependencies and waits readiness before publication. | adr-20260526-compose-backed-orchestration-design |
| Failure path | Model and isolation failures become preflight rejection before containers exist. | adr-20260526-compose-backed-orchestration-design |
| Cleanup | Owned Compose projects are stopped during startup rollback and foreground exit. | adr-20260526-compose-backed-orchestration-design |
| Diagnostics | Errors name project path, profile, Compose project name, field path, and owner where relevant. | adr-20260526-compose-backed-orchestration-design |

## Governance

| Reference | Type | Governs | Precedence | Notes |
| --- | --- | --- | --- | --- |
| adr-20260526-compose-backed-orchestration-design | adr | Compose backend design and implementation sequence. | Highest for this component. | Created for current change. |
| ref-naming-convention | ref | Deterministic identity and user-facing conflict names. | Applies where still consistent with rewrite. | Existing ref needs rewrite update. |
| N.A - stale topology gap | N.A - C3 topology is stale | Rewrite RFC file paths govern product and traced-TDD behavior until C3 is reconciled. | Must not override ADR or spec files. | c3x check currently reports pre-existing schema drift. |

## Contract

| Surface | Direction | Contract | Boundary | Evidence |
| --- | --- | --- | --- | --- |
| Compose model loader | IN | Only declared files in zerobased.yaml are loaded. | stack orchestrator to compose backend | docs/superpowers/specs/2026-05-26-compose-backed-orchestration-design.md |
| Effective project | OUT | Returns a validated project scoped by generated runtime identity. | compose backend to Docker Compose API | docs/superpowers/specs/2026-05-26-compose-backed-orchestration-design.md |
| Lifecycle | IN/OUT | Create/start/down owned stack resources and report typed lifecycle errors. | compose backend to stack orchestrator | docs/superpowers/specs/2026-05-13-zerobased-traced-tdd-rfc.md |

## Change Safety

| Risk | Trigger | Detection | Required Verification |
| --- | --- | --- | --- |
| Hidden autodiscovery returns | Code scans compose.yaml without zerobased.yaml declaration. | capability/static tests and loader tests | go test ./... |
| Global resource collision | Compose model uses container_name, host ports, or custom resource names. | isolation validator tests | go test ./internal/composebackend ./internal/up |
| Cleanup leaves owned stack | Interrupt or startup failure after Compose start. | fake backend order tests and system tests | go test ./... and system-tag tests after implementation |
| API drift | Compose module changes method signatures. | compile and module pinning | go test ./... |

## Derived Materials

| Material | Must derive from | Allowed variance | Evidence |
| --- | --- | --- | --- |
| Product spec | Goal, Foundational Flow, Business Flow, Contract | Must remain explicit config and daemon-owned claims. | docs/superpowers/specs/2026-05-26-compose-backed-orchestration-design.md |
| Tests | Business Flow, Change Safety, Contract | Test names may differ if coverage table maps them. | docs/superpowers/specs/zerobased-acknowledgment-coverage.yaml |
| Runtime code | Purpose, Contract, Change Safety | Package names may vary if C3 codemap is updated. | internal/composebackend planned |
