# zerobased

Zero-config Docker service router. Watches `docker.sock` for container events, auto-classifies ports, creates Unix sockets + HTTP routes. Cleans up when containers stop.

No config files. No Docker labels. No setup scripts. No external dependencies. Your existing `docker-compose.yml` works as-is.

## Install

```bash
npm install -g zerobased
```

Or build from source:

```bash
git clone https://github.com/lagz0ne/zerobased.git
cd zerobased && make install
```

## Quick start

```bash
# Start the daemon (background)
zerobased start -d

# Your existing workflow — unchanged
docker compose up -d

# See what was discovered
zerobased ps
```

Example output:

```
acountee:
  postgres        socket 5432 → postgresql://postgres@/postgres?host=~/.zerobased/sockets/acountee
  nats            http   80   → http://nats-80.acountee.localhost
  nats            http   8222 → http://nats-8222.acountee.localhost
  nats            port   4222 → localhost:26987
```

## CLI

```
zerobased [flags] <command> [args...]

Flags:
  -H, --docker-host <host>   Docker daemon socket
  --prefix <prefix>          Env var prefix (default: ZB; "" for no prefix)
  --profile <names>          Route profiles from zerobased.routes.yaml (comma-separated)

Commands:
  help [command]                           Show guided help
  version                                  Print build version
  start [-d]                               Start daemon (-d for background)
  stop                                     Stop daemon + cleanup
  logs [-f]                                Show daemon logs (-f to follow)
  run [-p port] [name] <cmd>               Wrap dev server, register route
  env [--export] [project]                 Print connection strings
  ps                                       Show all discovered services
  get <service> [-t template] [-v k=v]     Print one connection string
  domain add <domain> [--ttl 2h]           Add external domain (default TTL: 4h)
  domain add <domain> --persistent         Add domain that never expires
  domain list                              Show configured domains + TTL
  domain rm @N                             Remove domain by index
  share [@N]                               Show shareable URLs for all/one domain
  unshare @N | --all                       Remove domain(s) + deregister routes
```

Every command supports a guided help path:

```bash
zerobased help
zerobased help run
zerobased help domain
```

If you are driving zerobased from an LLM or automation, this sequence is the safest starting point:

```bash
zerobased start -d
docker compose up -d
zerobased ps
zerobased get <service>
```

### Daemon

```bash
zerobased start              # foreground (Ctrl+C to stop)
zerobased start -d           # background, auto-follows logs (Ctrl+C detaches)
zerobased logs -f            # follow daemon logs
zerobased stop               # stop + cleanup
```

### Wrapping dev servers

```bash
zerobased run acountee pnpm dev           # auto-detects port from stdout/stderr
zerobased run -p 3000 acountee pnpm dev   # explicit port
```

Port is auto-detected by scanning the child process output for `http://localhost:XXXX`. Works with Vite, Next, Nuxt, or any framework that prints its URL. Use `-p` if auto-detect doesn't work.

Also injects `ZB_*` env vars for all project services:

```
ZB_POSTGRES_5432=postgresql://postgres@/postgres?host=~/.zerobased/sockets/acountee
ZB_NATS_4222=localhost:26987
ZB_NATS_80=http://nats-80.acountee.localhost
```

### Path-based routing (routefile)

Drop a `zerobased.routes` file next to your `docker-compose.yml` to get a single gateway with path routing instead of one hostname per service:

```
# zerobased.routes
/api     api
/ws      ws
/        frontend
```

This creates `myapp.localhost/api`, `myapp.localhost/ws`, `myapp.localhost/` — each routing to the named docker-compose service. The gateway hostname is derived from your Compose project name.

No routefile = existing hostname-per-service behavior. Nothing breaks.

### Profiles & external upstreams

Use `zerobased.routes.yaml` for profiles with inheritance and external service targets:

```yaml
# zerobased.routes.yaml
profiles:
  default:
    routes:
      /api: api
      /ws: ws
      /: frontend

  debug:
    extends: [default]
    routes:
      /db: postgres://staging-db.example.com:5432

  staging:
    extends: [default]
    routes:
      /api: https://api.staging.example.com
      /ws: wss://ws.staging.example.com
```

```bash
zerobased start --profile debug          # local app + staging DB
zerobased start --profile staging,debug  # merge multiple profiles
```

Supported external targets: `https://`, `wss://`, `postgres://`, `nats://`, `redis://`. Profiles merge left-to-right with last-write-wins on conflicting paths.

### Connection strings

```bash
# Default
zerobased get postgres
# postgresql://postgres@/postgres?host=~/.zerobased/sockets/acountee

# With template + user vars
zerobased get postgres \
  -t 'postgresql://{{user}}:{{pass}}@/{{db}}?host={{socket_dir}}' \
  -v user=postgres -v pass=secret -v db=mydb

# Just the socket directory
zerobased get postgres -t '{{socket_dir}}'

# NATS with scheme
zerobased get nats -t 'nats://{{host}}:{{port}}'
```

Template variables: `project`, `service`, `container_port`, `method`, `conn`, `url`, `socket`, `socket_dir`, `host`, `port`

### Shell eval

```bash
eval "$(zerobased env --export acountee)"
echo $ZB_POSTGRES_5432

# Custom prefix
eval "$(zerobased --prefix APP env --export acountee)"
echo $APP_POSTGRES_5432

# No prefix
eval "$(zerobased --prefix '' env --export acountee)"
echo $POSTGRES_5432
```

## Portless service fabric

Status: planned runtime behavior. The pure contracts live in `internal/fabric`; CLI and daemon wiring are still future work. Do not present `zerobased up` or `zerobased.yaml` as released product behavior yet.

The goal is to remove port choice from normal local development. Users and agents name services. Zerobased leases private endpoints, injects them into processes or containers, probes readiness, exposes stable names, and cleans residue by lease.

The important inversion: zerobased should assign and inject private endpoints before the process starts. The app can use random private ports or sockets. Zerobased organizes them with env aliases and proxies. The fabric should not guess ports from stdout after startup.

### Intended usage

Create `zerobased.yaml`:

```yaml
identity:
  app: billing
  port_range: 42000-49999

ports:
  db:
    mode: tcp
    port: 5432
  api: http
  web: http

profiles:
  local:
    db.url: "postgres://localhost:{{db.port}}/app"
  staging:
    disable: [db]
    required: [db.url]

services:
  db:
    cmd: docker compose up db
    endpoint: env_port
    env:
      DB_PORT: "{{db.port}}"
    source:
      probe:
        http:
          port: db
          path: /health
          expect_status: 200
          expect_header: X-ZB-Service=db
      found: use
      missing: start
    ready:
      tcp: db
  api:
    cmd: go run ./cmd/api
    needs:
      db: required
    env:
      HOST: "{{api.bind_host}}"
      PORT: "{{api.port}}"
      DATABASE_URL: "{{db.url}}"
      AUTH_CALLBACK_URL: "{{api.url}}/auth/callback"
    ready:
      http: /health
  web:
    cmd: pnpm dev
    needs:
      api: required
    env:
      HOST: "{{web.bind_host}}"
      PORT: "{{web.port}}"
      API_URL: "{{api.url}}"
      CSP_CONNECT_SRC: "{{api.url}}"

routes:
  app:
    visibility: public
    /api/*: api
    /ws/*: api
    /*: web
  internal:
    visibility: internal
    /metrics: api
```

Run it:

```bash
zerobased up
zerobased up --profile staging --set db.url="$STAGING_DATABASE_URL"
zerobased ps
zerobased get api
zerobased env --export
```

Expected user-facing output:

```text
web.<branch>.<repo>.localhost
api.<branch>.<repo>.localhost
app.<branch>.<repo>.localhost/api
app.<branch>.<repo>.localhost/
```

Expected injected env:

```bash
HOST=127.0.0.1
PORT=<private-os-assigned-port>
API_URL=http://api.<branch-or-worktree>.<repo>.localhost
DATABASE_URL=postgres://localhost:5432/app
ZB_SERVICE_MAP=<json>
```

The short form expands to:

- `ports` declares named private connection points
- fixed ports are explicit constraints
- service `source` decides how an existing fixed endpoint may be used; `found: use` requires identity proof unless an explicit escape hatch is set
- `profiles` rewires values for local, staging, test, or CI
- `services.*.env` says how each process receives connection values
- Go templates turn endpoint facts into env values
- app-specific connection strings come from templates, not type-specific magic
- `routes` names final user-facing app surfaces; domains are generated from route name, branch-or-worktree, repo, and configured domain
- route `visibility` is canonical: `internal` means loopback/private only, no external domain expansion, no share output
- each service still gets its own host: `api.<branch-or-worktree>.<repo>.localhost`

Do not build new workflows around `localhost:<port>`. Numeric ports are private implementation detail. If a tool needs a port, zerobased must inject it before start through env, flags, listener handoff, or an adapter.

### Namespaces

Each service identity is derived from:

```text
repo + branch-or-worktree + service
```

That makes parallel branches and agent worktrees safe. Two branches running the same `api` service should get different host labels without user-selected ports.

Example shapes:

```text
api.<branch-or-worktree-label>.<repo-label>.localhost
app.<branch-or-worktree-label>.<repo-label>.localhost
```

Labels are URL-safe and collision-resistant. Conflicts should be visible, not silently overwritten.

### Endpoint policy

Zerobased should pick the strongest endpoint mechanism the service supports:

| Policy | Class | Meaning |
|---|---|---|
| `unix_socket` | normal | Use a Unix socket when the protocol supports it |
| `listener_handoff` | normal | Hand an already-bound listener to the process; fallback defaults to fail |
| `env_port` | normal | Lease a deterministic candidate, recheck before start, then inject `PORT` and `HOST` |
| `env_flag` | normal | Inject framework-specific flags |

Unsupported tools need adapters, not post-start guessing. An adapter may extract a port setting from a known config file or command shape, then rewrite/inject it before the process starts.

Adapter example:

```yaml
services:
  vite_app: pnpm vite --host $HOST --port $PORT
```

If zerobased cannot control a service endpoint before start, startup should fail with adapter guidance instead of falling back to stdout parsing.

### Env aliases and proxies

Processes should consume named aliases, not private endpoints:

```text
API_URL=http://api.<branch-or-worktree>.<repo>.localhost
DATABASE_URL=postgresql://postgres@/app?host=<endpoint-dir>
ZB_SERVICE_MAP=<machine-readable service map>
```

Proxy type depends on service kind:

| Proxy type | Use |
|---|---|
| HTTP host | browser apps and APIs |
| HTTP path | one gateway with `/api`, `/ws`, `/` |
| Unix socket | databases and local-only protocols |
| TCP bridge | protocols without socket support |

### Lifecycle

Runtime state must separate process start from readiness:

```text
waiting_for_deps -> starting -> probing -> ready
                              -> degraded
                              -> failed
```

Other cleanup states:

```text
stopping -> released -> pruned
crash/deleted/expired -> orphaned -> pruned
```

`ready` only means a probe passed. A running process is not enough.

### Cleanup

Every private endpoint has a lease owner:

| Owner | Example |
|---|---|
| `daemon_session` | gateway-owned endpoint |
| `wrapped_pid` | host command started by zerobased |
| `docker_container` | discovered Docker container |
| `compose_service` | Compose service abstraction |

Cleanup should be reviewable before destructive action:

```bash
zerobased prune --dry-run
zerobased prune
zerobased clean
```

Expected behavior:

| Event | First action | Later action |
|---|---|---|
| clean stop | release lease | prune residue |
| crash | mark orphaned | prune after review |
| expired lease | mark orphaned | prune |
| deleted worktree | mark orphaned | prune |
| gateway restart | rebuild ready/degraded routes | keep failed/orphaned hidden |

### Verification target

The portless fabric is not done until these user-visible checks pass:

| Impact | Proof |
|---|---|
| No normal port conflicts | Two worktrees run the same service without choosing ports |
| Docker and host commands share names | `ps`, `get`, and URLs show both kinds together |
| Dependencies wait correctly | Dependent services stay blocked until upstream probes pass |
| Stale state is repairable | Crashed processes become orphaned and prune explains cleanup |
| Gateway recovers | Restarted gateway rebuilds ready/degraded routes from leases |
| No post-start guessing | Endpoint is assigned before process start; unsupported tools fail with adapter guidance |

### Remote preview sharing

Share live previews with reviewers when working on a remote machine (behind Tailscale or Cloudflare Tunnel):

```bash
# Add an external domain (default 4h TTL, auto-cleans)
zerobased domain add preview.dev.co

# Custom TTL or persistent
zerobased domain add preview.dev.co --ttl 30m
zerobased domain add box.ts.net --persistent

# See all shareable URLs
zerobased share

# Filter by domain
zerobased share @1

# Clean up
zerobased unshare @1        # remove one domain
zerobased unshare --all     # remove all domains
```

All services are automatically exposed on every configured domain. Routes are registered for `localhost` + all domains simultaneously — local dev keeps working.

**Tailscale:**
```bash
tailscale serve --bg --https=443 80
zerobased domain add $(tailscale status --json | jq -r .Self.DNSName | sed 's/\.$//')
```

**Cloudflare Tunnel** (with wildcard DNS for `*.preview.yourdomain.com`):
```bash
cloudflared tunnel run dev-preview
zerobased domain add preview.yourdomain.com
```

**No tunnel** (direct IP via nip.io):
```bash
zerobased domain add 10.0.1.50.nip.io
```

## Claude Code plugin

zerobased ships as a Claude Code plugin — gives Claude procedural knowledge for service discovery, connection strings, dev server wrapping, and remote preview sharing.

```bash
# Install as marketplace
claude plugin marketplace add lagz0ne/zerobased
claude plugin install zerobased@zerobased
```

Triggers on: "start services", "connect to database", "get connection string", "share preview", "run dev server", "add domain", "set up tunnel", and any project with `docker-compose.yml`.

## How it works

1. `zerobased start` launches a Caddy container (host network mode) and watches `/var/run/docker.sock`
2. On container start: inspects ports + Compose labels, classifies each port, creates Unix socket bridges or Caddy routes
3. On container stop: removes sockets + deregisters routes automatically

Unix sockets are pure Go (`net.Listen("unix")` + bidirectional `io.Copy`) — no `socat` dependency.

### Auto-classification

| Container port | Type | Exposure |
|---|---|---|
| 5432, 3306, 6379, 27017, 4317 | Database / gRPC | Unix socket |
| 6222, 9222, 7946, 2377, 2380 | Internal / cluster | Skipped |
| 80, 443, 3000, 3001, 4318, 5173, 8000, 8025, 8080, 8222, 8443, 8888, 9090 | HTTP / WebSocket | Caddy reverse proxy |
| Everything else | TCP | Deterministic port hash |

Override per-port with a Docker label (rare):

```yaml
labels:
  zerobased.4222: http    # force HTTP instead of port hash
```

### Naming convention

Derived from Docker Compose labels (`com.docker.compose.project` + `com.docker.compose.service`):

| Type | Pattern | Example |
|---|---|---|
| HTTP | `<service>-<port>.<project>.localhost` | `nats-80.acountee.localhost` |
| Socket | `~/.zerobased/sockets/<project>/<service>-<port>.sock` | `~/.zerobased/sockets/acountee/redis-6379.sock` |
| Socket (Postgres) | `~/.zerobased/sockets/<project>/.s.PGSQL.5432` | Native Postgres socket convention |
| Port | `hash("<project>:<service>:<port>") % 20000 + 10000` | Stable across restarts |

### Multi-project isolation

Different Compose projects get different namespaces automatically. Run multiple projects simultaneously — no conflicts.

## Requirements

- Docker with Compose
- Linux or macOS (host must be able to reach container IPs directly)

## Release process

Releases are tag-driven. Pushing `v*` runs `.github/workflows/release.yml`.

Before tagging:

```bash
go test ./...
go build -ldflags "-s -w -X main.version=0.1.0-preflight" -o /tmp/zerobased ./cmd/zerobased
/tmp/zerobased help
/tmp/zerobased version
make cross
make npm
```

Release checklist:

1. Ensure `NPM_TOKEN` is configured in GitHub Actions secrets.
2. Pick a semver tag, for example `v0.1.0`.
3. Push the tag:

   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. GitHub Actions builds:
   - `linux/amd64`
   - `linux/arm64`
   - `darwin/amd64`
   - `darwin/arm64`

5. The release job creates the GitHub Release from `dist/*`.
   If the GitHub Release already exists for the tag, the workflow uploads assets with `--clobber` instead of failing.
6. The npm job publishes platform packages first, then the `zerobased` wrapper package.
7. Smoke test after publish:

   ```bash
   npm install -g zerobased
   zerobased version
   zerobased help
   ```

Current npm package layout:

| Package | Purpose |
| --- | --- |
| `zerobased` | wrapper package with `postinstall` binary copy |
| `@lagz0ne/zerobased-linux-x64` | Linux x64 binary |
| `@lagz0ne/zerobased-linux-arm64` | Linux arm64 binary |
| `@lagz0ne/zerobased-darwin-x64` | macOS Intel binary |
| `@lagz0ne/zerobased-darwin-arm64` | macOS Apple Silicon binary |

## License

MIT
