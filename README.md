# zerobased

Rewrite in progress.

The legacy Docker autodiscovery router has been removed from this working tree. This repo is now the starting point for the explicit machine-local control plane described in:

- `docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md`

Current implementation state:

- `zerobased start` and `zerobased up` are the only first-class product boundaries for new work.
- `zerobased up` reads explicit `zerobased.yaml` v1 from the project root.
- The old Docker watcher, `zerobased run`, routefile parser, classifier-based exposure, and direct Caddy mutation paths are not the target architecture.
- Runtime path: claim through the daemon, start declared Compose dependencies when present, launch declared local processes, wait for readiness, publish routes through the daemon-owned route runtime, then stay foreground until stopped.
- Compose is explicit. Zerobased loads only Compose files listed in `zerobased.yaml`; it does not infer `compose.yaml` from the directory.
- Owned Compose services get a zerobased-generated Compose project name and `dev.zerobased.*` labels. `container_name`, host `ports`, `network_mode: host`, external resources, and custom top-level resource names are rejected before containers start.

Minimal `zerobased.yaml` v1:

```yaml
version: 1
name: my-project
host: my-project.localhost
compose:
  files:
    - compose.yaml
  services:
    - postgres
  ownership: owned
processes:
  web:
    command: ["go", "run", "./cmd/web"]
    readiness:
      type: file
      path: /tmp/my-project-ready
routes:
  - path: /
    process: web
    port: 3000
```

Run it:

```bash
zerobased start
zerobased up
```

`up` is foreground. Stop it with Ctrl-C; zerobased owns orchestration, not detached babysitting.

Compose example:

```yaml
version: 1
name: acountee
host: acountee.localhost
profile: backend
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
processes:
  web:
    command: ["bun", "run", "dev"]
    readiness:
      type: file
      path: /tmp/acountee-web-ready
routes:
  - path: /
    process: web
    port: 3000
```

Keep Compose ports internal. Publish through zerobased routes/listeners, not Compose host port binds.

Development rule:

- No behavior should be added without RED-GREEN-TDD.
- Tests must follow traced-TDD: each failure belongs to one declared layer contract, and upper layers test only their own transformed or propagated outcomes.
- Repo-local `up` must publish to the daemon-owned control plane. It must not mutate Caddy directly.

Useful commands:

```bash
go test ./...
go test -tags system ./internal/systemtest
make build
```
