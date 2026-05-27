# Todo

## Rewrite State

- [done] Write the product RFC for zerobased as a machine-local control plane: `docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md`.
- [done] Run iterative subagent review on the control-plane RFC until no medium-or-higher issues remained across edge cases, performance, and realism.
- [done] Clean-slate the repo for the rewrite: remove legacy implementation paths, add rewrite guardrails, and leave a minimal buildable CLI stub.

## Next

- [done] Write the separate traced-TDD RFC from the revised start/up ownership graph before adding behavior.
- [done] Review the traced-TDD RFC with subagents until no medium-or-higher issues remain.
- [done] Write the implementation plan from the traced-TDD graph before coding the control plane: `docs/superpowers/plans/2026-05-14-error-kernel-implementation.md`.
- [done] Implement first rewrite foundation slice: typed error kernel and static capability boundary tests.
- [done] Before adding any runtime behavior package, write the layer acknowledgment coverage table that maps each consumed-error row to a named test id: `docs/superpowers/specs/zerobased-acknowledgment-coverage.yaml`.
- [done] Add RED build-tagged system acceptance harness for `start`/`up`: run with `go test -tags system ./internal/systemtest`.
- [done] Implement the minimal foreground `zerobased start` daemon boot path to make `TestStartCreatesDaemonLockSocketAndHealth` pass.
- [done] Implement the minimal `zerobased up` path to make `TestUpPublishesOnlyAfterProcessReadiness` pass.
- [done] Design a Compose-backed orchestration layer that uses `compose-go` to load the user model, inject zerobased runtime policy, and feed the result into the Compose SDK instead of string-bashing YAML or shelling out blindly: `docs/superpowers/specs/2026-05-26-compose-backed-orchestration-design.md`.
- [done] Decide the runtime model boundary: user `compose.yaml` is a template/base model; `zerobased.yaml` v1 owns intent; zerobased generates the Compose project name, injects `dev.zerobased.*` labels, rejects global escape hatches in the first slice, and keeps Docker Compose SDK usage behind an internal adapter.
- [done] Implement slice 1 of Compose-backed orchestration with RED-GREEN-TDD: strict loader/validator/adapter, explicit files only, generated project identity, injected labels, profile/service selection with unused resource pruning, and rejection of `container_name`, host `ports`, `network_mode: host`, custom top-level resource names, `external` resources in `owned`, and provider-managed services unless `reused`.
- [done] Add orchestrator tests with a fake Compose backend: claim before Compose start, Compose before dependent local processes, cleanup on startup failure, release after cleanup, no default Compose file discovery, and Compose error transforms into stack-orchestrator typed errors.
- [pending] Implement Compose-backed readiness and endpoint metadata: declared TCP/HTTP probes, declared endpoint metadata only, and no scanning of all exposed ports.
- [pending] Replace the stock `caddy:2-alpine` assumption with a custom Caddy build/image that includes `github.com/mholt/caddy-l4`, so `AddTCPRoute` becomes real layer-4 proxying instead of the current placeholder path.
- [pending] Evaluate embedded Caddy as an alternative to the external/containerized runtime. Main open risk: embedded zerobased would need host permission to bind low ports like `:80` for `*.localhost`, which is currently hidden by the Caddy container.

## Verification Notes

- 2026-05-26: Compose design reviewed by two subagents. No medium-or-higher unresolved issues remained after folding in: reject-not-rewrite for first slice, generated Compose identity independent from `.env`/`COMPOSE_PROJECT_NAME`/top-level `name`, explicit files only, `owned` first, `dev.zerobased.*` labels, pruning inactive-profile resources, and Compose SDK isolation behind `internal/composebackend`.
- 2026-05-26: `go test ./...` passed after the design/doc update.
- 2026-05-26: `git diff --check` passed after the design/doc update.
- 2026-05-26: C3 lookup maps the Compose design spec to `c3-109`. `c3x check --json` still fails on pre-existing stale legacy C3 components missing required schema sections; this is C3 topology debt, not a new Compose spec validation failure.
- 2026-05-26: Compose slice 1 added `internal/composebackend` with compose-go model loading, Docker Compose SDK lifecycle isolation, generated project identity, label injection, fail-fast isolation validation, and `up` orchestration wiring.
- 2026-05-26: Review pass found and fixed Compose slice issues: unsupported `reused` is rejected, partial Compose `Up` failures roll back with `Down`, unimplemented nested `profiles`/endpoint/readiness shape is rejected by strict YAML, `.env` interpolation is covered while project identity stays generated, configs/secrets isolation is covered, CLI traced-TDD coverage no longer overclaims, and Docker Compose `Up` receives project/services/remove-orphans options.
- 2026-05-26: Final verification passed: `go test ./...`, `go test -tags system ./internal/systemtest -count=1`, `CGO_ENABLED=1 go test -race ./...`, `CGO_ENABLED=1 go test -race -tags system ./internal/systemtest -count=1`, `make build && ./bin/zerobased help`, `git diff --check`, and C3 lookup for `internal/composebackend/backend.go`, `internal/up/runner.go`, and the Compose design spec all map to `c3-109`.
- 2026-05-27: Local acountee smoke found two real runtime gaps and fixed them with RED-GREEN-TDD: loaded Compose services now receive Docker Compose SDK `CustomLabels` so `Up`/`Start`/`Down` can find owned containers, and default `zerobased up` cleanup timeout is now 30s so normal database shutdown does not fail at 2s. Review pass then fixed graceful local process cleanup and bounded Compose rollback after partial `Up` failure. Sanitized acountee-shaped smoke (`postgres` + `nats` + one foreground process + route runtime) started 2 containers, published route PUT after readiness, handled SIGINT, deleted the route, and left 0 zerobased-labeled containers.
