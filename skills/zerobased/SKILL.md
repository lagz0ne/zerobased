---
name: zerobased
version: 2.0.0-rewrite
description: >-
  Rewrite-state reference for zerobased. The repo is moving to an explicit
  machine-local control plane with traced-TDD failure ownership.
---

# zerobased Rewrite State

The legacy zero-config Docker autodiscovery router is not the target architecture.

Do not use this skill as a CLI reference for the old behavior. The accepted product direction is:

- one daemon-owned local control plane
- `zerobased start` starts that control plane
- `zerobased up` starts an explicitly declared stack through that control plane
- route publication is daemon-owned desired state
- tests follow traced-TDD failure ownership

Current source of truth:

- `docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md`
- the traced-TDD RFC to be written next

Legacy behavior that must not be reintroduced by default:

- Docker event autodiscovery as the product model
- `zerobased run`
- routefile-driven routing
- classifier-based exposure
- legacy env discovery
- repo-local direct Caddy/admin API mutation

Before making changes:

1. Use C3.
2. Write or update the traced-TDD test contract for the layer being changed.
3. Implement with RED-GREEN-TDD.
