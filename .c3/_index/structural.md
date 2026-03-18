# C3 Structural Index
<!-- hash: sha256:216b40e31e2b3b8cb6923717d8958517eef06fba945747ff0e898049158822df -->

## adr-00000000-c3-adoption — C3 Architecture Documentation Adoption (adr)
blocks: Goal ✓

## c3-0 — zerobased (context)
reverse deps: adr-00000000-c3-adoption, c3-1
blocks: Abstract Constraints ✓, Containers ✓, Goal ✓

## c3-1 — cli (container)
context: c3-0
reverse deps: c3-101, c3-102, c3-103, c3-104, c3-105, c3-106, c3-107, c3-110
constraints from: c3-0
blocks: Complexity Assessment ✓, Components ✓, Goal ✓, Responsibilities ✓

## c3-101 — docker-client (component)
container: c3-1 | context: c3-0
files: internal/docker/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## c3-102 — classifier (component)
container: c3-1 | context: c3-0
files: internal/classifier/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ✓

## c3-103 — port-hasher (component)
container: c3-1 | context: c3-0
files: internal/ports/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## c3-104 — socat-bridge (component)
container: c3-1 | context: c3-0
files: internal/socat/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## c3-105 — caddy-manager (component)
container: c3-1 | context: c3-0
files: internal/caddy/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## c3-106 — env-generator (component)
container: c3-1 | context: c3-0
files: internal/env/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ✓

## c3-107 — daemon (component)
container: c3-1 | context: c3-0
files: internal/daemon/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## c3-110 — run-wrapper (component)
container: c3-1 | context: c3-0
files: internal/run/**
constraints from: c3-0, c3-1
blocks: Container Connection ✓, Dependencies ○, Goal ✓, Related Refs ○

## ref-naming-convention — Naming Convention (ref)
blocks: Choice ✓, Goal ✓, How ✓, Why ✓

## ref-npm-binary-distribution — npm Binary Distribution (ref)
files: npm/**
blocks: Choice ✓, Goal ✓, How ✓, Why ✓

## File Map
internal/caddy/** → c3-105
internal/classifier/** → c3-102
internal/daemon/** → c3-107
internal/docker/** → c3-101
internal/env/** → c3-106
internal/ports/** → c3-103
internal/run/** → c3-110
internal/socat/** → c3-104
npm/** → ref-npm-binary-distribution

## Ref Map
ref-naming-convention
ref-npm-binary-distribution
