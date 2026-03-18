---
id: c3-0
c3-version: 4
title: zerobased
goal: Zero-config Docker service routing — watch docker.sock, auto-classify ports, create Unix sockets + HTTP routes, clean up on stop
summary: CLI daemon that watches Docker events, classifies container ports by well-known conventions, and exposes them as Unix sockets (databases), HTTP hostname routes (web), or deterministic port hashes (TCP)
---

# zerobased

## Goal

Zero-config Docker service routing — watch docker.sock, auto-classify ports, create Unix sockets + HTTP routes, clean up on stop.

## Abstract Constraints

| Constraint | Rationale | Affected Containers |
|------------|-----------|---------------------|
| Zero configuration | Existing docker-compose.yml works as-is, no labels/config files required | c3-1 |
| Auto-cleanup | All sockets and routes removed when containers stop or daemon shuts down | c3-1 |
| Direct container IP | Connect via Docker network IP, not host port mappings — avoids port conflicts | c3-1 |

## Overview

```mermaid
graph TD
    USER[Developer] -->|zerobased start/ps/env/get| CLI[CLI Binary]
    CLI --> DAEMON[Daemon Loop]
    DAEMON -->|watches| DOCKER_SOCK[docker.sock]
    DOCKER_SOCK -->|container events| DAEMON
    DAEMON -->|inspect + classify| CLASSIFIER[Port Classifier]
    DAEMON -->|socket bridges| SOCAT[socat processes]
    DAEMON -->|HTTP routes| CADDY[Caddy Container]
    DAEMON -->|port hashing| HASH[Port Hasher]

    USER -->|zerobased run| RUN[Run Wrapper]
    RUN -->|register/deregister| CADDY
```

## Containers

| ID | Name | Boundary | Status | Responsibilities | Goal Contribution |
|----|------|----------|--------|------------------|-------------------|
| c3-1 | cli | app | active | Docker event watching, port classification, socat/Caddy orchestration, CLI interface | The single binary that delivers all zero-config routing |
