# Zerobased Control Plane RFC

Date: 2026-05-13
Status: Draft
Scope: Product definition for zerobased as a machine-local development control plane

## Summary

Zerobased is a per-user, machine-local control plane for development stacks.

It owns:

- stack claims for stable local hostnames
- route namespace claims for stable host/path ownership
- the global local routing plane
- declared local application processes
- declared containerized dependencies
- connection metadata for clients and tooling

It does not try to be a general container platform, a remote deployment system, or a policy engine for user behavior. Its job is orchestration for local development with strict integrity and predictable failure.

The core user experience is:

1. `zerobased start` boots the local control plane.
2. `zerobased up` in a repo asks for a stable hostname claim.
3. If the claim succeeds, zerobased starts exactly the stack declared in `zerobased.yaml`.
4. If the claim fails, zerobased stops immediately and tells the user who owns the hostname.

The primary product path is explicit stack orchestration through `zerobased up`.

## Motivation

Local development is usually messy for three reasons:

- port ownership is implicit and collisions are opaque
- app processes and infra processes are started by unrelated tools
- route ownership is fragmented across repos, scripts, and user memory

This gets worse across branches and worktrees. The user sees a symptom like "port 8080 is already in use" but not the owner, intent, or next action.

Zerobased fixes this by moving local stack ownership into one explicit control plane with strict claim semantics.
Its goal is explicit ownership and conflict clarity, not automatic stable-host coexistence for every parallel worktree under default naming.

## Product Definition

Zerobased is a local stack orchestrator with a single control-plane daemon and a single routing runtime per user environment.

This document is a product contract, not a claim that the current repository already satisfies every behavior below. Where the current runtime still differs, the contract defines the direction the implementation must converge toward.

Control-plane view:

- https://diashort.apps.quickable.co/d/a69efe96

Startup flow:

- https://diashort.apps.quickable.co/d/c0fe4801

Claim lifecycle:

- https://diashort.apps.quickable.co/d/c0699859

## Goals

- One obvious owner for local hostnames, route namespaces, and TCP listeners.
- One command path for repo-local stacks.
- Explicit stack declaration. No hidden startup of random `compose.yaml` files.
- Strict hostname claim integrity. No silent fallback or hidden remapping.
- Clear conflict reporting with owner path, profile, and stack identity.
- Work cleanly with local processes and containerized dependencies in one product.
- Keep the mental model caveman simple.

## Non-Goals

- High-availability distributed control plane.
- Multi-machine orchestration.
- Production deployment orchestration.
- Protecting users from dangerous staging data usage.
- Automatic conflict resolution by inventing alternate hostnames.
- Auto-discovery and auto-start of undeclared Docker Compose projects.

## Deployment Model

Zerobased runs as:

- one daemon per user environment
- one routing runtime owned by that daemon
- many repo stacks registered into that routing plane

This is a deliberate single-node design.

For a normal laptop or workstation:

- one user
- one zerobased daemon
- one route owner

Supported environments:

- single-user laptop or workstation
- isolated single-user runner where zerobased can own the route runtime, local networking model, and any required bind permissions for the supported transport set

Not supported as a product target:

- shared multi-user hosts with one machine-global route plane
- multi-tenant CI runners that expect host-level coexistence across users/jobs

The current filesystem model already points this way through `~/.zerobased`.

## Core Concepts

### Project Root

The project root is the directory that owns `zerobased.yaml`.

All default naming and stack identity are derived from that root, not from the current working directory if the command is launched from a subdirectory.

### Stable Host Claim

Each stack requests a stable hostname claim.

Default hostname:

- `<project-root-name>.localhost`

Example:

- `/home/lagz0ne/dev/acountee` -> `acountee.localhost`

Override:

- explicit host declaration in `zerobased.yaml`

Claim policy:

- host claim keys are canonicalized before comparison: lowercase, no trailing dot, and one canonical hostname spelling per claim
- first claimant wins
- later claimants fail fast
- no fallback host is invented automatically
- worktrees of the same repo will collide on the default stable host unless the user explicitly chooses a different host
- this default favors integrity and clear ownership over automatic parallel-worktree convenience
- generic directory names such as `app`, `web`, or `api` are expected to need explicit hostname overrides in real multi-repo environments

### Stack

A stack is the declared unit zerobased starts and owns for a repo invocation.

A stack may contain:

- local processes
- declared Compose-backed services
- generated routes
- generated socket bridges

### Canonical Stack Identity

Every stack must have one canonical owner record.

Required fields:

- stack id
- claim token
- repo root path
- project root name
- profile
- working directory
- owning process id, when local processes are involved
- Compose project name, when Compose is involved
- creation time
- publication generation

Rules:

- repo path is the human-facing identity
- Compose project name is a backend attribute, not the canonical owner identity
- when Compose is used, its effective project/namespace must be derived from canonical stack identity so parallel valid stacks cannot collide on containers, networks, or volumes
- Compose project-name scoping alone is not sufficient isolation when the model contains global-name escape hatches
- if a Compose model declares `container_name`, explicit resource names that escape project scoping, or `external` resources, zerobased must either rewrite them into an explicit ownership mode or reject startup before resources are created
- a Compose-backed `owned` resource must never keep an unresolved machine-global identity that can collide with another valid stack
- route ids, socket artifacts, and cleanup actions must be namespaced by stack id plus claim token
- publish and remove operations must be rejected when the claim token does not match the current owner record
- publish and remove operations must also be fenced by publication generation
- delayed or retried operations from an older generation must be rejected even if the claim token still matches

This is the fencing mechanism that prevents stale cleanup and cross-worktree interference.

### Listener Claim

Stable TCP listeners are first-class claims, not side effects.

For every published TCP listener, zerobased must track:

- listener address
- owning stack id
- claim token
- purpose

Listener policy:

- listener claim keys are canonicalized before comparison: one canonical bind address spelling, protocol, and port tuple per claim
- claims that would collide at OS socket-bind time must be treated as the same effective listener claim even if their textual addresses differ
- the daemon must compute listener claims from the effective bind strategy it will actually use, not the raw user spelling
- the actual OS bind attempt is the final arbiter; if the OS reports conflict, the claim fails even if a prior canonicalization step missed it
- any stack resource that would bind a machine-global listener must have that listener claim reserved before the resource is started
- no supported `zerobased up` path may let a resource bind a machine-global listener outside the daemon claim table
- declared host-port binds, host-network listeners, or equivalent machine-global publications must be rewritten into claimed listeners or rejected before startup
- first claimant wins
- bind conflict fails the stack before publish
- stale listener ownership must be recoverable through the same owner record model as hostnames
- only transports the product explicitly supports may use stable listener claims

### Route Namespace Claim

HTTP route ownership is a first-class claim, not only a generated proxy line.

A route namespace claim reserves one stable host/path namespace before the stack starts resources.

Claim key:

- canonical host claim key
- route kind, currently HTTP
- normalized path namespace

For every published HTTP route, zerobased must track:

- canonical hostname
- normalized path namespace
- owning stack id
- claim token
- source route id or service id
- publication generation
- purpose

Route namespace policy:

- route namespace claim keys are canonicalized before comparison using the same host normalization and path normalization rules the routing runtime will apply
- root `/` claims the whole host namespace unless explicit composition rules say otherwise
- exact duplicate namespace claims conflict
- ambiguous overlaps conflict
- non-overlapping path namespaces on the same host may coexist only when the runtime precedence rules are deterministic and owner diagnostics remain exact
- broader/narrower cross-owner overlaps may be rejected until an explicit composition mode is implemented
- a stack must request every host, listener, and route namespace it will publish before starting resources
- no supported `zerobased up` path may publish a route namespace outside the daemon claim table
- first claimant wins for each namespace key
- stale route namespace ownership must be recoverable through the same owner record model as hostnames and listeners
- conflict output must name the existing owner record and the exact host/path namespace

This keeps path composition possible without weakening the golden rule: every visible route has one explainable owner.

### Dependency Ownership Modes

Every dependency zerobased touches must have one ownership mode.

Modes:

- `owned`: started for one stack invocation and torn down with that stack
- `shared`: managed by zerobased across multiple stacks using persisted owner leases or reference counts
- `reused`: preexisting dependency that zerobased may depend on but must not tear down

Rules:

- ownership mode must be declared or deterministically derived before startup
- shutdown and recovery behavior must follow ownership mode
- shared dependencies require persisted ownership metadata strong enough to survive daemon restart
- shared dependencies also require an explicit app-supplied compatibility contract plus a normalized compatibility fingerprint; stacks may share only when both match
- zerobased must not infer application-semantic compatibility from infra metadata alone
- the compatibility fingerprint must cover the dependency aspects that affect safe reuse, such as image/runtime identity, config/env inputs, persistent data shape, and startup/init contract
- without an explicit compatibility contract, a dependency may be `owned` or `reused`, but it is not eligible for `shared` mode
- zerobased must maintain one canonical dependency identity per touched backend and reject conflicting ownership modes for the same live dependency identity
- if one live dependency identity is already registered as `owned`, `shared`, or `reused`, later stacks must either match that mode under the same compatibility contract or fail before startup
- if the fingerprint does not match, zerobased must refuse sharing rather than guessing
- reused dependencies are never stopped as part of one stack’s rollback or crash cleanup

### Shared Domains

External domain sharing is an optional exposure layer owned by the same control plane.

Rules:

- shared-domain publication is daemon-owned
- shared-domain host variants are attached to the same local owner record as the underlying stack
- shared-domain publication is a secondary exposure plane layered on top of an already-published local stack
- external domains do not create separate stack identity
- local publication does not wait for optional shared-domain success
- if a shared-domain variant cannot be published safely, local publication remains live and the stack is marked degraded for shared exposure
- auxiliary/shared-domain panics or unknown failures after local publication commits are degraded status with origin details, not request failures
- within one daemon, a shared-domain hostname still has at most one local owner record at a time
- zerobased owns local publication and share intent; it does not own global DNS, public tunnel reachability, or third-party external writers
- shared-domain ownership is therefore advisory and local-control-plane scoped, not a guarantee of global exclusivity on the public internet
- if a newer local generation cannot safely refresh an older shared-domain publication, the older shared-domain publication must be withdrawn rather than left serving stale traffic

### Routing Runtime

The routing runtime is the single local route owner. It serves:

- stable HTTP hostnames
- path-based HTTP routes
- TCP listeners for non-HTTP services

Product-wise, there is one routing runtime. Whether it is embedded Caddy or a daemon-owned external Caddy process is an implementation detail.

Only the daemon may mutate the routing runtime.

Repo-local flows such as `zerobased up` must publish desired state to the daemon. They must not call Caddy directly once this control-plane model is in effect.

### Legacy Compatibility

The legacy Docker-autodiscovery router path has been removed from the rewrite working tree.

Product rule:

- explicit stack orchestration through `zerobased start` and `zerobased up` is the only path covered by this RFC
- legacy Docker autodiscovery must not be reintroduced as a parallel product model
- if any legacy behavior is temporarily restored for migration, it must be isolated behind an explicit compatibility boundary and remain outside RFC acceptance coverage
- the product must not present compatibility behavior and explicit owner-record orchestration as equivalent ownership models

## How It Works

### Components

The product has four major runtime roles:

1. Control-plane daemon
2. Stack runner
3. Routing runtime
4. Compose backend

### 1. Control-Plane Daemon

The daemon owns:

- stack registry
- hostname claims
- route namespace claims
- route table
- socket bridge registry
- operational state under `~/.zerobased`
- the authoritative owner record registry for hosts and listeners

It receives events from stack runners:

- claim requested
- startup succeeded
- startup failed
- stack stopped
- endpoints changed
- lease renewed

The daemon is the only authoritative read path for:

- claim checks
- `status`
- `who <host>`
- shared-domain ownership
- published route ownership

### 2. Stack Runner

`zerobased up` acts as a stack runner.

Its job is:

1. find project root
2. load `zerobased.yaml`
3. derive the effective dependency and publication plan
4. request every required hostname, listener, and route-namespace claim from the daemon and validate publishability before starting resources
5. start declared Compose services, if any
6. start declared local processes, if any
7. wait for readiness
8. commit one publication generation to the daemon
9. keep ownership until stopped

If any required phase fails, the stack runner releases the claim and exits non-zero.

Liveness contract:

- startup claims have a bounded startup lease
- published stacks maintain a renewable session lease with the daemon
- loss of lease during startup causes the unpublished claim to expire automatically
- loss of lease after publish does not by itself force quarantine; the daemon must try to classify the stack as reattachable, provably dead, or ambiguous
- if lease is lost but live process/runtime evidence still matches the owner record, the daemon or a reconnecting runner may reattach within the grace window without manual cleanup
- if lease is lost and the stack is provably dead, the daemon treats it as a crash, withdraws publication, and releases the identity cleanly
- only ambiguous ownership state enters suspected-orphan
- while a stack is in ambiguous suspected-orphan, local publication must be withdrawn immediately and the claim remains reserved only for recovery or explicit release
- suspected-orphan has a bounded grace period; after that grace expires, the daemon must choose one terminal state: reattach if ownership is proven, release if the stack is proven dead, or quarantine only when live ownership remains ambiguous

Startup and crash cleanup rules:

- resources started by the current stack invocation must be tracked as owned resources
- if startup fails, zerobased stops or rolls back owned resources before releasing the claim
- if a stack crashes, zerobased removes owned publication artifacts and stops owned resources where safe
- shared dependencies are released according to persisted shared-ownership metadata and are not torn down while still leased by another live stack
- reused dependencies are never torn down as part of one stack’s rollback or crash cleanup

### 3. Routing Runtime

The routing runtime is fed from daemon-owned desired state.

It should not be mutated ad hoc by arbitrary repo-local scripts.

Desired-state model:

- daemon computes full route configuration
- runtime updates must preserve atomic visibility per stack publication generation
- route changes are event-driven and batched by stack publication generation

Triggers for reload:

- daemon startup
- stack becomes ready
- stack stops or crashes
- route/domain metadata changes
- explicit reload command

Implementation note:

- the product requires atomic stack publication semantics
- the implementation may use full desired-state reloads or batched delta updates
- the implementation must not reload or mutate once per tiny per-service transition if the same stack generation can be published in one batch

Routing rules:

- paths are normalized before comparison: root is `/`, non-root paths have no trailing slash
- duplicate slashes and dot segments are normalized according to the same path-matching semantics the routing runtime will apply
- if equivalent-path handling cannot be proven identical between validator and runtime, the ambiguous routes must be rejected
- matching is by path-segment boundary, not raw text prefix
- `/api` matches `/api` and `/api/...`
- `/api` does not match `/api2`
- exact match beats wildcard expansion when both are otherwise equally specific
- longest normalized path prefix wins
- equal-specificity ambiguous overlaps are invalid and must be rejected
- daemon and `up` publication paths must use the same precedence rules

Publication rules:

- readiness alone does not make a stack visible
- a stack becomes visible only after the daemon commits one publication generation
- core local publication is all-or-nothing for the core local stack surface
- the core local stack surface includes local routes, stable local listener claims, and required socket bridges
- if any core local publication step fails before the first committed local generation, zerobased must roll back that generation and keep the local stack unpublished
- if a replacement generation fails after a prior local generation is already committed, zerobased must preserve the last committed local generation until the replacement generation commits successfully or the stack is explicitly released
- shared-domain exposure and cached/generated connection metadata are auxiliary publication surfaces
- failure in an auxiliary publication surface must not take down an already-valid local publication; it marks the stack degraded for that auxiliary surface instead
- auxiliary publication surfaces are post-core-local-publication work in this product generation
- the daemon may coalesce multiple near-simultaneous generation commits into one bounded runtime update batch while preserving per-stack generation ordering and visibility guarantees

### 4. Compose Backend

Compose is a backend, not the user interface.

Zerobased uses Compose only when the repo explicitly asks for it in `zerobased.yaml`.

Expected implementation:

- `compose-go` loads and normalizes user Compose files
- zerobased injects runtime policy
- zerobased validates or rewrites Compose escape hatches before resource creation, including `container_name`, explicit global resource names, host-network publication, and machine-global host-port binds
- any Compose model that still escapes stack ownership after normalization is rejected as unsupported for `zerobased up`
- Compose SDK runs lifecycle operations

This avoids brittle YAML string rewriting and keeps compatibility with normal Compose models.

## Explicit Configuration Model

Zerobased must never start Compose only because a `compose.yaml` file happens to exist.

Compose use is explicit in `zerobased.yaml`.

This preserves user intent and avoids surprising infra startup.

Configuration principles:

- simple defaults
- explicit ownership
- no hidden auto-discovery
- hostname derived from project root name unless overridden
- Compose startup is impossible unless declared explicitly

## Coverage

### Covered

- Local app process orchestration.
- Docker Compose-backed local dependencies when explicitly declared.
- Stable local hostname ownership.
- Stable local route namespace ownership.
- Conflict reporting with owner details.
- HTTP reverse proxy routing.
- TCP listener routing for the supported non-HTTP transport set of the current product generation.
- Local socket bridge generation for supported DB-style ports.
- Shared-domain fan-out for published stacks.
- Per-stack readiness and lifecycle management.
- Per-user machine-local control plane operations.
- Owner lookup for stable hosts and listeners.
- Stack recovery decisions after daemon restart.

### Not Covered

- Production ingress or cluster routing.
- Distributed leases or consensus.
- Team-shared or multi-tenant host routing plane.
- Policy enforcement for staging writes.
- Arbitrary undeclared Docker containers.
- Automatic host fallback on conflict.
- Automatic merging of multiple repos into one shared namespace.
- Windows support in the current product generation.
- generic arbitrary TCP proxy support beyond the supported transport set of the current product generation
- Compose declarations that require unresolved global resource names or unmanaged machine-global listener binds

### Reasoning

Zerobased is valuable because it narrows the local-dev problem to one machine, one user environment, and one explicit stack declaration path.

Every non-covered item above either:

- belongs to a different product category
- weakens integrity
- or introduces automation that surprises the user

## Integrity Level

Integrity is the golden rule.

Product invariants:

- one stable host has at most one owner
- one route namespace has at most one owner
- one stable listener has at most one owner
- stack startup does not continue after claim failure
- undeclared infra is never auto-started
- no supported stack may bind a machine-global listener outside the daemon claim table
- no Compose-backed owned resource may escape stack ownership through unresolved global names
- the daemon is the source of truth for published routes
- route publication happens only after readiness
- startup failure releases ownership cleanly
- publish/remove operations are fenced by claim token
- externally visible publication is atomic per stack generation
- older publication generations can never overwrite newer ones

What zerobased will do:

- fail fast
- explain the real owner
- preserve honest system state

What zerobased will not do:

- silently swap to another hostname
- partially publish a broken stack as healthy
- start extra infra because it guessed wrong

Conflict error quality matters. A useful error includes:

- hostname
- owning project
- owning profile
- owning repo path
- owning stack identity
- whether the conflicting claim is local-only or also shared to external domains
- exact conflicting route namespace when the conflict is path-based
- exact next actions

## Operational Readiness

### Health Model

The control plane is healthy when:

- daemon is running
- routing runtime is running
- route table can be reloaded
- declared stack endpoints match daemon registry
- socket bridges are healthy

### State Management

Persistent local state under `~/.zerobased` should include:

- daemon pid and lock data
- stack registry
- claim table
- route namespace claim table
- listener claim table
- sockets
- route-runtime state if needed for recovery
- logs
- owner records with claim tokens and generation metadata
- process birth or equivalent strong reattachment proof for local processes
- backend identity proof for Compose-backed stacks
- lease/session state

Proof examples:

- local processes: pid plus process birth/start-time proof plus daemon-issued session token metadata
- Compose-backed stacks: daemon-issued stack/session labels plus matching backend runtime metadata

### Recovery

On daemon restart:

1. restart route runtime if needed
2. load persisted claim and stack metadata, if still valid
3. reconcile with actual running stacks and owning processes
4. reattach claims only when owner record and live process/runtime evidence match
5. release claims that are provably dead instead of quarantining dead state
6. quarantine only stacks whose live ownership state remains ambiguous after reconciliation
7. republish live routes from reconciled daemon state
8. require explicit user resolution before a quarantined identity can be reused

On stack crash:

- release claim
- deregister routes
- remove socket bridges
- leave a clear error state in logs and status output

On local-process stacks after daemon crash:

- if the owning pid, process birth proof, and owner record token/generation evidence all match, the daemon may reattach and restore publication without manual cleanup
- if the local process set is provably gone, the daemon releases the claim and cleans up owned artifacts
- quarantine is reserved for conflicting or insufficient evidence while live ownership may still exist

On Compose-backed stacks after daemon restart:

- reattachment requires matching owner record plus backend identity proof
- Compose project name alone is not enough proof

### Observability

Operational commands should expose:

- `zerobased ps`
- `zerobased status`
- `zerobased logs`
- `zerobased who <host>`
- `zerobased who --port <port>`
- `zerobased release <stack-or-host>` for explicit owner release, including quarantined identities

Operational data should show:

- stack owner
- repo path
- profile
- startup phase
- readiness status
- published endpoints
- claimed route namespaces
- claim age
- claim token or generation id when needed for debugging

## Troubleshooting

### Host Claim Conflict

Symptom:

- stack fails immediately because hostname is occupied

Expected output:

- exact hostname
- current owner path/profile/stack
- how to stop or inspect the owner

Reasoning:

- "port in use" is weak
- "owned by project X at path Y" is actionable

### Route Runtime Down

Symptom:

- daemon running but hosts do not resolve or proxy

Expected action:

- daemon detects runtime failure
- tries controlled restart
- if restart fails, status becomes degraded and new stacks should not publish
- existing claims remain reserved but unpublished until recovery succeeds or the stack is released

### Compose Service Fails Readiness

Symptom:

- `zerobased up` hangs or exits before publish

Expected action:

- startup stops before route publication
- logs show which declared service or process failed readiness
- no stable host or listener becomes visible for that stack generation

### Stale Socket Bridge

Symptom:

- socket file exists but target is dead

Expected action:

- health checks mark it unhealthy
- daemon removes stale bridge
- next valid startup recreates it

### Orphaned Owner Record

Symptom:

- hostname is marked owned, but no healthy stack can be found

Expected action:

- daemon reconciles owner record against live process/runtime evidence
- unverifiable owners are quarantined instead of silently reused
- the user resolves or force-releases the quarantined identity explicitly with `zerobased release <stack-or-host>`

### Path Overlap Conflict

Symptom:

- one stack declares overlapping paths with ambiguous precedence
- or another stack already owns the same host/path namespace

Expected action:

- validation fails before publish
- error explains which paths are ambiguous or which owner already holds the namespace
- output includes current owner path/profile/stack for cross-stack namespace conflicts

## Onboarding

The onboarding story should be short.

Machine setup:

1. install zerobased
2. start zerobased once
3. verify daemon health

Repo setup:

1. add `zerobased.yaml`
2. declare local processes
3. declare optional Compose dependencies explicitly
4. run `zerobased up`

Platform support for this product generation:

- Linux
- macOS

Not part of the current product generation:

- Windows

The user should not need to:

- manually reserve ports
- manually wire proxy routes
- remember which branch owns which host

They may still need to:

- choose explicit hostnames when they want multiple worktrees of the same repo live at the same time under stable names
- choose explicit hostnames when generic directory names would otherwise collide across unrelated repos

## Availability

Availability is local control-plane availability, not distributed service uptime.

Target posture:

- zerobased should survive app stack churn
- one stack failure should not kill the whole route plane
- route reloads should be atomic from the user point of view
- daemon restart should be recoverable without manual cleanup in the common case
- route-plane failure should degrade publication but not silently corrupt ownership

Not targeted:

- zero-downtime upgrades across a cluster
- quorum-based control-plane resilience

## Performance

Performance matters because this sits on the hot path of developer feedback loops.

Primary performance goals:

- fast claim checks
- low-latency route publication after readiness
- low steady-state CPU while idle
- one publication batch per stack generation in the common case
- cheap status inspection from daemon-owned state, not fresh full-system scans

User-visible target budgets on a healthy local machine within supported scale:

- warm claim check: typically under 50ms, should remain under 200ms
- `who`/`status` from warm daemon state: typically under 100ms, should remain under 300ms
- route publication after the last required readiness signal: typically under 250ms, should remain under 1s
- daemon restart to authoritative status availability: typically under 2s, should remain under 5s

Expected cost centers:

- Compose model loading
- service readiness checks
- route-runtime reloads
- socket bridge health checks
- domain variant fan-out

Performance principles:

- event-driven, not polling-heavy
- route updates must batch by stack generation, not once per tiny mutation
- concurrent stack churn should be coalesced into bounded global runtime update batches when that does not violate per-stack ordering
- keep route computation in-memory
- status and claim reads must come from daemon state in the common path
- warm-path claim and status reads must be served from snapshots or equivalent read-isolated daemon state, not blocked behind heavy publish/reload work
- domain fan-out is a first-class scale dimension
- avoid shelling out when in-process APIs exist

Target scale:

- tens of active stacks on one machine
- low hundreds of total expanded publication objects

Expanded publication objects include:

- local routes
- shared-domain route variants
- listener claims
- socket bridges

Low tens of external domains are acceptable only while the total expanded publication object count remains within the supported envelope.

The product is not optimized for hundreds of concurrently active stacks on one host.

Overload policy:

- when the supported envelope is exceeded, zerobased may reject new claims or defer auxiliary shared-domain publication
- overload must not silently corrupt ownership or route visibility for already-published local stacks
- preserving claim integrity and warm-path reads is higher priority than accepting more work

## Availability and Performance Tradeoffs

Some choices are deliberate:

- strict hostname claims reduce convenience but preserve integrity
- strict default stable host claims mean same-repo worktrees collide unless the user names them explicitly
- explicit Compose declaration reduces surprise but adds one config step
- single route owner reduces conflict but centralizes one local dependency
- atomic publication matters more than the exact internal route update strategy
- branch/worktree clarity is guaranteed through ownership diagnostics, not through automatic no-config parallel stable-host allocation

## Security and Permission Model

Zerobased is not a sandbox.

It already implies local trust because it:

- orchestrates local processes
- talks to Docker
- manages local routes

Permission boundaries should still stay explicit:

- only declared repo config can start infra
- route claims are explicit
- ownership is auditable through status commands

Implementation caveats that affect the product boundary:

- if the routing runtime is embedded and binds low ports directly, Linux capability management becomes a product concern
- if the routing runtime remains external, it must still be daemon-owned and not directly mutated by repo-local runners

These do not change the product model, but they do constrain acceptable implementations.

## Current Implementation State

The working tree is intentionally clean-slate for this contract.

Current state:

- legacy Docker autodiscovery, routefile routing, `zerobased run`, direct Caddy mutation, and classifier-based exposure have been removed from executable code
- the remaining CLI is a minimal rewrite notice stub
- this RFC defines the product contract, not an already implemented behavior set
- the next required artifact is the traced-TDD RFC that assigns typed failure ownership and test placement for `zerobased start` and `zerobased up`

This is deliberate. New behavior should be added only through RED-GREEN-TDD against the traced-TDD ownership graph.

## Recommended Implementation Direction

The product should be implemented as:

- per-user zerobased daemon
- single daemon-owned routing runtime
- explicit repo-local stack declarations
- Compose as backend only
- authoritative daemon-owned owner records and claim tokens
- atomic stack publication generations
- strict claim semantics

The `acountee` repo is the right first pilot because it already exercises:

- local app process startup
- shared and repo-owned dependencies
- real host naming needs
- real conflict-diagnostics pressure across branches and worktrees

## Open Questions

- Whether the routing runtime is embedded or external in the first release.
- How much owner/process metadata is minimally required for safe reattachment after restart.
- Whether readiness contracts need stronger built-in diagnostics for Compose-backed services.

## Decision Summary

Zerobased should be treated as a local development control plane, not a bag of scripts.

Its value comes from:

- one honest owner for local routes
- explicit stack intent
- strict integrity on hostname claims
- clear diagnostics when conflicts happen

That is the complete product direction.
