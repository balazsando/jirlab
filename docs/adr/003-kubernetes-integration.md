# ADR 003 — Kubernetes Integration Design

**Date:** 2025  
**Status:** Accepted

## Context

Jirlab needed a live Kubernetes view so developers can check pod health and deployment state without leaving the terminal. The key questions were:

1. How to talk to Kubernetes (client library vs. CLI)?
2. How to handle multiple clusters/environments?
3. How to keep the view fresh without blocking the TUI?
4. How to support scaling deployments inline?

## Decisions

### kubectl CLI subprocess (not a Go client library)

`kubectl` is invoked via `exec.Command` (wrapped by `KubectlService`) rather than using the official `client-go` library because:

- `client-go` adds significant binary size and dependency surface.
- `kubectl` is already a prerequisite on the developer's machine.
- The subset of operations needed (get, describe, logs, scale) maps directly to simple CLI flags with JSON output.
- Output is parsed from the `kubectl get <resource> -o json` format, which is stable across versions.

**Alternative considered:** `client-go` — rejected due to complexity and binary bloat.

### Kubeconfig discovery via glob pattern

jirlab discovers cluster configs by globbing `~/.kube/config-*.yaml` and `~/.kube/config-*.yml`. This convention:

- Keeps the standard `~/.kube/config` untouched (not overwritten or merged).
- Makes adding/removing a cluster as simple as dropping a file.
- Allows optional `monitoring` suffix convention to mark read-only observability configs.

Configs are sorted with `test` → `preprod` → `prod` priority for keyboard shortcuts `t` / `s` / `p`.

### Two-pane layout with shared tab bar widths

The Kubernetes tab uses a split view: a top **config table** and a bottom **resource table** (Pods / Services / Deployments). The bottom tab bar is rendered at the same pixel width as the first three top-level app tabs so the two tab bars are visually aligned.

Focus alternates between panes with `tab`. When the top pane is focused, the bottom table rows have no background highlight (no ghost selection).

### 10-second auto-refresh via tea.Every

Resources are refreshed every 10 seconds using Bubble Tea's `tea.Every` tick. Existing data is kept visible during background refresh — a loading spinner only appears on the very first fetch (when there is nothing to show yet). Manual refresh with `r` also preserves existing data.

### Inline scaling (deployments pane, `r` key)

Scaling a deployment is triggered by `r` while the Deployments pane is focused. This choice:

- Avoids a dedicated hotkey that would conflict with the staging shortcut (`s`).
- Uses an `InputModal` → `ConfirmModal` flow consistent with other destructive actions (merge, close MR).
- Falls back to a full refresh when `r` is pressed outside the Deployments pane.

## Consequences

- `kubectl` must be installed and on `PATH` inside the execution environment (WSL or native Linux).
- The `KUBECONFIG` environment variable is set on the jirlab process for the active cluster; this does not affect the parent shell.
- Monitoring configs (read-only) are shown in grey and cannot be activated via `t`/`s`/`p` hotkeys.
- Pod status is resolved from container `state.waiting.reason` / `state.terminated.reason` first, falling back to the pod phase. This gives accurate error names (e.g. `CrashLoopBackOff`) rather than the generic `Running` phase.
