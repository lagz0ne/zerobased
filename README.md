# zerobased

Explicit local dev stack orchestration.

`zerobased start` runs once per machine and owns the local control plane plus
route runtime. `zerobased up` runs from a project directory, reads explicit
`zerobased.yaml` v1, starts declared dependencies and processes, waits for
readiness, publishes routes, and stays foreground until Ctrl-C.

## Install

```bash
bun i -g zerobased
zerobased version
```

## Quick Start

Start the machine-local control plane in one terminal:

```bash
zerobased start
```

Create `zerobased.yaml` in the project root:

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
      path: .zerobased/web.ready
routes:
  - path: /
    process: web
    port: 3000
```

Run the project:

```bash
zerobased up
```

Open `http://my-project.localhost`.

## Config

`zerobased.yaml` must start with `version: 1`.

Use `compose.files` to list the Compose files zerobased may load. Compose files
are not discovered automatically. Use `compose.services` and `compose.profiles`
to select the declared dependency slice for this project.

In `owned` Compose mode, zerobased generates the Compose project identity and
adds `dev.zerobased.*` labels. It rejects `container_name`, host `ports`,
host networking, host PID/IPC/cgroup namespace sharing, provider-managed
services, external resources, and custom top-level resource names before
containers start.

Declare local processes under `processes`. Every process must declare file
readiness with `type: file` and `path`. Declare HTTP routes under `routes`,
pointing each route to a process and port.

Local processes published through the default Docker Caddy route runtime must
listen on an address reachable from Docker, normally `0.0.0.0`. A process bound
only to `127.0.0.1` cannot be reached from the Caddy container.

## Acountee-Style Example

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
      path: .zerobased/web.ready
routes:
  - path: /
    process: web
    port: 3000
```

Keep Compose ports internal. Publish through zerobased routes/listeners, not
Compose host port binds.

## Commands

```bash
zerobased start      # start the machine-local control plane
zerobased up         # run the current project from zerobased.yaml
zerobased version    # print the installed version
zerobased help       # show CLI help
```

## From Source

```bash
go test ./...
make build
./bin/zerobased help
```
