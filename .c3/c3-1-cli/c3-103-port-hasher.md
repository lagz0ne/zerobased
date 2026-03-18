---
id: c3-103
c3-version: 4
title: port-hasher
type: component
category: foundation
parent: c3-1
goal: Deterministic port hashing — stable localhost port for TCP services across restarts
summary: FNV-1a hash of project:service:port → port in [10000, 30000)
---

# port-hasher

## Goal

Deterministic port hashing — stable localhost port for TCP services across restarts.

## Container Connection

Handles the "port" classification from c3-102. Services like NATS client (4222) that aren't HTTP and don't use Unix sockets get a stable hashed port.

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| OUT | stable hashed port | c3-107 |
| OUT | stable hashed port | c3-106 |

## Code References

| File | Purpose |
|------|---------|
| `internal/ports/hash.go` | DeterministicPort() |
| `internal/ports/hash_test.go` | Stability, range, uniqueness tests |
