---
id: c3-105
c3-version: 4
title: caddy-manager
type: component
category: foundation
parent: c3-1
goal: Manage Caddy container lifecycle and register/deregister HTTP routes via admin API
summary: Starts Caddy in host-network mode, loads config via /load, manages routes via /config and /id endpoints
---

# caddy-manager

## Goal

Manage Caddy container lifecycle and register/deregister HTTP routes via admin API.

## Container Connection

Implements the "http" path from c3-102. Web services get hostname-based routing (`<service>-<port>.<project>.localhost`) through Caddy reverse proxy. Also handles TCP port routing for the "port" path.

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| IN | Docker client for container mgmt | c3-101 |
| IN | route ID, hostname, upstream | c3-107 |

## Code References

| File | Purpose |
|------|---------|
| `internal/caddy/caddy.go` | Start(), Stop(), AddHTTPRoute(), AddTCPRoute(), RemoveRoute(), EnsureHTTPServer() |
