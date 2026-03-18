---
id: c3-104
c3-version: 4
title: socat-bridge
type: component
category: foundation
parent: c3-1
goal: Spawn and manage socat processes that bridge Unix sockets to container TCP addresses
summary: Host-side socat UNIX-LISTEN ↔ TCP bridge with lifecycle tracking per socket path
---

# socat-bridge

## Goal

Spawn and manage socat processes that bridge Unix sockets to container TCP addresses.

## Container Connection

Implements the "socket" path from c3-102. Database containers (Postgres, MySQL, Redis, etc.) get native Unix socket access — apps connect as if the database is local.

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| IN | socket dir, filename, TCP target | c3-107 |
| OUT | socket file path | c3-106 |

## Code References

| File | Purpose |
|------|---------|
| `internal/socat/socat.go` | Bridge(), Remove(), RemoveAll(), RemoveByPrefix() |
