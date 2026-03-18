---
id: c3-1
c3-version: 4
title: cli
type: container
boundary: app
parent: c3-0
goal: Single Go binary that watches Docker events, classifies ports, and orchestrates socat/Caddy to expose services
summary: The daemon + CLI — all routing logic lives here as internal packages
uses: [ref-naming-convention]
---

# cli

## Goal

Single Go binary that watches Docker events, classifies ports, and orchestrates socat/Caddy to expose services.

## Responsibilities

- Watch docker.sock for container start/stop events
- Classify container ports into socket/http/port categories
- Manage socat processes for Unix socket bridges
- Manage Caddy container for HTTP hostname routing
- Provide CLI commands for inspection and dev server wrapping

## Complexity Assessment

**Level:** moderate
**Why:** Multiple external process management (socat, Caddy container), Docker API event stream, concurrent cleanup on shutdown

## Components

| ID | Name | Category | Status | Goal Contribution |
|----|------|----------|--------|-------------------|
| c3-101 | docker-client | foundation | active | Docker SDK wrapper — events, inspect, list |
| c3-102 | classifier | foundation | active | Port → expose method mapping |
| c3-103 | port-hasher | foundation | active | Deterministic port hashing for TCP services |
| c3-104 | socat-bridge | foundation | active | Unix socket ↔ container IP bridging |
| c3-105 | caddy-manager | foundation | active | Caddy container lifecycle + admin API |
| c3-106 | env-generator | foundation | active | Connection string generation |
| c3-107 | daemon | foundation | active | Event loop orchestrator — ties everything together |
| c3-110 | run-wrapper | feature | active | Wrap dev server process with auto-routing |

