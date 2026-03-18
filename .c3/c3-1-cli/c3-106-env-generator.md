---
id: c3-106
c3-version: 4
title: env-generator
type: component
category: foundation
parent: c3-1
goal: Generate protocol-correct connection strings for each service endpoint
summary: Maps (project, service, port, method) → connection string with correct URI scheme
uses: [ref-naming-convention]
---

# env-generator

## Goal

Generate protocol-correct connection strings for each service endpoint.

## Container Connection

Powers `zerobased env` and `zerobased get` CLI commands. Produces the actual strings developers paste into `.env` files — must be correct for each protocol (postgresql://, mysql://, redis+unix://, http://, localhost:port).

## Dependencies

| Direction | What | From/To |
| --------- | ---- | ------- |
| IN | classifier results | c3-102 |
| IN | port hash | c3-103 |

## Code References

| File | Purpose |
|------|---------|
| `internal/env/env.go` | ForPort(), Hostname(), PrintEndpoints() |

## Related Refs

| Ref | How It Serves Goal |
|-----|-------------------|
| ref-naming-convention | Hostname and socket path patterns |
