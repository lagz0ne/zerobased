# AGENTS.md

## Rewrite-State Guardrails

Zerobased is being rewritten as an explicit local control plane, not extended as the legacy Docker autodiscovery router. Do not continue legacy behavior by default.

Hard boundaries:

- First-class product commands are `zerobased start` and `zerobased up`.
- Project config is explicit `zerobased.yaml` v1 only. It must start with `version: 1`.
- Legacy Docker autodiscovery, `zerobased run`, `zerobased.routes.yaml`, classifier-based exposure, and legacy env discovery are not the target architecture.
- Compose support is current but explicit: only load files declared under `compose.files` in `zerobased.yaml`.
- User Compose files are base models. Zerobased owns the generated Compose project name and `dev.zerobased.*` labels.
- In `owned` Compose mode, reject global escape hatches before container creation: `container_name`, host `ports`, `network_mode: host`, custom top-level network/volume names, and `external` resources.
- New work should move toward daemon-owned claims, owner records, desired-state publication, and traced-TDD contracts.
- Repo-local `up` claims before process start, waits for declared readiness, publishes routes through the daemon-owned control plane, and stays foreground until stopped.

Work rules:

- Use C3 before exploration or code changes.
- Use RED-GREEN-TDD for every behavior change.
- Place tests by traced-TDD failure ownership: the layer that declares or transforms an error owns the test.
- If existing code/docs conflict with these guardrails, prefer the clean-slate control-plane RFC and update stale context as part of the work.
