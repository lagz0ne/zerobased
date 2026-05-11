---
id: adr-20260511-release-help-hardening
c3-seal: 4b0bc6bca9ce865f0f3cf1a18550d060600642f240b9b0026c4405c81829cd4f
title: release-help-hardening
type: adr
goal: Harden release-facing help, README documentation, and release process guidance so a user or LLM can discover the next command without hitting a dead end.
status: implemented
date: "2026-05-11"
---

## Goal

Harden release-facing help, README documentation, and release process guidance so a user or LLM can discover the next command without hitting a dead end.

## Context

The current CLI has a basic global usage block plus terse error messages. README describes the current Docker-router product and planned fabric work, but release preparation needs command guidance, honest status, and a repeatable preflight. The release workflow already builds GitHub binaries and npm packages from `v*` tags, but the binary lacks an implemented `version` command despite Makefile and release ldflags setting `main.version`.

## Decision

Keep runtime behavior unchanged. Improve CLI help and error guidance, add a `version` command backed by the existing ldflags variable, update README with a guided command map and release checklist, and document release preflight around CI, npm token, GitHub tag, and smoke install.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-1 | container | Owns CLI commands and release-facing docs. | Keep behavior compatible; verify with tests/build/help smoke. |
| N.A - release workflow files | N.A - reason | Release workflow is repo infrastructure, not mapped in C3. | Inspect and document process. |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Help and README expose command, project, service, and route names users copy into workflows. | comply |
| ref-npm-binary-distribution | npm wrapper/platform package release flow is directly affected. | comply |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no active rule entities listed | No C3 rule entities available. | N.A - no active rules |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| cmd/zerobased/main.go | Add version command and guided help/errors. | go test ./...; go build |
| cmd/zerobased/share.go | Add next-step guidance for domain/share/unshare dead ends. | go test ./... |
| README.md | Make current product usage and release process clear. | inspection |
| release workflow/package files | Inspect existing tag release path and note requirements. | inspection |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| ADR registry | Add this ADR. | focused C3 check |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| CLI smoke | zerobased help and zerobased version produce actionable output. | local build smoke |
| Go tests | Existing behavior remains compatible. | go test ./... |
| Release docs | Tag-driven GitHub/npm process is documented. | README inspection |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Only edit README | LLMs and users often enter through CLI help, so dead ends remain. |
| Add new runtime release features | Release hardening should not change daemon/router semantics. |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Help promises commands not implemented | Keep help to current product plus clearly planned fabric section in README. | help smoke |
| Release process docs drift from workflow | Base checklist on inspected .github/workflows/release.yml and npm package layout. | inspection |

## Verification

| Check | Result |
| --- | --- |
| go test ./... | Required before implemented |
| go build ./cmd/zerobased | Required before implemented |
| ./tmp binary help/version smoke | Required before implemented |
| c3 check --include-adr --only <this-adr> | Required before implemented |
