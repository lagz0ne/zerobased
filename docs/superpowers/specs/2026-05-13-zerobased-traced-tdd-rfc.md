# zerobased Traced-TDD RFC

Status: Draft
Date: 2026-05-13
Depends on: `docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md`

## Purpose

This RFC defines the test ownership model for the clean-slate zerobased rewrite.

The product contract is the explicit local control plane. The test contract is traced-TDD: every externally visible failure traces to one owning layer through typed, machine-checkable error contracts.

Behavior code must not be added before its owning layer, typed errors, acknowledgments, and tests are defined here or in a follow-up RFC that extends this one.

## Scope

Covered:

- `zerobased start`
- `zerobased up`
- daemon-owned control plane
- daemon IPC/API server
- stack orchestration through a control-plane client
- route runtime adapter
- control-plane store
- Compose backend adapter
- local process launcher adapter
- readiness probe adapter
- socket bridge adapter
- auxiliary publication adapter for shared-domain and cached/generated metadata surfaces
- in-process fabric core
- black-box system acceptance tests for `start` and `up`

Not covered:

- legacy Docker autodiscovery
- `zerobased run`
- routefile-driven routing
- classifier-based exposure
- legacy env discovery
- direct Caddy mutation from repo-local `up`
- package release tests beyond minimal scaffolding

Legacy behavior may exist in git history. It is not acceptance coverage for this rewrite.

## Layer Graph

Graph: `https://diashort.apps.quickable.co/d/9f2e5fa3`

The graph has three verification paths.

`zerobased start` path:

- CLI `start`
- daemon IPC/API server
- control-plane service
- control-plane store
- route runtime adapter

`zerobased up` path:

- CLI `up`
- stack orchestrator
- control-plane client
- daemon IPC/API server
- remote control-plane service
- fabric core
- Compose backend adapter
- process launcher adapter
- readiness probe adapter
- socket bridge adapter

System acceptance path:

- built `zerobased` binary
- temp `ZEROBASED_HOME`
- real daemon process
- real daemon IPC/API socket
- fake route runtime through the daemon-owned route-runtime adapter seam
- real short-lived local process

Compatibility-only legacy paths are outside RFC acceptance.

## Error Contract Shape

Every layer must expose typed errors.

Go shape:

```go
type Layer string

const (
	LayerCLI              Layer = "cli"
	LayerDaemonIPC        Layer = "daemon_ipc"
	LayerControlPlane     Layer = "control_plane"
	LayerControlPlaneStore Layer = "control_plane_store"
	LayerControlPlaneClient Layer = "control_plane_client"
	LayerStackOrchestrator Layer = "stack_orchestrator"
	LayerRouteRuntime     Layer = "route_runtime"
	LayerComposeBackend   Layer = "compose_backend"
	LayerProcessLauncher  Layer = "process_launcher"
	LayerReadinessProbe   Layer = "readiness_probe"
	LayerSocketBridge     Layer = "socket_bridge"
	LayerAuxPublication   Layer = "auxiliary_publication"
	LayerFabricCore       Layer = "fabric_core"
)

type Code string

type Error struct {
	Layer  Layer
	Code   Code
	Phase  string
	Owner  string
	Cause  error
	Origin []Layer
}
```

Rules:

- `Layer` names the boundary that owns the current error identity.
- `Code` is machine-checkable.
- The unique error identity is `(Layer, Code)`, not `Code` alone.
- Reused human-readable code names such as `Critical` are allowed only when the `Layer` differs.
- Propagated remote errors keep their original `(Layer, Code)` identity; wrappers may add a new outer `(Layer, Code)` through `Cause`.
- `Phase` is required for CLI `StartFailed` and `UpFailed`.
- `Owner` carries conflict ownership when relevant.
- `Cause` keeps the lower error.
- `Origin` records where unknowns first crossed a boundary.
- Unknown errors may cross a boundary only as `Critical`.
- Exception: post-core auxiliary-phase unknowns are converted to `PublicationDegraded` status with origin details because they are not request failures after local publication commits.
- A mechanical test must prove every declared error/status `(Layer, Code)` pair is unique and every error code name reused across layers has an explicit propagation or transform rule.

The exact Go package name is implementation detail. The contract is not.

## Declared Error Sets

### CLI

`start` declares:

- `InvalidInvocation`
- `StartFailed`
- `Critical`

`up` declares:

- `InvalidInvocation`
- `UpFailed`
- `Critical`

`StartFailed` and `UpFailed` must be discriminated by phase and cause.

Examples:

- `StartFailed{phase:"boot", cause: RouteRuntimeUnavailable}`
- `UpFailed{phase:"claim", cause: HostClaimConflict}`
- `UpFailed{phase:"claim", cause: RouteNamespaceClaimConflict}`
- `UpFailed{phase:"readiness", cause: ReadinessFailed}`
- `UpFailed{phase:"publish", cause: PublicationRejected}`

### Control-Plane Service

Declares originated request errors:

- `BootFailed`
- `HostClaimConflict`
- `ListenerClaimConflict`
- `RouteNamespaceClaimConflict`
- `LeaseRejected`
- `PublicationRejected`
- `StaleTokenRejected`
- `RecoveryConflict`
- `RouteRuntimeUnavailable`
- `StoreUnavailable`
- `CorruptState`
- `Critical`

Declares status signals:

- `PublicationDegraded`

The service originates claim, lease, publication, status, and recovery decisions. It consumes store and route-runtime adapter errors. `PublicationDegraded` is typed status, not a request failure.

### Daemon IPC/API Server

Declares:

- `SocketBindFailed`
- `HealthCheckFailed`
- `ProtocolDecodeFailed`
- `DispatchFailed`
- `Critical`

The daemon IPC/API server owns local socket binding, request decoding, response encoding, health serving, and dispatch into the control-plane service. It does not own claim, lease, route, or publication decisions.

### Control-Plane Store

Declares:

- `OpenFailed`
- `LockHeld`
- `ReadFailed`
- `WriteFailed`
- `CorruptState`
- `AtomicCommitFailed`
- `Critical`

Store tests own filesystem persistence and restart rehydration behavior.

### Control-Plane Client

Declares:

- `ConnectionFailed`
- `Critical`

The client owns transport failures only. Remote control-plane errors keep their original control-plane `(Layer, Code)` identity across the process/socket boundary.

### Stack Orchestrator

Declares:

- `ConfigInvalid`
- `ClaimRejected`
- `PreflightRejected`
- `DependencyStartFailed`
- `ReadinessFailed`
- `GenerationCommitFailed`
- `CleanupFailed`
- `Critical`

`ClaimRejected` is distinct from generic preflight rejection. It is the only orchestrator error that maps to CLI `UpFailed{phase:"claim"}`.

### Route Runtime Adapter

Declares:

- `BootstrapFailed`
- `ReloadFailed`
- `ApplyRejected`
- `UnsupportedTransport`
- `Critical`

### Compose Backend Adapter

Declares:

- `ModelInvalid`
- `IsolationRejected`
- `LifecycleFailed`
- `Critical`

### Process Launcher Adapter

Declares:

- `SpawnFailed`
- `ExitedEarly`
- `ShutdownFailed`
- `Critical`

### Readiness Probe Adapter

Declares:

- `ProbeInvalid`
- `Timeout`
- `ProbeFailed`
- `Critical`

### Socket Bridge Adapter

Declares:

- `BridgeInvalid`
- `BridgeStartFailed`
- `BridgeHealthFailed`
- `BridgeStopFailed`
- `Critical`

Socket bridge failures are core local publication failures when the socket bridge is required for the stack surface.

### Auxiliary Publication Adapter

Declares:

- `SharedDomainPublishFailed`
- `MetadataRenderFailed`
- `MetadataCacheFailed`
- `Critical`

Auxiliary publication failure is daemon-owned. It does not reject a valid core local publication. It becomes `PublicationDegraded` in the control plane and must appear in status.

Auxiliary publication runs only after a core local generation is committed in this RFC. Therefore every auxiliary-phase failure, including aux `Critical`, degrades the auxiliary surface instead of failing the already-valid local publication. The degradation status must preserve the aux origin chain for troubleshooting.

### Fabric Core

Declares:

- `ConfigInvalid`
- `IdentityInvalid`
- `NamespaceInvalid`
- `EndpointPlanInvalid`
- `LifecycleInvalid`
- `TemplateRenderInvalid`
- `Critical`

Fabric core is pure and deterministic.

## Acknowledgment Rules

### CLI `start`

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| invalid args | Resolve | `InvalidInvocation` at process boundary |
| daemon IPC `SocketBindFailed` | Transform | `StartFailed{phase:"ipc"}` |
| daemon IPC `HealthCheckFailed` | Transform | `StartFailed{phase:"health"}` |
| daemon IPC `DispatchFailed` | Transform | `StartFailed{phase:"boot"}` |
| daemon IPC `Critical` | Transform | CLI `Critical` with origin chain |
| control-plane `BootFailed` | Transform | `StartFailed{phase:"boot"}` |
| control-plane `RouteRuntimeUnavailable` | Transform | `StartFailed{phase:"route_runtime"}` |
| control-plane `StoreUnavailable` | Transform | `StartFailed{phase:"store"}` |
| control-plane `CorruptState` | Transform | `StartFailed{phase:"recovery"}` |
| control-plane `RecoveryConflict` | Transform | `StartFailed{phase:"recovery"}` |
| control-plane `Critical` | Transform | CLI `Critical` with origin chain |
| unknown | Transform | CLI `Critical` |

### CLI `up`

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| invalid args | Resolve | `InvalidInvocation` at process boundary |
| orchestrator `ConfigInvalid` | Transform | `UpFailed{phase:"config"}` |
| orchestrator `ClaimRejected` | Transform | `UpFailed{phase:"claim"}` |
| orchestrator `PreflightRejected` | Transform | `UpFailed{phase:"preflight"}` |
| orchestrator `DependencyStartFailed` | Transform | `UpFailed{phase:"dependency"}` |
| orchestrator `ReadinessFailed` | Transform | `UpFailed{phase:"readiness"}` |
| orchestrator `GenerationCommitFailed` | Transform | `UpFailed{phase:"publish"}` |
| orchestrator `CleanupFailed` | Transform | `UpFailed{phase:"cleanup"}` |
| orchestrator `Critical` | Transform | CLI `Critical` with origin chain |
| unknown | Transform | CLI `Critical` |

Claim conflicts must reach CLI as `UpFailed{phase:"claim", cause: HostClaimConflict}`, `UpFailed{phase:"claim", cause: ListenerClaimConflict}`, or `UpFailed{phase:"claim", cause: RouteNamespaceClaimConflict}` with owner details intact.

Auxiliary publication degradation is not an `up` failure in this RFC. When core local publication succeeds and the daemon returns `PublicationDegraded` status, CLI `up` resolves it at the process boundary as successful output plus a warning/status detail. A future "required auxiliary surface" product mode must add a separate RFC with a declared error path before implementation.

### Daemon IPC/API Server Consumes Control-Plane Service

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| invalid request payload | Resolve | daemon IPC `ProtocolDecodeFailed` |
| handler dispatch failure before service call | Transform | daemon IPC `DispatchFailed` |
| control-plane `BootFailed` | Propagate | unchanged control-plane `BootFailed` |
| control-plane `HostClaimConflict` | Propagate | unchanged control-plane `HostClaimConflict` |
| control-plane `ListenerClaimConflict` | Propagate | unchanged control-plane `ListenerClaimConflict` |
| control-plane `RouteNamespaceClaimConflict` | Propagate | unchanged control-plane `RouteNamespaceClaimConflict` |
| control-plane `LeaseRejected` | Propagate | unchanged control-plane `LeaseRejected` |
| control-plane `PublicationRejected` | Propagate | unchanged control-plane `PublicationRejected` |
| control-plane `StaleTokenRejected` | Propagate | unchanged control-plane `StaleTokenRejected` |
| control-plane `RecoveryConflict` | Propagate | unchanged control-plane `RecoveryConflict` |
| control-plane `RouteRuntimeUnavailable` | Propagate | unchanged control-plane `RouteRuntimeUnavailable` |
| control-plane `StoreUnavailable` | Propagate | unchanged control-plane `StoreUnavailable` |
| control-plane `CorruptState` | Propagate | unchanged control-plane `CorruptState` |
| control-plane `Critical` | Propagate | unchanged control-plane `Critical` |
| control-plane `PublicationDegraded` status | Propagate | unchanged control-plane status in successful response |
| unknown | Transform | daemon IPC `Critical` |

### Control-Plane Service Consumes Store

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| store `OpenFailed` | Transform | `StoreUnavailable` |
| store `LockHeld` during startup lock acquisition | Transform | `BootFailed` |
| store `LockHeld` during recovery/reattach | Transform | `RecoveryConflict` |
| store `ReadFailed` | Transform | `StoreUnavailable` |
| store `WriteFailed` | Transform | `StoreUnavailable` |
| store `CorruptState` | Transform | control-plane `CorruptState` |
| store `AtomicCommitFailed` during publication journal commit | Transform | `PublicationRejected` |
| store `AtomicCommitFailed` during non-publication state write | Transform | `StoreUnavailable` |
| store `Critical` | Transform | control-plane `Critical` with origin chain |
| unknown | Transform | control-plane `Critical` |

### Control-Plane Service Consumes Route Runtime

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| route `BootstrapFailed` | Transform | `RouteRuntimeUnavailable` |
| route `ReloadFailed` | Transform | `RouteRuntimeUnavailable` |
| route `ApplyRejected` | Transform | `PublicationRejected` |
| route `UnsupportedTransport` | Transform | `PublicationRejected` |
| route `Critical` | Transform | control-plane `Critical` with origin chain |
| unknown | Transform | control-plane `Critical` |

### Control-Plane Service Consumes Auxiliary Publication

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| aux `SharedDomainPublishFailed` | Transform | `PublicationDegraded` status |
| aux `MetadataRenderFailed` | Transform | `PublicationDegraded` status |
| aux `MetadataCacheFailed` | Transform | `PublicationDegraded` status |
| aux `Critical` after core local publish committed | Transform | `PublicationDegraded` status with origin chain |
| unknown auxiliary-phase failure after core local publish committed | Transform | `PublicationDegraded` status with origin chain |

Auxiliary calls are forbidden before the core local generation is committed. If a future product mode introduces pre-commit auxiliary requirements, it must add separate request-failure errors and acknowledgment rows before implementation.

### Control-Plane Originated Outcomes

These are not consumed lower-layer errors and are therefore not `Resolve` in the traced-TDD sense.

| Originated condition | Emitted error/status |
| --- | --- |
| duplicate host claim | `HostClaimConflict` |
| duplicate listener claim | `ListenerClaimConflict` |
| duplicate route namespace claim | `RouteNamespaceClaimConflict` |
| stale token | `StaleTokenRejected` |
| lease mismatch | `LeaseRejected` |
| publication generation conflict | `PublicationRejected` |
| auxiliary publication failure after core local publish succeeds | `PublicationDegraded` status |
| ambiguous recovery evidence | `RecoveryConflict` |

The control-plane client propagates these identities unchanged to the stack orchestrator when they are request failures. `PublicationDegraded` is not a request failure; it is returned in status or warnings after the core local publication has already succeeded.

### Control-Plane Client

| Consumed remote/control-plane error | Decision | Becomes |
| --- | --- | --- |
| transport unavailable | Transform | `ConnectionFailed` |
| daemon IPC `HealthCheckFailed` | Transform | `ConnectionFailed` |
| daemon IPC `ProtocolDecodeFailed` | Transform | client `Critical` with origin chain |
| daemon IPC `DispatchFailed` | Transform | client `Critical` with origin chain |
| daemon IPC `Critical` | Transform | client `Critical` with origin chain |
| remote `HostClaimConflict` | Propagate | unchanged control-plane `HostClaimConflict` |
| remote `ListenerClaimConflict` | Propagate | unchanged control-plane `ListenerClaimConflict` |
| remote `RouteNamespaceClaimConflict` | Propagate | unchanged control-plane `RouteNamespaceClaimConflict` |
| remote `LeaseRejected` | Propagate | unchanged control-plane `LeaseRejected` |
| remote `PublicationRejected` | Propagate | unchanged control-plane `PublicationRejected` |
| remote `StaleTokenRejected` | Propagate | unchanged control-plane `StaleTokenRejected` |
| remote `RecoveryConflict` | Propagate | unchanged control-plane `RecoveryConflict` |
| remote `StoreUnavailable` | Propagate | unchanged control-plane `StoreUnavailable` |
| remote `CorruptState` | Propagate | unchanged control-plane `CorruptState` |
| remote `RouteRuntimeUnavailable` | Propagate | unchanged control-plane `RouteRuntimeUnavailable` |
| remote `Critical` | Transform | client `Critical` with origin chain |
| unknown transport/protocol error | Transform | client `Critical` |

Status propagation:

- remote `PublicationDegraded` status is decoded by the control-plane client as a successful response warning.
- the daemon IPC/API server owns encoding `PublicationDegraded` status into successful responses without changing its `(Layer, Code)` identity.
- the stack orchestrator carries that warning to CLI `up` without converting it to `GenerationCommitFailed`.
- CLI `up` renders the warning/status detail and exits zero.
- there is no error acknowledgment row for this path because no request failure is emitted.

### Stack Orchestrator Consumes Control-Plane Client And Service Errors

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| client `ConnectionFailed` before dependency start | Transform | `PreflightRejected` |
| client `ConnectionFailed` after dependency start | Transform | `GenerationCommitFailed` |
| control-plane `HostClaimConflict` | Transform | `ClaimRejected` |
| control-plane `ListenerClaimConflict` | Transform | `ClaimRejected` |
| control-plane `RouteNamespaceClaimConflict` | Transform | `ClaimRejected` |
| control-plane `LeaseRejected` | Transform | `GenerationCommitFailed` |
| control-plane `PublicationRejected` | Transform | `GenerationCommitFailed` |
| control-plane `StaleTokenRejected` | Transform | `GenerationCommitFailed` |
| control-plane `RecoveryConflict` | Transform | `GenerationCommitFailed` |
| control-plane `StoreUnavailable` | Transform | `GenerationCommitFailed` |
| control-plane `CorruptState` | Transform | `GenerationCommitFailed` |
| control-plane `RouteRuntimeUnavailable` | Transform | `GenerationCommitFailed` |
| client `Critical` | Transform | orchestrator `Critical` with origin chain |
| control-plane `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

Orchestrator tests for claim and publish failures must assert cleanup/release behavior at this layer.

### Stack Orchestrator Consumes Fabric Core

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| core `ConfigInvalid` | Transform | orchestrator `ConfigInvalid` |
| core `IdentityInvalid` | Transform | `PreflightRejected` |
| core `NamespaceInvalid` | Transform | `PreflightRejected` |
| core `EndpointPlanInvalid` | Transform | `PreflightRejected` |
| core `LifecycleInvalid` | Transform | `PreflightRejected` |
| core `TemplateRenderInvalid` | Transform | `ConfigInvalid` |
| core `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

### Stack Orchestrator Consumes Compose Backend

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| Compose `ModelInvalid` | Transform | `PreflightRejected` |
| Compose `IsolationRejected` | Transform | `PreflightRejected` |
| Compose `LifecycleFailed` | Transform | `DependencyStartFailed` |
| Compose `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

### Stack Orchestrator Consumes Process Launcher

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| process `SpawnFailed` | Transform | `DependencyStartFailed` |
| process `ExitedEarly` | Transform | `ReadinessFailed` |
| process `ShutdownFailed` | Transform | `CleanupFailed` |
| process `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

### Stack Orchestrator Consumes Readiness Probe

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| readiness `ProbeInvalid` | Transform | `PreflightRejected` |
| readiness `Timeout` | Transform | `ReadinessFailed` |
| readiness `ProbeFailed` | Transform | `ReadinessFailed` |
| readiness `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

### Stack Orchestrator Consumes Socket Bridge

| Consumed error | Decision | Becomes |
| --- | --- | --- |
| socket `BridgeInvalid` | Transform | `PreflightRejected` |
| socket `BridgeStartFailed` | Transform | `DependencyStartFailed` |
| socket `BridgeHealthFailed` | Transform | `ReadinessFailed` |
| socket `BridgeStopFailed` | Transform | `CleanupFailed` |
| socket `Critical` | Transform | orchestrator `Critical` with origin chain |
| unknown | Transform | orchestrator `Critical` |

## Test Ownership Matrix

| Layer | Test style | Real dependencies allowed | Must fake |
| --- | --- | --- | --- |
| CLI | subprocess or command harness | filesystem temp dir, fake daemon socket | Docker, Caddy, Compose, process internals |
| system acceptance | built binary | temp `ZEROBASED_HOME`, real daemon process/socket, fake route runtime, short-lived local process | Docker, real Caddy, real Compose unless explicitly testing Compose |
| daemon IPC/API server | socket/API server tests | temp socket dir, fake control-plane service | store, route runtime, Compose, child processes |
| control-plane service | in-process service tests | fake clock, fake store, fake route runtime | Docker, Caddy, Compose, child processes |
| control-plane store | temp-filesystem adapter tests | temp dir, file locks, real filesystem atomic rename | daemon process, route runtime, Compose |
| control-plane client | socket/API client tests | fake daemon socket/API | control-plane internals |
| stack orchestrator | in-process orchestration tests | temp project files | daemon, Compose, process launcher, probes |
| route runtime adapter | adapter tests | fake admin endpoint or embedded runtime fixture | control plane, Compose, processes |
| Compose backend adapter | adapter tests with fixtures | `compose-go` parser fixtures | CLI, control plane, child app processes |
| process launcher | adapter tests | short-lived test process | control plane, route runtime, Compose |
| readiness probe | adapter tests | loopback listener/test server | control plane, Compose |
| socket bridge | adapter tests | loopback target, temp socket dir | control plane, Compose |
| auxiliary publication | adapter tests | temp metadata cache, fake domain publisher | core local route runtime |
| fabric core | pure unit tests | none | all external systems |

System acceptance fake route runtime seam:

- the daemon owns adapter selection; CLI `up` and repo config cannot bypass it.
- tests may start the built daemon with `ZEROBASED_ROUTE_RUNTIME_BACKEND=admin-url` and `ZEROBASED_ROUTE_RUNTIME_ADMIN_URL=<fake-admin-url>`.
- the fake admin endpoint must exercise the same route-runtime adapter request/response contract as an external route runtime.
- hidden test-only branches inside CLI or stack orchestration are not allowed.

Static capability boundary:

- owner: C3 component `rewrite-guardrails`.
- artifact: `docs/superpowers/specs/zerobased-capabilities.yaml`.
- type: static architecture test input, not a runtime layer.
- output: test failure on forbidden imports, commands, or symbols; no product typed error is required because no user runtime path consumed it.
- schema fields: `version`, `schema`, `owner.c3_component`, `owner.rfc`, `capabilities[].id`, `capabilities[].allowed_packages`, `capabilities[].allowed_imports`, `capabilities[].forbidden_imports`, `capabilities[].forbidden_symbols`, `capabilities[].allowed_by_rfc`.

## Required Test List Before Behavior Lands

Machine-checkable error registry:

- every declared error/status `(Layer, Code)` pair is unique.
- every reused error code name has an explicit propagation or transform rule.
- every layer has a `Critical` code.
- every `Critical` preserves origin chain.
- every acknowledgment row has at least one test id in the implementation plan.
- conditional rows are forbidden; if one consumed error can produce different outcomes, the RFC must split it into one row per `(consumed error, condition, outcome)`.
- the implementation plan must include an acknowledgment coverage table that names every row in this RFC and the exact test that covers it.

Import and command boundary checks:

- no `zerobased run` command is registered.
- capability allowlist rejects Docker event watching unless an explicit future RFC adds it to the target graph.
- capability allowlist rejects routefile parsing unless an explicit future RFC adds it to the target graph.
- capability allowlist rejects classifier/env discovery unless an explicit future RFC adds it to the target graph.
- capability allowlist rejects direct Caddy/admin mutation from repo-local `up`.
- repo-local `up` cannot import the route runtime adapter directly.
- Compose default files are ignored unless declared in `zerobased.yaml`: `compose.yaml`, `compose.yml`, `docker-compose.yml`, and `docker-compose.yaml`.
- directory scanning for Compose files is not allowed before `zerobased.yaml` declares a Compose backend.

System acceptance:

- built binary `start` creates daemon lock/socket/health under temp `ZEROBASED_HOME`.
- second `start` reports existing owner/process instead of starting another daemon.
- stale lock is recovered or reported with an owner detail.
- built binary `start` with occupied daemon socket exits through `StartFailed{phase:"ipc"}`.
- built binary `start` with unhealthy daemon exits through `StartFailed{phase:"health"}`.
- built binary `up` claims before starting a short-lived local process.
- built binary `up` commits publication only after readiness.
- built binary `up` conflict output includes current owner details.
- built binary `up` route-namespace conflict output includes current owner details.
- built binary `up` exits zero with a warning/status detail when only auxiliary publication is degraded.
- built binary `up` cleans up on signal.
- built binary `up` cleans up on startup failure.

CLI:

- `start` invalid invocation resolves to `InvalidInvocation`.
- `start` control-plane boot failure becomes `StartFailed{phase:"boot"}`.
- `up` invalid invocation resolves to `InvalidInvocation`.
- `up` claim conflict renders owner details and exits non-zero.
- `up` route-namespace conflict renders owner details and exits non-zero.
- `up` auxiliary publication degradation renders warning/status detail and exits zero.
- `up` stale token rejection exits non-zero.
- `up` readiness failure exits non-zero with phase `readiness`.
- CLI `Critical` renders origin chain.

Control-plane service:

- first host claimant wins.
- host conflict returns existing owner details.
- listener claim canonicalization catches equivalent bind addresses.
- route-namespace conflict returns existing owner details.
- stale claim token is rejected.
- older publication generation cannot overwrite newer generation.
- failed replacement publish preserves last committed local generation.
- ambiguous lease loss withdraws publication and reserves identity.
- provably dead lease loss releases identity.
- auxiliary publication failure marks `PublicationDegraded` in status without rejecting core local publication.
- auxiliary `Critical` after core local publish marks `PublicationDegraded` in status with origin details.
- `who` and `status` read from daemon state.
- route runtime apply failure maps to `PublicationRejected`.
- auxiliary publication adapter failures map to `PublicationDegraded` status after core local publication succeeds.
- unknown store/runtime failure wraps to `Critical`.

Daemon IPC/API server:

- socket bind failure maps to `SocketBindFailed`.
- health endpoint failure maps to `HealthCheckFailed`.
- malformed request maps to `ProtocolDecodeFailed`.
- dispatch failure before service call maps to `DispatchFailed`.
- service errors are encoded without changing their `(Layer, Code)` identity.
- `PublicationDegraded` status is encoded in successful responses without changing its `(Layer, Code)` identity.
- unknown server failure wraps to daemon IPC `Critical`.

Control-plane store:

- lock acquisition succeeds once.
- second lock reports `LockHeld`.
- corrupt state reports `CorruptState`.
- atomic write failure reports `AtomicCommitFailed`.
- restart rehydrates owner records.

Control-plane client:

- transport unavailable maps to `ConnectionFailed`.
- daemon IPC health failure maps to `ConnectionFailed`.
- daemon IPC protocol failure maps to client `Critical`.
- remote control-plane request errors preserve original `(Layer, Code)` identity.
- `PublicationDegraded` status is decoded as a successful response warning, not an error.

Stack orchestrator:

- config invalid stops before control-plane claim.
- repo with only `compose.yaml` fails before claim/start.
- repo with only `compose.yml` fails before claim/start.
- repo with only `docker-compose.yml` fails before claim/start.
- repo with only `docker-compose.yaml` fails before claim/start.
- repo directory scanning does not discover Compose files without explicit `zerobased.yaml` backend declaration.
- explicit `zerobased.yaml` is required before planning/orchestration.
- claim happens before Compose or process start.
- all publishable claims are preflighted before side effects.
- claim conflict becomes `ClaimRejected` and starts no dependencies.
- route-namespace conflict becomes `ClaimRejected` and starts no dependencies.
- `PublicationDegraded` warning is carried to CLI without becoming `GenerationCommitFailed`.
- Compose start failure releases claim and stops owned resources.
- process spawn failure releases claim and stops started dependencies.
- readiness timeout stops owned resources and releases claim.
- publish failure preserves cleanup ordering.
- cleanup failure becomes `CleanupFailed`.
- lower `Critical` preserves origin chain.

Adapters:

- route runtime desired-state apply succeeds.
- route runtime apply rejection maps to `ApplyRejected`.
- route runtime unsupported L4 maps to `UnsupportedTransport`.
- Compose rejects unresolved global names.
- Compose derives scoped project name.
- Compose lifecycle failure maps to `LifecycleFailed`.
- process launcher maps spawn failure.
- process launcher maps early exit.
- process launcher sends shutdown signal.
- readiness probe maps TCP timeout.
- readiness probe maps HTTP non-ready.
- socket bridge maps health failure.
- auxiliary metadata cache failure maps to `MetadataCacheFailed`.

Fabric core:

- project root basename produces default hostname.
- explicit hostname override wins.
- invalid host is rejected.
- route path precedence is deterministic.
- ambiguous route overlap is rejected.
- listener claim inputs canonicalize deterministically.
- endpoint plan rejects unsupported delivery.
- lifecycle rejects invalid transitions.

## Current Repo Mapping

The working tree has been cleaned for rewrite.

Current code:

- `cmd/zerobased/main.go`: minimal rewrite notice stub.
- `cmd/zerobased/main_test.go`: confirms the notice names rewrite state and first-class boundaries.

Removed from executable source:

- legacy Docker watcher
- Docker client wrapper
- classifier
- routefile parser
- env generator
- Caddy manager
- socat bridge
- `run` wrapper
- old `up` runtime
- fabric seed implementation

Current docs and scaffolding:

- `README.md`: rewrite state
- `CLAUDE.md`: rewrite guardrails for Claude
- `AGENTS.md`: rewrite guardrails for Codex/agents
- `skills/zerobased/SKILL.md`: rewrite-state skill stub
- C3 codemap: reduced to rewrite scaffolding and 100% mapped

## Migration Rules

1. Do not restore old code wholesale.
2. Restore a legacy concept only by reintroducing it behind a target layer contract from this RFC.
3. Each new package must belong to one layer in the graph.
4. Each layer must declare typed errors before behavior tests are written.
5. Each typed error must have an acknowledgment decision in the immediate consumer.
6. Every acknowledgment must have a test at the owning layer.
7. Catch-all handling is allowed only at a layer boundary and must wrap into `Critical`.
8. Tests for upper layers must use fakes for lower layers.
9. No test may assert lower-layer implementation details through an upper-layer mock.
10. Each implementation slice must create or update the C3 component and codemap entry for the layer it touches.
11. Each slice must run `c3x lookup` for touched files, then `c3x check`, `c3x coverage`, and the relevant Go tests.
12. The implementation plan must give every acknowledgment row a test id before behavior for that layer lands.
13. Acknowledgment coverage is mechanical: the implementation plan must include a table that maps every acknowledgment row in this RFC to one named test before that layer can be implemented.
14. If a consumed error needs condition-specific behavior, split the acknowledgment into separate rows before implementation.

## Acceptance Criteria

This RFC is accepted when:

- subagent review finds no medium-or-higher issues in the graph, error contracts, and test ownership
- C3 check passes
- C3 coverage remains 100%
- `go test ./...` passes
- `make build` passes

Mechanical acceptance for implementation work:

- error registry uniqueness test exists before layer behavior lands
- capability allowlist and command boundary tests exist before behavior lands
- red system acceptance harness exists before `start` or `up` is implemented as real behavior
- C3 component and codemap updates are part of every implementation slice

After acceptance, the implementation plan must follow this order:

1. C3 topology for target layers
2. typed error kernel and registry uniqueness tests
3. capability allowlist and command boundary tests
4. red system acceptance harness for `start` and `up`
5. fabric core tests and implementation
6. control-plane store tests and implementation
7. route runtime adapter/bootstrap tests and implementation
8. auxiliary publication adapter tests and implementation
9. control-plane service tests and in-memory implementation
10. daemon IPC/API server tests and implementation
11. `zerobased start` CLI tests against fake service, then real daemon boot path
12. control-plane client tests and implementation
13. readiness probe adapter tests and implementation
14. socket bridge adapter tests and implementation
15. stack orchestrator with fake control-plane client and fake adapters
16. `zerobased up` fake-daemon CLI tests
17. process launcher adapter tests and implementation
18. Compose backend adapter tests and implementation
19. black-box system acceptance expansion for full `start` and `up`
20. acountee live smoke test
