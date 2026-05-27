---
id: adr-20260527-route-runtime-shutdown-order
c3-seal: d2048bc9f92b9c6a9dbce4b4de6dc4a1a9ef9758f0363576d184e7301e0473aa
title: route-runtime-shutdown-order
type: adr
goal: Make the current rewrite usable as a real local control plane for HTTP development by giving `zerobased start` a default localhost route runtime and by preserving route integrity during `zerobased up` shutdown. The change must keep explicit `zerobased.yaml` v1 behavior, publish routes only after readiness, and remove routes before local processes or Compose services are torn down.
status: proposed
date: "2026-05-27"
---

## Goal

Make the current rewrite usable as a real local control plane for HTTP development by giving `zerobased start` a default localhost route runtime and by preserving route integrity during `zerobased up` shutdown. The change must keep explicit `zerobased.yaml` v1 behavior, publish routes only after readiness, and remove routes before local processes or Compose services are torn down.

## Context

The daemon currently only wires a route runtime when `ZEROBASED_ROUTE_RUNTIME_BACKEND=admin-url` is set, so default `zerobased start` accepts startup but later rejects `up` route publication. Review also found LIFO cleanup unpublishes routes after the app process and Compose services stop, creating a short stale-route window. The affected topology is the daemon route runtime and `up` orchestration cleanup order.

## Decision

Use a daemon-owned Docker Caddy HTTP route runtime for the current HTTP-only slice. The default CLI route runtime writes a generated Caddyfile under `ZEROBASED_HOME`, starts an owned `zerobased-route-runtime-caddy` container with `127.0.0.1:80:80` and `host.docker.internal:host-gateway`, routes declared hosts/paths to `host.docker.internal:<route.port>`, and fails daemon startup if Docker, the owned container, or port binding is unavailable. Route state changes are staged on a copied map and committed only after Caddy reload succeeds. Keep `admin-url` as the test/alternate route runtime. After successful publication, `up` attempts route release before stopping local processes or Compose; if release fails, it returns a cleanup error and leaves resources running rather than creating a dead published route.

## Affected Topology

| Entity | Type | Why affected | Governance review |
| --- | --- | --- | --- |
| c3-107 | component | Owns zerobased start, route runtime startup, and route publication handling | Ensure default start fails fast when the route port cannot bind and keeps test backend support |
| c3-109 | component | Owns zerobased up process/Compose cleanup and route publication lifecycle | Ensure route deletion happens before app/process teardown |

## Compliance Refs

| Ref | Why required | Action |
| --- | --- | --- |
| ref-naming-convention | Route names, hosts, and labels stay explicit and deterministic | comply |
| N.A - no additional governing ref | Existing control-plane RFC and traced-TDD docs already govern this rewrite slice in repo docs | review |

## Compliance Rules

| Rule | Why required | Action |
| --- | --- | --- |
| N.A - no explicit C3 rule applies | No project rule entity governs route runtime implementation beyond ADR and tests | N.A - no rule to update |

## Work Breakdown

| Area | Detail | Evidence |
| --- | --- | --- |
| route runtime | Add default local HTTP proxy route runtime and tests | internal/route_runtime/, internal/daemon/ tests |
| daemon startup | Wire default route runtime unless admin-url env override is configured | internal/daemon/start.go test and zerobased start smoke |
| up cleanup | Unpublish route before stopping process and Compose backend | internal/up/runner.go test |
| system smoke | Verify real default daemon can serve http://acountee.localhost | local smoke command with Acountee |

## Underlay C3 Changes

| Underlay area | Exact C3 change | Verification evidence |
| --- | --- | --- |
| N.A - application code ADR | No C3 CLI/schema/template implementation changes are required | c3x check --json executed after work; existing C3 debt reported separately |

## Enforcement Surfaces

| Surface | Behavior | Evidence |
| --- | --- | --- |
| internal/route_runtime tests | Proxy applies, replaces, deletes, and forwards HTTP routes | go test ./internal/route_runtime -count=1 |
| internal/daemon tests | Default daemon config has a real route runtime; admin-url override still works | go test ./internal/daemon -count=1 |
| internal/up tests | Route deletion happens before process/Compose cleanup | go test ./internal/up -count=1 |
| system smoke | zerobased start + zerobased up can serve a real project route | Acountee smoke with HTTP request to acountee.localhost |

## Alternatives Considered

| Alternative | Rejected because |
| --- | --- |
| Keep nil runtime and document env-only admin-url | Default zerobased start would still fail later during up, which is surprising and not usable for Acountee |
| Silent no-op runtime | It would make up appear healthy while no domain route works, violating the integrity/no-surprise rule |
| In-process Go proxy on port 80 | This machine cannot bind privileged port 80 from the user process (net.ipv4.ip_unprivileged_port_start=1024), so it would fail on the target dev host |
| Full custom Caddy/L4 integration now | It is the right long-term backend for TCP/L4, but stock Docker Caddy is enough to validate current HTTP route publication today |

## Risks

| Risk | Mitigation | Verification |
| --- | --- | --- |
| Host port 80 is occupied or rootless Docker cannot bind it | Docker Caddy bootstrap fails daemon startup immediately with the Docker error | default zerobased start smoke |
| Docker Caddy cannot reach loopback-only dev servers on Linux bridge networking | Projects using the Docker Caddy backend must bind the process port on a host-gateway reachable address; Acountee does this with Vite --host 0.0.0.0 --strictPort | Acountee smoke through http://acountee.localhost |
| Failed Caddy reload corrupts in-memory desired route state | Stage route map edits on a copy and commit memory only after reload succeeds | TestDockerCaddyDoesNotCommitRouteStateWhenReloadFails |
| Route unpublish fails during shutdown | Return cleanup failure and preserve process/Compose resources so the published route still has a live target | TestRunDoesNotStopPublishedResourcesWhenReleaseFails |
| Path matching picks the wrong route | Use exact host and longest path prefix when rendering Caddyfile | route runtime unit test |
| Stale non-zerobased runtime container exists | Bootstrap inspects ownership label and rejects non-owned containers; runtime container name is specific enough to avoid ordinary user Caddy containers | route runtime bootstrap test |

## Verification

| Check | Result |
| --- | --- |
| go test ./internal/route_runtime ./internal/daemon ./internal/up -count=1 | passed |
| go test ./... -count=1 | pending final rerun |
| go test -tags system ./internal/systemtest -count=1 | pending final rerun |
| Acountee default zerobased start + zerobased up smoke against http://acountee.localhost | pending final rerun |
