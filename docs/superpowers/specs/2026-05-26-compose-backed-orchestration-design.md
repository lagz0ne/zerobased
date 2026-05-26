# Zerobased Compose-Backed Orchestration Design

Date: 2026-05-26
Status: Draft
Depends on:

- `docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md`
- `docs/superpowers/specs/2026-05-13-zerobased-traced-tdd-rfc.md`

## Purpose

This document defines how `zerobased up` orchestrates containerized project dependencies through Docker Compose without returning to legacy Docker autodiscovery.

Compose is a backend. The user interface remains explicit `zerobased.yaml` v1.

## Primary Sources

- `compose-go` is the reference Go library for parsing and loading Compose files: https://github.com/compose-spec/compose-go
- `compose-go/v2/loader.Options` supports profiles and known `x-*` extensions: https://pkg.go.dev/github.com/compose-spec/compose-go/v2/loader
- `compose-go/v2/types.Project` exposes project transforms such as `WithProfiles`, `WithSelectedServices`, `WithServicesTransform`, and `WithoutUnnecessaryResources`: https://pkg.go.dev/github.com/compose-spec/compose-go/v2/types
- Docker Compose v2 exposes a Go `api.Compose` interface with lifecycle methods including `Create`, `Start`, `Up`, `Down`, `Ps`, `Logs`, `Wait`, and `Port`: https://pkg.go.dev/github.com/docker/compose/v2/pkg/api
- Docker docs define Compose as services, networks, volumes, configs, and secrets; `version` is obsolete in Compose files, while `name` controls default project naming: https://docs.docker.com/reference/compose-file/
- Docker docs define `x-*` extensions as ignored by Compose and usable for tool metadata: https://docs.docker.com/reference/compose-file/extension/

## Runtime Graph

Graph: https://diashort.apps.quickable.co/d/abe35e03

Sequence: https://diashort.apps.quickable.co/d/204b3741

## Product Contract

`zerobased up` owns one stack invocation in the foreground.

The stack can include:

- local processes from `processes`
- containerized dependencies from `compose`
- routes from `routes`
- readiness probes from both local processes and Compose-backed services
- daemon-owned host, route, and listener claims

The daemon owns:

- global hostname claims
- route namespace claims
- listener claims
- owner records
- route runtime mutation

The Compose backend owns:

- loading declared Compose files
- validating the Compose model against zerobased integrity rules
- injecting zerobased runtime identity into the in-memory model
- creating, starting, stopping, and inspecting Compose resources

`zerobased up` still owns the orchestration sequence:

1. find project root
2. load `zerobased.yaml` v1
3. select profile
4. build stack plan
5. load and validate Compose model when declared
6. compute required daemon claims
7. claim before starting resources
8. start Compose dependencies
9. start local processes
10. wait for declared readiness
11. publish one route generation through the daemon
12. stay foreground
13. on stop or failure, cleanup owned resources and release claims

## Configuration Shape

`zerobased.yaml` remains the only entry point. It must start with `version: 1`.

Example for `~/dev/acountee`:

```yaml
version: 1
name: acountee
host: acountee.localhost
profile: backend

profiles:
  backend:
    compose:
      files:
        - compose.yaml
      profiles:
        - backend
      services:
        - postgres
        - nats
        - otel-collector
      ownership: owned
      readiness:
        postgres:
          type: tcp
          port: 5432
        nats:
          type: tcp
          port: 4222
        otel-collector:
          type: http
          port: 13133
          path: /healthz
      endpoints:
        postgres:
          service: postgres
          port: 5432
          protocol: tcp
        nats:
          service: nats
          port: 4222
          protocol: tcp
        otel-grpc:
          service: otel-collector
          port: 4317
          protocol: tcp

processes:
  web:
    command: ["bun", "run", "dev"]
    readiness:
      type: http
      port: 3000
      path: /healthz

routes:
  - path: /
    process: web
    port: 3000
```

The `profile` key selects a profile by name. If absent, `dev` is selected when present; otherwise the first implementation can reject with `ConfigInvalid` until CLI profile selection is implemented.

Profiles are stack variants, not safety policy. They are for convenience:

- `frontend`: commonly uses shared or reused facilities
- `backend`: commonly uses owned dependencies
- `testing`: commonly uses owned ephemeral dependencies

Zerobased does not judge whether using staging data is wise. It only starts exactly what was declared, claims what it will expose, and fails when declared ownership is impossible.

## Compose File Role

The user Compose file is a base model.

It is not the source of zerobased ownership. It describes containers, networks, volumes, env, and service relationships.

Zerobased then creates an effective runtime model:

- project name is generated from canonical stack identity
- service labels get zerobased owner/session metadata under the `dev.zerobased.*` prefix
- route/listener exposure is derived from `zerobased.yaml`, not hidden Compose port scanning
- unsupported global escape hatches are rejected before containers are created

No YAML string-bashing is allowed for semantic changes. The implementation loads the model into `types.Project`, transforms typed services/resources, and passes the effective project to the Compose API.

First implementation policy: reject, do not rewrite, any global escape hatch. Rewriting can come only after there is a typed contract and tests proving the rewritten model is equivalent and explainable.

## Runtime Identity

Canonical stack identity:

- project root path
- selected zerobased profile
- zerobased stack name
- canonical host
- session id
- claim token
- publication generation

Generated Compose project name:

```text
zb_<sanitized-project-name>_<profile>_<short-hash(project-root)>
```

Rules:

- Compose project name is a backend namespace, not the human owner identity.
- Human diagnostics use repo path, profile, host, and stack name.
- The generated Compose project name must be deterministic for the same root/profile but distinct across different roots/profiles.
- User top-level Compose `name` is ignored or overridden by the generated project name.
- Shell `COMPOSE_PROJECT_NAME`, `.env`, and Compose default directory naming must not decide backend identity.
- The generated Compose project name is passed explicitly into the loader/backend path.
- Zerobased owner/session labels are injected into every service before lifecycle operations.
- Zerobased must not write `com.docker.compose.*` labels; that prefix is reserved for Docker Compose.
- The generated Compose project name and labels are stored in the owner record as backend recovery proof.
- If Compose interpolation depends on `COMPOSE_PROJECT_NAME`, the generated value is the value users get. That is intentional because zerobased owns backend identity.

## Ownership Modes

### `owned`

Default and first implementation target.

Behavior:

- zerobased creates/starts the Compose project
- zerobased labels owned resources
- zerobased stops/removes the owned project on foreground exit or startup failure
- named volumes are kept or removed according to explicit config, not guessed

Default cleanup:

- containers and networks are removed
- named volumes are preserved
- anonymous volumes follow Compose default behavior unless `remove_volumes: true` is declared

### `shared`

Product mode for intentional cross-stack facilities.

Behavior:

- sharing requires an explicit compatibility fingerprint
- stacks can share only when the fingerprint matches
- daemon persists reference ownership
- teardown happens only when the last compatible live stack releases

Not first implementation unless the typed owner store is implemented in the same slice.

### `reused`

Product mode for preexisting dependencies, including staging-like data.

Behavior:

- zerobased does not create or stop the dependency
- config declares endpoint coordinates
- readiness can probe the endpoint
- published routes/listeners still require daemon claims if zerobased exposes anything

This is the explicit way to say "use what is already there."

## Compose Integrity Rules

Zerobased must fail before resource creation when a Compose model escapes owned stack identity.

Reject by default:

- `container_name`
- `network_mode: host`
- service `ports` that publish host ports outside zerobased listener claims
- top-level networks with `name` because Docker uses the name as-is
- top-level volumes with `name` because Docker uses the name as-is
- `external: true` networks or volumes in `owned` mode
- `pid: host`, `ipc: host`, or similar host namespace sharing when it creates machine-global side effects
- provider-managed services unless explicitly modeled as `reused`

Allow by default:

- `expose`
- internal service ports
- unnamed/default Compose networks
- project-scoped named volumes without custom `name`
- bind mounts, because they are explicit filesystem side effects the user declared
- `x-*` metadata

Policy:

- no automatic fallback names
- no automatic port remapping
- no best-effort repair
- fail fast with the field path and reason

Example failure:

```text
zerobased up: preflight rejected
compose service postgres uses container_name: acountee-postgres
reason: container_name is machine-global and bypasses zerobased stack identity
fix: remove container_name or move this dependency to ownership: reused
```

## Route And Listener Exposure

Compose service ports do not become routes automatically.

Publication is explicit:

- HTTP routes use `routes`
- TCP endpoints use `compose.endpoints` plus daemon listener/socket policy
- local process routes continue to point at a named process and port

For the first Compose slice, TCP endpoints can remain internal connection metadata until the route runtime has real TCP/L4 support.

If a Compose-backed endpoint must be reachable from the host through zerobased, the daemon must claim the listener before Compose starts.

## Readiness

Readiness is declared in `zerobased.yaml`.

Supported probe types for Compose-backed services:

- `tcp`: connect to service container port through the Compose project network or inspected container address
- `http`: HTTP request to declared path
- `command`: optional later only if the Compose backend exposes an exec-safe adapter with timeout

Readiness must not be inferred from Docker healthchecks in the first implementation. Docker healthchecks can be added as a source later, but zerobased readiness remains the explicit product contract.

## Lifecycle

### Startup

1. load config
2. load Compose model
3. validate model
4. inject identity
5. compute claims
6. claim through daemon
7. start Compose
8. start local processes
9. wait readiness
10. publish generation

### Foreground

`zerobased up` stays attached.

It exits when:

- user sends interrupt
- a managed local process exits unexpectedly
- a managed Compose dependency exits in a way the selected mode treats as fatal
- the daemon rejects publication or release

### Cleanup

On startup failure before publication:

- stop local processes already started
- run Compose cleanup for owned stack if Compose was started
- release daemon claim

On normal interrupt:

- unpublish/release daemon claim
- stop local processes
- stop/remove owned Compose project

Cleanup errors become `stack_orchestrator:CleanupFailed`.

## Traced-TDD Ownership

New layer:

- `compose_backend`

Declared errors:

- `ModelInvalid`
- `IsolationRejected`
- `LifecycleFailed`
- `Critical`

Acknowledgment table:

| Consumed error | Decision | Becomes | Owning test layer |
|---|---|---|---|
| Compose parse/load error | Transform | `compose_backend:ModelInvalid` | Compose backend |
| invalid zerobased compose config | Transform | `compose_backend:ModelInvalid` | Compose backend |
| unsupported global field | Transform | `compose_backend:IsolationRejected` | Compose backend |
| Docker Compose API create/start/down failure | Transform | `compose_backend:LifecycleFailed` | Compose backend |
| unknown panic/error in Compose adapter | Transform | `compose_backend:Critical` | Compose backend |
| `compose_backend:ModelInvalid` consumed by stack orchestrator | Transform | `stack_orchestrator:PreflightRejected` | Stack orchestrator |
| `compose_backend:IsolationRejected` consumed by stack orchestrator | Transform | `stack_orchestrator:PreflightRejected` | Stack orchestrator |
| `compose_backend:LifecycleFailed` consumed before readiness | Transform | `stack_orchestrator:DependencyStartFailed` | Stack orchestrator |
| `compose_backend:LifecycleFailed` consumed during cleanup | Transform | `stack_orchestrator:CleanupFailed` | Stack orchestrator |

Test ownership:

- Compose backend tests use fixture Compose files and fake Compose API where possible.
- Stack orchestrator tests use a fake Compose backend, not Docker.
- System tests with real Docker/Compose are build-tagged and not required for every unit run.

## Implementation Slices

### Slice 1: Owned Compose Backend Contract

Files likely affected:

- `internal/composebackend`
- `internal/up`
- `internal/zberr`
- `docs/superpowers/specs/zerobased-acknowledgment-coverage.yaml`

Tests first:

- config rejects `compose` unless `version: 1` is first key
- declared Compose file is required to exist
- undeclared `compose.yaml` is ignored
- shell `COMPOSE_PROJECT_NAME`, `.env`, Compose top-level `name`, and directory defaults do not control generated project identity
- loader rejects `container_name`
- loader rejects `ports` with host publication
- loader rejects `network_mode: host`
- loader rejects custom top-level network and volume names
- loader rejects `external` networks and volumes in `owned`
- loader rejects provider-managed services unless explicitly modeled as `reused`
- loader injects generated project name and labels
- stack orchestrator claims before Compose start
- stack orchestrator starts Compose before local processes when local processes depend on services
- stack orchestrator cleans Compose on startup failure
- stack orchestrator releases claim after Compose cleanup

### Slice 2: Readiness And Endpoint Metadata

Tests first:

- TCP readiness waits for declared Compose service port
- HTTP readiness waits for declared path
- endpoint metadata is generated from declared endpoints
- endpoint metadata is not generated by scanning all exposed ports

### Slice 3: Real Compose Integration

Tests first:

- build-tagged integration test starts `postgres`, `nats`, or a minimal HTTP container from a fixture Compose file
- interrupt stops/removes owned project containers
- rejected global fields fail before any container is created

## Operational Readiness

State needed under `~/.zerobased`:

- stack owner record
- claim token and generation
- selected profile
- generated Compose project name
- Compose backend labels
- cleanup status
- last failure reason

Useful commands later:

- `zerobased ps`: show stack, profile, host, Compose project, owner path, state
- `zerobased who <host>`: show current owner
- `zerobased logs <stack>`: stream process and Compose logs through owner identity
- `zerobased down`: explicit cleanup for current project/profile

## Troubleshooting

### Claim conflict

Show:

- requested host/path/listener
- current owner path
- current profile
- current stack name
- current session if safe to show

Do not:

- invent another host
- start the stack anyway

### Compose isolation rejection

Show:

- Compose file path
- service/resource name
- exact field path
- reason
- one concrete fix

### Compose lifecycle failure

Show:

- generated Compose project name
- operation: create/start/up/down/ps
- underlying Compose error
- whether daemon claims were released

### Cleanup failure

Show:

- owned resources that may still exist
- generated Compose project name
- next command to inspect with Docker or future zerobased cleanup command

## Availability

This is a local single-user control plane.

Availability expectations:

- daemon restart should not corrupt owner records
- `up` should fail if daemon is unavailable before resources start
- after resources start, daemon publication failure must cleanup owned resources
- route runtime reload is daemon-owned and batched per publication generation

No distributed HA is in scope.

## Performance

Expected costs:

- Compose model load is O(size of declared Compose files)
- validation is O(number of services + networks + volumes + ports)
- startup time is dominated by image pull/build and container readiness
- route publication is one daemon call per stack generation

Rules:

- load Compose model once per `up` invocation
- avoid polling Docker for every service in a tight loop
- use bounded readiness intervals
- batch route publication after readiness
- do not reload the route runtime per service

## Covered

- Explicit Compose-backed dependencies from `zerobased.yaml`
- Owned Compose stack lifecycle
- Generated runtime project identity
- Fail-fast validation of global Compose escape hatches
- Foreground `zerobased up`
- Daemon-owned claims and routes
- Readiness before publication
- Cleanup on startup failure and interrupt

## Not Covered

- Automatic Compose file discovery
- Legacy Docker event watching
- Legacy `zerobased run`
- Automatic port scanning/classification
- Automatic hostname fallback
- Production deployment
- Multi-user host isolation
- Shared dependency reference counting before a persisted owner store exists
- Arbitrary TCP routing before Caddy L4 support is implemented

## Design Decisions

### Use Compose API, Not Shell Text

The Go API gives typed project lifecycle calls and avoids parsing CLI output.

Shelling out can remain a debugging fallback, not the product path.

### Reject Global Escape Hatches First

Fields like `container_name`, custom resource `name`, `external`, and host port bindings bypass stack identity.

Zerobased's integrity rule is stronger than convenience here. It fails before creating resources.

### Keep Compose Profiles Separate From Zerobased Profiles

Zerobased profile selects the whole stack variant.

Compose profiles select services inside the Compose model.

Both can exist:

- `zerobased profile`: user workflow shape
- `compose profiles`: Compose service selection

After selecting Compose profiles and services, zerobased prunes unused model resources before validation/lifecycle. Top-level networks, volumes, configs, and secrets are not themselves profile-scoped, so pruning is required to avoid validating or carrying resources that no active service uses.

### Do Not Infer Routes From Compose

Explicit routes keep the product explainable.

Compose `ports` and `expose` are container model details. Zerobased publication is a control-plane decision.

### Hide Compose SDK Version Churn

Docker currently documents newer Compose SDK module paths while the v2 API still exposes the lifecycle methods zerobased needs.

Zerobased must keep Docker Compose behind a narrow internal adapter:

- load/build effective project
- create/start/up
- down
- ps/logs/events when needed
- port lookup only for declared endpoints

No package outside the Compose backend should import Docker Compose SDK packages.
