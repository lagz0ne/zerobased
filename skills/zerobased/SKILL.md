---
name: zerobased
version: 1.0.0
description: >-
  This skill should be used when the user invokes "/zerobased" or
  explicitly mentions "zerobased" by name. Provides the CLI reference
  for zerobased — a zero-config Docker service router that auto-detects
  containers, generates connection strings, wraps dev servers, and
  shares live previews with remote reviewers.
---

# zerobased — Zero-Config Docker Service Router

Watches `docker.sock`, auto-classifies container ports, creates Unix sockets + HTTP routes via Caddy. No config files, no labels, no setup scripts.

## Prerequisite Check

Before running any zerobased command, verify the binary is available:

```bash
which zerobased || command -v zerobased
```

If not installed, install via one of:
- `npm install -g zerobased` (cross-platform binary via npm)
- `make build && make install` from source (requires Go 1.25+)
- Download from [github.com/lagz0ne/zerobased/releases](https://github.com/lagz0ne/zerobased/releases)

**Do not proceed with any zerobased command until the binary is confirmed available.** If installation fails, inform the user and suggest alternatives.

## Core Workflow

### 1. Start the Daemon

```bash
zerobased start -d    # background mode, auto-follows logs
```

The daemon watches Docker events. As containers start/stop, routes register/deregister automatically.

If the daemon fails to start or appears stuck, run `zerobased stop` to clean up, then `zerobased start -d`. The daemon recovers automatically from crashes — stale sockets and routes are swept on startup.

### 2. Discover Services

```bash
zerobased ps          # list all services by project
zerobased env myapp   # connection strings for a project
```

### 3. Get Connection Strings

```bash
# Simple
zerobased get postgres
# → postgresql://postgres@/postgres?host=/home/user/.zerobased/sockets/myapp

# Templated
zerobased get postgres -t 'postgresql://{{user}}:{{pass}}@/{{db}}?host={{socket_dir}}' \
  -v user=app -v pass=secret -v db=mydb

# Shell eval (inject all as env vars)
eval "$(zerobased env --export myapp)"

# Custom prefix (APP_POSTGRES_5432 instead of ZB_POSTGRES_5432)
eval "$(zerobased --prefix APP env --export myapp)"
```

Template variables: `project`, `service`, `container_port`, `method`, `conn`, `url`, `socket`, `socket_dir`, `host`, `port`.

### 4. Run Dev Servers

```bash
zerobased run frontend next dev          # auto-detect port from stdout
zerobased run -p 3000 frontend next dev  # explicit port
zerobased run api go run ./cmd/api       # any command
```

Auto-detects the listening port by scanning stdout for `http(s)://host:port`. Registers a Caddy route, injects `ZB_*` env vars from running Docker services, cleans up on exit.

### 5. Share Previews with Reviewers

When working on a remote machine (behind Tailscale or Cloudflare Tunnel), share live previews:

```bash
# Add external domain (default 4h TTL)
zerobased domain add preview.dev.co

# Custom TTL or persistent
zerobased domain add preview.dev.co --ttl 30m
zerobased domain add box.ts.net --persistent

# See shareable URLs
zerobased share
zerobased share @1    # filter by domain index

# Clean up
zerobased unshare @1        # remove one domain
zerobased unshare --all     # remove all
```

All services are automatically exposed on every configured domain. Routes register for localhost + all domains simultaneously.

## Tunnel Setup

### How It Works

1. Tunnel exposes Caddy's port 80 to the internet (or tailnet)
2. `zerobased domain add <domain>` registers routes for all services on that domain
3. Caddy matches incoming `Host` headers and proxies to the right container
4. Wildcard DNS on the tunnel side resolves `*.domain` to the tunnel endpoint

### Tailscale

One-time setup:
```bash
tailscale serve --bg --https=443 80     # private (tailnet only)
tailscale funnel --bg --https=443 80    # public (internet)
```

Register domain:
```bash
zerobased domain add $(tailscale status --json | jq -r .Self.DNSName | sed 's/\.$//')
```

Reviewer accesses: `https://api-3000.myapp.dev-box.tail1234.ts.net`

| Aspect | Detail |
|--------|--------|
| DNS | Automatic via MagicDNS (wildcard subdomains supported) |
| TLS | Automatic via Tailscale |
| Auth | Tailnet-private by default; public via Funnel |
| Reviewer needs client | Yes for Serve, No for Funnel |

### Cloudflare Tunnel

One-time setup:
```bash
cloudflared tunnel create dev-preview
# Configure wildcard ingress in ~/.cloudflared/config.yml:
#   hostname: "*.preview.yourdomain.com" → http://localhost:80
cloudflared tunnel run dev-preview
```

Register domain:
```bash
zerobased domain add preview.yourdomain.com
```

Reviewer accesses: `https://api-3000.myapp.preview.yourdomain.com`

| Aspect | Detail |
|--------|--------|
| DNS | Requires wildcard CNAME → tunnel ID |
| TLS | Automatic via Cloudflare edge |
| Auth | Public by default; restrict via Cloudflare Access |
| Reviewer needs client | No |

### Comparison

| | Tailscale | Cloudflare Tunnel |
|--|-----------|-------------------|
| Best for | Team-internal previews | Public/client-facing previews |
| Setup | `tailscale serve 80` | Tunnel + wildcard DNS + domain |
| Auth | Tailnet (automatic) | Public or Cloudflare Access |
| Domain cost | Free (*.ts.net) | Requires owned domain |

### Fallback: nip.io (No Tunnel)

For machines with a directly reachable IP:
```bash
zerobased domain add 10.0.1.50.nip.io
```

No tunnel, no DNS setup. HTTP only (no TLS).

### Path-Based Routing with Tunnels

If the tunnel doesn't support wildcard DNS, use a routefile for single-hostname path routing:

```
# zerobased.routes
/api     api
/        frontend
```

`myapp.preview.dev.co/api` → API, `myapp.preview.dev.co/` → frontend. No wildcard needed.

### TTL Behavior

- Default: 4 hours (auto-cleans if forgotten)
- Custom: `--ttl 30m`, `--ttl 8h`
- Persistent: `--persistent` (never expires)
- Daemon sweeps expired domains every 30 seconds

## Port Classification

| Method | Ports | Result |
|--------|-------|--------|
| Socket | 5432, 3306, 6379, 27017, 4317 | Unix socket at `~/.zerobased/sockets/{project}/` |
| HTTP | 80, 443, 3000, 3001, 4318, 5173, 8000, 8025, 8080, 8222, 8443, 8888, 9090 | `{service}-{port}.{project}.localhost` |
| Port | everything else | `localhost:{hash_port}` (deterministic FNV-1a) |
| Internal | 6222, 9222, 7946, 2377 | never exposed |

Override with Docker labels: `zerobased.{port}=socket|http|port|internal`.

## Routefile (Path-Based Routing)

Create `zerobased.routes` or `zerobased.routes.yaml` in the project root for a single-gateway pattern.

**Text format** (`zerobased.routes`):
```
/api     api
/ws      ws
/        frontend
```

**YAML with profiles** (`zerobased.routes.yaml`):
```yaml
profiles:
  default:
    routes:
      /api: api
      /: frontend
  staging:
    extends: [default]
    routes:
      /db: postgres://staging-db:5432
```

Start with profile: `zerobased start --profile staging`.

External targets (https://, wss://, postgres://, nats://, redis://) are supported.

## Quick Reference

| Task | Command |
|------|---------|
| Start daemon | `zerobased start -d` |
| Stop daemon | `zerobased stop` |
| List services | `zerobased ps` |
| Connection strings | `zerobased env [project]` |
| One connection | `zerobased get <service>` |
| Shell export | `eval "$(zerobased env --export)"` |
| Wrap dev server | `zerobased run [name] <cmd>` |
| Add share domain | `zerobased domain add <domain> [--ttl 2h]` |
| List domains | `zerobased domain list` |
| Show share URLs | `zerobased share [@N]` |
| Remove domain | `zerobased unshare @N` or `--all` |
| Daemon logs | `zerobased logs -f` |

## Decision Guide

- **Need a database URL?** → `zerobased get postgres` or `zerobased env --export`
- **Starting a frontend?** → `zerobased run frontend next dev`
- **Reviewer needs to see progress?** → `zerobased domain add preview.dev.co && zerobased share`
- **Done sharing?** → `zerobased unshare --all`
- **Which services are up?** → `zerobased ps`
- **Custom connection format?** → `zerobased get <svc> -t '{{template}}' -v key=val`
