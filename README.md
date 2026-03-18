---
status: active
started: 2026-03-18
tags: [docker, developer-tools, zero-config, unix-sockets]
---

# zerobased

Zero-config Docker service router. Watches `docker.sock` for container events, auto-classifies ports, creates Unix sockets + HTTP routes. Cleans up when containers stop.

## Install

```bash
npm install -g zerobased
```

## Usage

```bash
# Start daemon (once per machine)
zerobased start

# Your existing workflow — unchanged
docker compose up -d          # zerobased auto-detects, creates sockets + routes
zerobased ps                  # see what was discovered
zerobased env acountee        # print connection strings

# Wrap a dev server
zerobased run bun --bun vite dev   # → http://acountee.localhost

# Stop everything
zerobased stop
```

No config files. No Docker labels. Existing `docker-compose.yml` works as-is.

## How it works

1. `zerobased start` launches a Caddy container (host network) and watches `/var/run/docker.sock`
2. On container start: inspects ports, classifies by well-known port number, creates socat bridges (sockets) or Caddy routes (HTTP)
3. On container stop: removes sockets + deregisters routes

### Auto-classification

| Container port | Type | Exposure |
|---|---|---|
| 5432, 3306, 6379, 27017, 4317 | Database/gRPC | Unix socket |
| 80, 443, 3000, 5173, 8080, 8222, 9090... | HTTP | Caddy hostname route |
| Everything else | TCP | Deterministic port hash |

### Naming

- HTTP: `<service>-<port>.<project>.localhost`
- Sockets: `~/.zerobased/sockets/<project>/<service>-<port>.sock`
- Ports: `hash("<project>:<service>:<port>") % 20000 + 10000`

## Build

```bash
make build    # local binary → bin/zerobased
make test     # run tests
make cross    # cross-compile for all platforms
```
