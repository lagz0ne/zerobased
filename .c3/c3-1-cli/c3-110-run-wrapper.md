---
id: c3-110
c3-version: 4
title: run-wrapper
type: component
category: feature
parent: c3-1
goal: Wrap a dev server process, register its route with Caddy, clean up on exit
summary: Spawns child process, registers hostname route, forwards signals, deregisters on exit
---

# run-wrapper

## Goal

Wrap a dev server process, register its route with Caddy, clean up on exit.

## Container Connection

The only feature component — implements `zerobased run [name] <cmd>`. Unlike the daemon's auto-discovery flow, this handles non-Docker processes (local dev servers) by registering them directly with Caddy.

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| IN | Caddy admin API | c3-105 |

## Code References

| File | Purpose |
|------|---------|
| `internal/run/run.go` | Run(), registerRoute(), deregisterRoute(), detectPort() |
