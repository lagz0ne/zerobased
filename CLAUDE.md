# CLAUDE.md

## Zerobased Rewrite Guardrails

This repo is in a clean-slate rewrite toward an explicit machine-local control plane with traced-TDD failure ownership.

The current target is not the removed legacy Docker autodiscovery router. Do not reintroduce the old default model: Docker event watching, `zerobased run`, routefile-driven routing, classifier-based exposure, legacy env discovery, or repo-local direct Caddy mutation.

`zerobased start` and `zerobased up` are the only first-class product boundaries for new architecture work. Other commands may support those boundaries later, but must not define a parallel product model.

`zerobased up` uses explicit project-root `zerobased.yaml` v1 only. The file starts with `version: 1`, declares `name`, optional `compose`, `processes`, readiness, and `routes`. Do not infer Compose files, Docker containers, routefiles, ports, or env from the directory.

Compose support is explicit and current. Use:

```yaml
compose:
  files:
    - compose.yaml
  services:
    - postgres
  ownership: owned
```

The user Compose file is a base model. Zerobased owns the runtime Compose project name, injects `dev.zerobased.*` labels, and rejects global escape hatches before containers start: `container_name`, host `ports`, `network_mode: host`, custom top-level network/volume names, and `external` resources in `owned`.

Use C3 before exploration or changes. If C3 or older docs describe legacy autodiscovery as the product shape, treat that as stale context to reconcile against the control-plane RFC, not as target architecture.

Use RED-GREEN-TDD for behavior changes. Tests must follow traced-TDD: each failure belongs to one owning layer contract, lower-layer behavior is tested at the lower layer, and upper layers test only their declared transformed or propagated outcomes.

Repo-local `zerobased up` must not mutate Caddy directly. It claims through the daemon before starting Compose or local processes, waits for readiness, publishes desired routes to the daemon, and remains foreground until stopped.
