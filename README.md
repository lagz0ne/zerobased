# zerobased

Zero-config Docker service router. Watches `docker.sock` for container events, auto-classifies ports, creates Unix sockets + HTTP routes. Cleans up when containers stop.

No config files. No Docker labels. No setup scripts. Your existing `docker-compose.yml` works as-is.

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
# Start the daemon (once per machine)
zerobased start

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
  otel-collector  socket 4317 → unix://~/.zerobased/sockets/acountee/otel-collector-4317.sock
  otel-collector  http   4318 → http://otel-collector-4318.acountee.localhost
```

## CLI

```
zerobased start              Start daemon (watches docker.sock, manages Caddy)
zerobased stop               Stop daemon + Caddy + cleanup all sockets
zerobased run [name] <cmd>   Wrap dev server, register route, cleanup on exit
zerobased env [project]      Print connection strings for a project
zerobased ps                 Show all discovered services across all projects
zerobased get <service>      Print one connection string
```

### Wrapping dev servers

```bash
zerobased run acountee pnpm dev    # → http://acountee.localhost
zerobased run api bun run serve    # → http://api.localhost
```

The route is registered on start and cleaned up when the process exits.

## How it works

1. `zerobased start` launches a Caddy container (host network mode) and watches `/var/run/docker.sock`
2. On container start: inspects ports + Compose labels, classifies each port, creates socat bridges or Caddy routes
3. On container stop: removes sockets + deregisters routes automatically

### Auto-classification

| Container port | Type | Exposure |
|---|---|---|
| 5432, 3306, 6379, 27017, 4317 | Database / gRPC | Unix socket via socat |
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
| Socket (Postgres) | `~/.zerobased/sockets/<project>/.s.PGSQL.5432` | Uses native Postgres socket convention |
| Port | `hash("<project>:<service>:<port>") % 20000 + 10000` | Stable across restarts |

### Multi-project isolation

Different Compose projects get different namespaces automatically. Run multiple projects simultaneously — no conflicts.

## Requirements

- Docker with Compose
- `socat` installed on the host (`apt install socat` / `brew install socat`)
- Linux or macOS (host must be able to reach container IPs directly)

## License

MIT
