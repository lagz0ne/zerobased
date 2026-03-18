---
id: c3-102
c3-version: 4
title: classifier
type: component
category: foundation
parent: c3-1
goal: Map container ports to expose methods (socket/http/port) via well-known port tables
summary: Pure function — port number + optional label override → ExposeMethod enum
uses: [ref-naming-convention]
---

# classifier

## Goal

Map container ports to expose methods (socket/http/port) via well-known port tables.

## Container Connection

Central decision point — every port discovered by c3-101 flows through here before being routed to socat (c3-104), Caddy (c3-105), or port hash (c3-103).

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| OUT | ExposeMethod, SocketFilename | c3-107 |
| OUT | ExposeMethod, SocketFilename | c3-106 |

## Code References

| File | Purpose |
|------|---------|
| `internal/classifier/classifier.go` | Classify(), SocketFilename(), well-known port tables |
| `internal/classifier/classifier_test.go` | 6 tests — sockets, HTTP, unknown, label override, filenames |

## Related Refs

| Ref | How It Serves Goal |
|-----|-------------------|
| ref-naming-convention | Socket filenames follow conventions (e.g., `.s.PGSQL.5432`) |
