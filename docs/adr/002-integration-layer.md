# ADR 002 — Integration Layer Pattern

**Date:** 2025  
**Status:** Accepted

## Context

The TUI sections (board, repos, MRs, kube, tracker) need to call external systems: Jira REST API, GitLab REST API, git CLI, kubectl CLI, the OS clipboard, and the OS browser. Mixing these calls directly in Bubble Tea update functions would:

- Make the code hard to test (no way to mock a subprocess or HTTP call).
- Couple platform-specific logic (WSL paths, clipboard tools) to UI business logic.
- Make it impossible to swap implementations (e.g. replace `wslview` with `xdg-open`, or support PowerShell in the future).

## Decision

All external calls are hidden behind **Go interfaces** in the `internal/integration/` package:

| Interface | Responsibilities |
|---|---|
| `JiraService` | Fetch issues, transitions, worklogs; assign; comment |
| `GitLabService` | Fetch MRs; merge; close; download patches |
| `GitCmdService` | Checkout branches; write nav path; scan repos |
| `KubectlService` | Run `kubectl get/describe/logs/scale` subprocesses |
| `FilesystemService` | `MkdirAll` / `WriteFile` (patch download) |
| `ShellService` | Open browser; copy to clipboard; resolve editor paths |

TUI sections **only hold interface values** — never concrete types or `exec.Command` calls. Each section receives its services via its constructor.

`app.go` is the single composition root: it calls `integration.New*()` factories and wires everything together.

## Consequences

- All TUI section logic is unit-testable with mock implementations.
- Platform-specific code (WSL browser, JetBrains path discovery) is isolated to `internal/integration/`.
- Adding PowerShell support requires only a new `windowsShellService` implementing `ShellService` — zero TUI changes.
- The interface boundary makes the dependency graph explicit and prevents import cycles.
