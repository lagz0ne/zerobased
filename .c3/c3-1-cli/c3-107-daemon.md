---
id: c3-107
c3-version: 4
title: daemon
type: component
category: foundation
parent: c3-1
goal: Event loop orchestrator — watches Docker events, classifies ports, dispatches to socat/Caddy, cleans up on stop
summary: Central coordinator that wires docker-client → classifier → socat/caddy, tracks all service entries, handles cleanup
---

# daemon

## Goal

Event loop orchestrator — watches Docker events, classifies ports, dispatches to socat/Caddy, cleans up on stop.

## Container Connection

The brain. Without the daemon, nothing connects. It:
1. Starts Caddy (c3-105)
2. Scans existing containers (c3-101)
3. Classifies each port (c3-102)
4. Routes to socat (c3-104), Caddy HTTP (c3-105), or Caddy TCP (c3-103 + c3-105)
5. On container stop: reverses everything

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| IN | Docker events + inspection | c3-101 |
| IN | port classification | c3-102 |
| IN | port hashing | c3-103 |
| IN | socat bridging | c3-104 |
| IN | Caddy routing | c3-105 |
| IN | connection string display | c3-106 |

## Code References

| File | Purpose |
|------|---------|
| `internal/daemon/daemon.go` | Start(), Stop(), Services(), handleStart(), handleStop() |
