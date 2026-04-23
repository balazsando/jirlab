# ADR 001 — Technology Choices

**Date:** 2024  
**Status:** Accepted

## Context

jirlab is a personal productivity tool for developers working with Jira and GitLab from the terminal. The goals were:

- Fast, keyboard-driven workflow
- No browser context switches for common actions
- Runs on Linux and macOS
- Minimal external dependencies

## Decisions

### Bubble Tea for TUI

[Bubble Tea](https://github.com/charmbracelet/bubbletea) was chosen as the TUI framework because:

- The Elm-like architecture (Model/Update/View) scales well for a multi-tab layout.
- It handles async commands cleanly via `tea.Cmd → tea.Msg`.
- It is widely maintained and has good community support.

**Alternatives considered:** `tview` (imperative, harder to test), raw `termbox-go` (too low-level).

### Lip Gloss for Styling

[Lip Gloss](https://github.com/charmbracelet/lipgloss) provides declarative styling compatible with Bubble Tea and supports 256-color terminals.

### Cobra + Viper for CLI/Config

Standard Go CLI stack. Cobra provides subcommands; Viper loads `.env` files transparently.

### REST API Clients (hand-rolled)

Jira and GitLab clients are hand-rolled using `net/http` rather than generated SDKs:

- Lighter binary.
- Only the subset of endpoints used are implemented.
- Full control over request/response handling.

### No database

All state is fetched on startup or on refresh. There is no local cache or database. Simplicity is preferred over offline capability.

### Shell Navigation via /tmp file

A TUI process cannot change its parent shell's `CWD`. Rather than using a FIFO or dbus, jirlab writes the target path to `/tmp/jirlab_nav` and quits. The shell wrapper reads this file after the process exits. This approach:

- Requires only a short shell function in `~/.zshrc`.
- Works on any POSIX shell.
- Has no runtime dependencies.

## Consequences

- The tool requires a terminal with 256-color support.
- GitLab and Jira credentials must be available as environment variables.
- Navigation requires the shell wrapper to be installed.
