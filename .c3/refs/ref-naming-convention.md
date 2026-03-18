---
id: ref-naming-convention
c3-version: 4
title: Naming Convention
type: ref
goal: Deterministic, collision-free naming for HTTP hostnames, socket paths, and hashed ports
summary: All names derived from Docker Compose labels (project + service) — no config needed
via: [c3-102, c3-106]
---

# Naming Convention

## Goal

Deterministic, collision-free naming for HTTP hostnames, socket paths, and hashed ports.

## Choice

All external names (hostnames, socket paths, port numbers) are derived deterministically from Docker Compose labels:
- `com.docker.compose.project` → project name
- `com.docker.compose.service` → service name

## Why

Zero-config means no naming config. Compose already has project + service identity — reuse it. Deterministic naming means restart-stable URLs and paths.

## How

| Type | Pattern | Example |
|------|---------|---------|
| HTTP hostname | `<service>-<port>.<project>.localhost` | `nats-80.acountee.localhost` |
| Socket path | `~/.zerobased/sockets/<project>/<service>-<port>.sock` | `~/.zerobased/sockets/acountee/redis-6379.sock` |
| Socket (Postgres) | `~/.zerobased/sockets/<project>/.s.PGSQL.5432` | Native `libpq` convention |
| Hashed port | `FNV-1a(project:service:port) % 20000 + 10000` | `localhost:26987` |
| Run hostname | `<name>.localhost` | `acountee.localhost` |

## Scope

Applies to: c3-102 (SocketFilename), c3-106 (Hostname, ForPort), c3-103 (DeterministicPort)
