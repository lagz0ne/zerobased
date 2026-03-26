---
name: zerobased
version: 1.0.0
description: >-
  This skill should be used when the user asks to "start services",
  "connect to database", "get connection string", "share preview",
  "share with reviewer", "expose service", "run dev server",
  "what services are running", "set up local dev", "docker compose services",
  "add domain", "list domains", "set up tunnel", "tailscale preview",
  "cloudflare tunnel", "path-based routing", "routefile", "zerobased logs",
  or when working in a project with docker-compose.yml that needs service routing,
  connection strings, or remote preview sharing via zerobased.
---

# zerobased — Zero-Config Docker Service Router

Watches `docker.sock`, auto-classifies container ports, creates Unix sockets + HTTP routes via Caddy. No config files, no labels, no setup scripts.

Install: `npm install zerobased` (cross-platform binary via npm) or `make build` from source (requires Go 1.25+).

## When to Use

Activate zerobased whenever a project uses Docker Compose and needs:
- Service connection strings (database URLs, HTTP endpoints)
- Dev server routing with auto-port detection
- Remote preview sharing with reviewers

## Core Workflow

### 1. Start the Daemon

```bash
zerobased start -d    # background mode, auto-follows logs
```

The daemon watches Docker events. As containers start/stop, routes register/deregister automatically.

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

For detailed tunnel setup instructions, consult `references/sharing.md`.

## Port Classification

Ports are auto-classified by well-known number:

| Method | Ports | Result |
|--------|-------|--------|
| Socket | 5432, 3306, 6379, 27017, 4317 | Unix socket at `~/.zerobased/sockets/{project}/` |
| HTTP | 80, 3000, 5173, 8080, ... | `{service}-{port}.{project}.localhost` |
| Port | everything else | `localhost:{hash_port}` (deterministic FNV-1a) |
| Internal | 6222, 9222, 7946, 2377 | never exposed |

Override with Docker labels: `zerobased.{port}=socket|http|port|internal`.

## Routefile (Path-Based Routing)

For projects needing a gateway pattern, create `zerobased.routes` or `zerobased.routes.yaml` in the project root.

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

## Additional Resources

### Reference Files

- **`references/sharing.md`** — Tunnel setup guides for Tailscale and Cloudflare, comparison table, nip.io fallback
