# Error Kernel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first executable rewrite foundation: typed traced-TDD errors/status declarations and static capability boundary tests.

**Architecture:** `internal/zberr` owns all layer/code/status declarations and the reusable error type. `internal/archtest` owns static architecture tests that read the RFC capability allowlist and block removed legacy behavior before runtime code lands. No daemon, Caddy, Compose, or process orchestration behavior is added in this slice.

**Tech Stack:** Go standard library, `go test`, YAML capability artifact under `docs/superpowers/specs/zerobased-capabilities.yaml`.

---

## File Structure

- Create `internal/zberr/error.go`: core `Layer`, `Code`, `Kind`, `Error`, constructors, options, and helpers.
- Create `internal/zberr/registry.go`: declared layer/code/status registry from the traced-TDD RFC.
- Create `internal/zberr/error_test.go`: RED/GREEN tests for error behavior, origin preservation, declaration uniqueness, and status-vs-error shape.
- Create `internal/archtest/capabilities_test.go`: static capability allowlist tests against current repo files.
- Modify `tasks/todo.md`: mark this first implementation slice in progress/done.

## Task 1: Typed Error Kernel

**Files:**
- Create: `internal/zberr/error_test.go`
- Create: `internal/zberr/error.go`
- Create: `internal/zberr/registry.go`

- [ ] **Step 1: Write the failing tests**

```go
package zberr

import (
	"errors"
	"testing"
)

func TestErrorCarriesLayerCodePhaseOwnerCauseAndOrigin(t *testing.T) {
	cause := errors.New("disk is gone")

	err := New(LayerControlPlane, CodeStoreUnavailable,
		WithPhase("store"),
		WithOwner("stack-1"),
		WithCause(cause),
		WithOrigin(LayerControlPlaneStore),
	)

	if err.Layer != LayerControlPlane {
		t.Fatalf("Layer = %q, want %q", err.Layer, LayerControlPlane)
	}
	if err.Code != CodeStoreUnavailable {
		t.Fatalf("Code = %q, want %q", err.Code, CodeStoreUnavailable)
	}
	if err.Phase != "store" {
		t.Fatalf("Phase = %q, want store", err.Phase)
	}
	if err.Owner != "stack-1" {
		t.Fatalf("Owner = %q, want stack-1", err.Owner)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is did not unwrap cause")
	}
	if len(err.Origin) != 1 || err.Origin[0] != LayerControlPlaneStore {
		t.Fatalf("Origin = %#v, want [%q]", err.Origin, LayerControlPlaneStore)
	}
	if !Is(err, LayerControlPlane, CodeStoreUnavailable) {
		t.Fatalf("Is did not match layer/code")
	}
}

func TestCriticalDefaultsOriginToOwningLayer(t *testing.T) {
	err := Critical(LayerDaemonIPC, errors.New("panic"))

	if err.Code != CodeCritical {
		t.Fatalf("Code = %q, want %q", err.Code, CodeCritical)
	}
	if len(err.Origin) != 1 || err.Origin[0] != LayerDaemonIPC {
		t.Fatalf("Origin = %#v, want [%q]", err.Origin, LayerDaemonIPC)
	}
}

func TestDeclarationsAreUniqueAndEveryLayerHasCritical(t *testing.T) {
	if err := ValidateDeclarations(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationDegradedIsStatusNotError(t *testing.T) {
	decl, ok := Lookup(LayerControlPlane, CodePublicationDegraded)
	if !ok {
		t.Fatalf("PublicationDegraded declaration missing")
	}
	if decl.Kind != KindStatus {
		t.Fatalf("Kind = %q, want %q", decl.Kind, KindStatus)
	}
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/zberr`

Expected: FAIL because `New`, `LayerControlPlane`, `CodeStoreUnavailable`, `Critical`, `ValidateDeclarations`, `Lookup`, and related declarations do not exist.

- [ ] **Step 3: Implement minimal kernel**

Implement `internal/zberr/error.go` with:

```go
package zberr

import "errors"

type Layer string
type Code string
type Kind string

const (
	KindError  Kind = "error"
	KindStatus Kind = "status"
)

type Error struct {
	Layer  Layer
	Code   Code
	Phase  string
	Owner  string
	Cause  error
	Origin []Layer
}

type Option func(*Error)
```

Add `New`, `Critical`, `WithPhase`, `WithOwner`, `WithCause`, `WithOrigin`, `Error`, `Unwrap`, and `Is`.

- [ ] **Step 4: Implement minimal registry**

Implement `internal/zberr/registry.go` with all layer constants and all declared error/status codes from `docs/superpowers/specs/2026-05-13-zerobased-traced-tdd-rfc.md`.

The registry must include `PublicationDegraded` as `KindStatus`; all other declared layer/code pairs are `KindError`.

- [ ] **Step 5: Run GREEN**

Run: `go test ./internal/zberr`

Expected: PASS.

## Task 2: Static Capability Boundary Tests

**Files:**
- Create: `internal/archtest/capabilities_test.go`
- Read: `docs/superpowers/specs/zerobased-capabilities.yaml`

- [ ] **Step 1: Write the failing tests**

Add tests that:

- parse `zerobased-capabilities.yaml`
- assert capability ids are unique
- assert command-surface capability rejects `zerobased run`, `Use: "run"`, `Use: 'run'`, `case "run":`, and `case 'run':`
- assert current source files outside `allowed_packages` do not contain forbidden command strings or route-runtime symbols
- assert current source files outside `allowed_packages` do not import forbidden legacy packages such as `internal/routes`, `internal/classifier`, `internal/env`, or `internal/docker`
- assert repo-local `up` packages do not import route runtime packages when those packages are added later
- assert runtime behavior package directories do not exist before their acknowledgment coverage table is written

- [ ] **Step 2: Run RED**

Run: `go test ./internal/archtest`

Expected: FAIL until the parser/test implementation exists.

- [ ] **Step 3: Implement tests using only standard library**

Use a narrow parser for this artifact instead of adding a YAML dependency. The test only needs `id`, `forbidden_imports`, and `forbidden_symbols` list items.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/archtest`

Expected: PASS against the clean-slate tree.

## Behavior Acknowledgment Coverage Gate

This slice intentionally adds no runtime behavior packages. Before any package such as `internal/daemon`, `internal/controlplane`, `internal/orchestrator`, `internal/route_runtime`, `internal/composebackend`, `internal/process`, `internal/readiness`, `internal/socketbridge`, `internal/auxpub`, or `internal/fabric` lands, the implementer must add a row-to-test acknowledgment coverage table for that layer.

The static architecture test in this slice enforces that gate by failing if those behavior package directories contain Go source before acknowledgment coverage is created.

## Task 3: Verification And Handoff

**Files:**
- Modify: `tasks/todo.md`

- [ ] **Step 1: Mark first slice complete**

Update `tasks/todo.md` so the implementation plan item is done and the typed error kernel/static boundary slice is recorded as complete.

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
make build
./bin/zerobased help
C3X_MODE=agent bash /home/lagz0ne/.agents/skills/c3/bin/c3x.sh check --json
C3X_MODE=agent bash /home/lagz0ne/.agents/skills/c3/bin/c3x.sh coverage
```

Expected:

- all Go tests pass
- build exits zero
- help still reports rewrite state and `v0.7.0-dirty`
- C3 check has no issues
- C3 coverage remains 100%

## Self-Review

- Spec coverage: covers RFC implementation order items 1-3 only. It intentionally does not implement daemon, store, route runtime, orchestration, Caddy, or Compose behavior.
- Placeholder scan: no task relies on TODO/TBD behavior.
- Type consistency: package name is `zberr`, core declarations use `Layer`, `Code`, `Kind`, `Error`, and `Declaration`.
