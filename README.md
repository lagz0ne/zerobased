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
  start [-d]                               Start daemon (-d for background)
  stop                                     Stop daemon + cleanup
  logs [-f]                                Show daemon logs (-f to follow)
  run [-p port] [name] <cmd>               Wrap dev server, register route
  env [--export] [project]                 Print connection strings
  ps                                       Show all discovered services
  get <service> [-t template] [-v k=v]     Print one connection string
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

## License

MIT
