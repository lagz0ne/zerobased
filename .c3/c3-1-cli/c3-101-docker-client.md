---
id: c3-101
c3-version: 4
title: docker-client
type: component
category: foundation
parent: c3-1
goal: Docker SDK wrapper — event stream, container inspection, network IP extraction
summary: Thin wrapper over github.com/docker/docker/client providing filtered events and ContainerInfo extraction
---

# docker-client

## Goal

Docker SDK wrapper — event stream, container inspection, network IP extraction.

## Container Connection

Without this, the daemon has no visibility into Docker. Provides the event stream that drives all routing decisions and the inspection data (IPs, ports, Compose labels) that other components consume.

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| OUT | ContainerInfo, event channels | c3-107 |

## Code References

| File | Purpose |
|------|---------|
| `internal/docker/client.go` | Events(), Inspect(), ListRunning() |
