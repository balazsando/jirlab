# jirlab

A terminal UI for Jira sprint boards, GitLab merge requests, Kubernetes workloads, time tracking, and Microsoft Teams chats.

---

## Table of Contents

1. [Setup](#setup)
2. [How to Use the TUI](#how-to-use-the-tui)
   - [Global Navigation](#global-navigation)
   - [Sprint Board (1)](#sprint-board-1)
   - [Repositories (2)](#repositories-2)
   - [Merge Requests (3)](#merge-requests-3)
   - [Kubernetes (4)](#kubernetes-4)
   - [Chats (5)](#chats-5)
     - [Chats panel](#chats-panel)
     - [Notes & Templates panel](#notes--templates-panel)
3. [Theming](#theming)
4. [CLI Reference](#cli-reference)
5. [Further Reading](#further-reading)

---

## Setup

### Prerequisites

Install the following tools in WSL before running jirlab:

```bash
# Git
sudo apt install git

# kubectl (for Kubernetes tab)
curl -LO "https://dl.k8s.io/release/$(curl -sL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# curl (for API health checks / manual testing)
sudo apt install curl

# wslview — opens URLs in the Windows host browser
sudo apt install wslu

# xdg-open fallback (optional)
sudo apt install xdg-utils
```

### Install

#### Option A — Download pre-built binary (recommended)

```bash
# Linux / WSL (amd64)
curl -Lo jirlab https://github.com/balazsando/jirlab/raw/refs/heads/develop/bin/jirlab
chmod +x jirlab
sudo mv jirlab /usr/local/bin/    # or ~/.local/bin/ with PATH set
```

#### Option B — Build from source

> Requires **Go 1.22+**: `sudo apt install golang-go` or download from https://golang.org/dl/

```bash
git clone https://github.com/balazsando/jirlab
cd jirlab
go mod tidy
go build -o jirlab .
sudo mv jirlab /usr/local/bin/    # or ~/.local/bin/ with PATH set
```

### Environment Variables


You can copy the provided `.env.example` file as a starting point:

```bash
cp .env.example .env
```

Then edit `.env` with your own credentials. jirlab will load environment variables from this file (or from your shell profile).

You can place the `.env` file in your home directory **or** the directory you run jirlab from, or export the variables in your `~/.zshrc`.

```bash
# Jira
export JIRA_URL="https://yourcompany.atlassian.net"
export JIRA_API_URL="https://yourcompany.atlassian.net"
export JIRA_EMAIL="you@yourcompany.com"
export JIRA_TOKEN="<jira-api-token>"       # https://id.atlassian.com/manage-profile/security/api-tokens
export JIRA_BOARD_ID="42"                  # numeric board ID from your Jira board URL

# GitLab
export GITLAB_TOKEN="<personal-access-token>"   # api scope required
export GITLAB_API_URL="https://gitlab.yourcompany.com"

# Microsoft Graph (optional — enables Chats section)
export AZURE_CLIENT_ID="<app-registration-client-id>"  # see Chats section for setup

# Optional: restrict repository scan to a specific directory (faster startup)
# Defaults to $HOME when not set
export REPOS_DIR="$HOME/projects"
```

### Folder Structure

```
$HOME/
├── .kube/
│   ├── config-test.yaml       # Kubernetes config for test env
│   ├── config-preprod.yaml    # Kubernetes config for pre-production
│   └── config-prod.yaml       # Kubernetes config for production
│   # Config files must follow the pattern: config-<name>.yaml or config-<name>.yml
│   # Priority order: test → preprod → prod → alphabetical
│
├── .jirlab/
│   └── msgraph_token.json     # MS Graph OAuth token (created on first auth)
│
├── project-a/                 # Your project repos (in $HOME)
│   ├── .gitlab-ci.yml         # Required for jirlab to discover the repo
│   ├── .jirlab/
│   │   ├── ticket/            # Ticket descriptions saved on branch creation
│   │   │   └── PROJ-42.md
│   │   └── mr/                # MR patch files downloaded from GitLab
│   │       └── PROJ-42.patch
│   └── ...
├── project-b/
│   └── .gitlab-ci.yml
```

> The repository scanner searches recursively under `$HOME` by default. Set `REPOS_DIR` to a narrower directory (e.g. `$HOME/projects`) to speed up discovery.

### Shell Navigation (zsh)

The TUI cannot change its parent shell's working directory directly. Add this wrapper function to `~/.zshrc`:

```zsh
function jirlab() {
    local nav_file="/tmp/jirlab_nav"
    rm -f "$nav_file"
    command jirlab "$@"
    if [[ -f "$nav_file" ]]; then
        local dir
        dir="$(cat "$nav_file")"
        rm -f "$nav_file"
        if [[ -n "$dir" && -d "$dir" ]]; then
            cd "$dir"
        fi
    fi
}
function jl() {
    jirlab board
}
```

After saving, reload: `source ~/.zshrc`

> **PowerShell / Windows users:** see [docs/setup_powershell.md](docs/setup_powershell.md) for the Windows build, PATH setup, and the equivalent shell navigation wrapper for PowerShell.

---

## How to Use the TUI

### Global Navigation

| Key | Action |
|-----|--------|
| `1` – `5` | Switch between tabs |
| `0` | Refresh tables |
| `?` | Open keyboard help (section-specific hotkeys + colour legend) |
| `esc` | Close modal / dismiss error |
| `q` | Quit |

Press `?` at any time to see all available hotkeys for the current section along with a colour legend.

---

### Sprint Board `1`

Shows your team's active sprint issues. Issues are colour-coded by their MR and branch status so you can see the full pipeline at a glance.

**Colour legend**

| Colour | Meaning |
|--------|---------|
| Red | Others have an open, unapproved MR for this issue |
| Green | Your MR has been approved — ready to merge |
| Yellow | You have an open MR, waiting for review |
| Orange | Others' MR is approved but not yet merged |
| Light blue | You have a local branch for this issue |
| Dark blue | Assigned to you, no branch or MR yet |
| White | No branch, no MR |

**Hotkeys**

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate issues |
| `enter` / `d` | Open issue description |
| `a` | Assign issue to yourself |
| `u` | Unassign |
| `i` | Move to **In Progress** + assign to yourself (with confirm) |
| `t` | Move to **Testing** + unassign (with confirm) |
| `r` | Open the MR for this issue in the browser |
| `w` | Open the Jira ticket in the browser |
| `m` | Merge the MR (with confirm) |
| `b` | Create a new branch for this issue across your local repos (saves ticket description to `.jirlab/ticket/<key>.md`, copies path to clipboard) |
| `n` | Navigate your shell CWD to the repo that has this issue's branch |
| `c` | Add a Jira comment |
| `l` | Log hours to this issue — opens a single-digit input modal; new log starts after the last existing log for the day |
| `f` | Toggle filter: show only your issues |
| `→` | Open command palette with all available actions |

---

### Repositories `2`

Shows all GitLab repos found under `$HOME`, with their current git branch and MR status.

**Top table colour legend** matches Sprint Board (red/green/yellow/orange for MR states, light blue for feature branch).

Each repo has subtabs (`r` branches, `t` tags, `m` MRs, `p` pipelines) with their own colour legends:

**Branches subtab**

| Colour | Meaning |
|--------|--------|
| Green | Active (currently checked-out) branch |
| Grey | Protected branch |

**MRs subtab**

| Colour | Meaning |
|--------|--------|
| Red | Others' MR, not yet approved |
| Green | Your MR, approved |
| Yellow | Your MR, waiting for review |
| Orange | Others' MR, approved |

**Pipelines subtab**

| Colour | Meaning |
|--------|--------|
| Green | Running |
| Yellow | Pending |
| Red | Failed |
| Grey | Canceled / Skipped |

**Hotkeys**

| Key | Action |
|-----|--------|
| `enter` / `n` | Navigate shell CWD to this repo (quits TUI) |
| `d` | Checkout default branch (`main` / `master`) |
| `b` | Create a new branch |
| `g` | Stage all changes, enter commit message, and push |
| `i` | Trigger a CI/CD pipeline (choose ref) |
| `c` | Create a merge request for the current branch (MR browser URL copied to clipboard) |
| `o` | Open repo in editor (choose: `c` VS Code, `i` IntelliJ IDEA, `v` vim) |
| `v` | Copy Maven version to clipboard |
| `w` | Open GitLab repo URL in browser |
| `→` | Open command palette with all available actions |

---

### Merge Requests `3`

Shows all open MRs across your local repos (both created by you and assigned to you).

**Colour legend**

| Colour | Meaning |
|--------|---------|
| Red | Others' MR, not yet approved |
| Green | Your MR, approved |
| Yellow | Your MR, waiting for review |
| Orange | Others' MR, approved |

**Hotkeys**

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate MRs |
| `enter` / `w` | Open MR in browser |
| `p` | Download MR diff patch (saved to `{repo}/.jirlab/mr/{ticket}.patch`, path copied to clipboard) |
| `c` | Checkout MR branch locally |
| `m` | Merge MR (with confirm) |
| `d` | Close MR (with confirm) |
| `→` | Open command palette with all available actions |

**MR diff patch** (`p` key) fetches all changed files from the GitLab API
(`GET /api/v4/projects/:id/merge_requests/:iid/diffs`) and saves a
`git apply`-compatible unified diff to `{local_repo}/.jirlab/mr/{ticket}.patch`
(ticket number resolved from the source branch name; falls back to `mr-{id}.patch`).
The file path is copied to clipboard. The `.jirlab/mr/` directory is created automatically.
Requires GitLab 15.7+. Needs `read_api` token scope.

---

### Kubernetes `4`

Two-pane view: **top pane** lists kubeconfig files; **bottom pane** shows pods, services, or deployments for the active config. All data auto-refreshes every 10 seconds.

**Config discovery**: jirlab looks for `~/.kube/config-*.yaml` and `~/.kube/config-*.yml`. Files with `monitoring` in the name are shown but not selectable via hotkey.

**Top pane — config table**

| Colour | Meaning |
|--------|---------|
| Green | Currently active config |
| Grey | Monitoring config (read-only view) |

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate configs |
| `enter` | Activate highlighted config |
| `t` | Activate **test** config |
| `s` | Activate **staging / preprod** config |
| `p` | Activate **production** config |
| `tab` | Switch focus to bottom resource pane |
| `→` | Open command palette with all available actions |

**Bottom pane — resource tables**

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate rows |
| `tab` | Switch focus back to config pane |
| `o` | Switch to Pods tab |
| `e` | Switch to Services tab |
| `l` | View pod logs (only in Pods tab) |
| `x` | **Exec interactive shell** inside selected pod (Pods tab only) |
| `m` | Switch to Deployments tab |
| `r` | Scale replicas (on Deployments pane) / refresh (anywhere else) |
| `d` | Describe selected resource |
| `y` | Get resource YAML |

> **Pod exec (`x`)**: suspends the TUI and opens `kubectl exec -it <pod> -- /bin/sh`
> directly in the terminal (TTY + stdin/stdout + resize), identical to the k9s UX.
> Exit the shell normally (`exit` or `ctrl+d`) to return to jirlab.

**Pod status colours**

| Colour | Meaning |
|--------|---------|
| Green | Running / Completed / Succeeded |
| Yellow | Pending / ContainerCreating / Initialising |
| Red | Failed / CrashLoopBackOff / Error / OOMKilled |

---

### Chats `5`

Two-panel view: **top panel** lists your pinned/favorited Microsoft Teams conversations; **bottom panel** is the Notes & Templates manager (see below). Press `tab` to move keyboard focus between panels.

#### Prerequisites: Azure App Registration

The Chats panel requires your own Azure App Registration (not the Microsoft Azure CLI app, which cannot be used for `Chat.Read` due to Microsoft policy). This is a one-time setup:

1. Open [Azure Portal → Microsoft Entra ID → App registrations](https://portal.azure.com/#blade/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/RegisteredApps) → **New registration**.
2. Name it anything (e.g. `jirlab`). Leave redirect URI blank. Click **Register**.
3. Under **Authentication** → **Advanced settings** → enable **Allow public client flows** → Save.
4. Under **API permissions** → **Add a permission** → **Microsoft Graph** → **Delegated** → select `Chat.Read` → Add. Then **Grant admin consent** (or ask your tenant admin).
5. Copy the **Application (client) ID** from the Overview page.
6. Add it to your `.env`:
   ```
   AZURE_CLIENT_ID=<your-application-client-id>
   ```

Without `AZURE_CLIENT_ID` set, the Chats panel will show a configuration message and the auth flow will not start.

#### Microsoft Graph Authentication

On first launch, jirlab attempts to load a saved token from `~/.jirlab/msgraph_token.json`. If no token exists, the top panel shows a placeholder row:

```
Authentication required — press Enter to authenticate
```

Pressing `enter` starts the **Device Code Flow**:

1. jirlab opens `https://login.microsoft.com/device` in your default browser.
2. The user code is copied to your clipboard automatically.
3. Paste the code in the browser and sign in with your Microsoft 365 account.
4. After sign-in, press any key in jirlab to exchange the code for a token.
5. The token is saved to `~/.jirlab/msgraph_token.json` (mode `0600`).

Subsequent launches are seamless — no browser needed.

**Chats panel**

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate chats |
| `enter` | Trigger authentication (if not authenticated) |
| `r` | Refresh chats list |
| `tab` | Move focus to Notes & Templates panel |

#### Notes & Templates panel

A persistent note and template manager stored in `~/.jirlab/notes.json` and `~/.jirlab/templates.json`.

| Colour | Meaning |
|--------|---------|
| Red | High-priority note |
| Yellow | Medium-priority note |
| Green | Low-priority note |

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate entries |
| `o` | Switch to Notes sub-pane |
| `t` | Switch to Templates sub-pane |
| `n` | Create new note / template (modal) |
| `enter` | Open note detail / edit template |
| `d` | Delete selected entry (with confirm) |
| `s` | Cycle sort: date → priority → remaining tasks (notes only) |
| `f` | Cycle category filter (notes only) |
| `tab` | Move focus back to Chats panel |

**Creating a note** — the create modal (`n`) has five tab-switchable fields:
- **Title** (required)
- **Category** — type freely or reuse an existing one shown as a hint
- **Priority** — `←` / `→` to cycle Low / Med / High
- **Description** — free-form reminder text
- **Tasks** — `enter` adds each task; `ctrl+d` removes the last one

When all tasks in a note are completed, jirlab prompts: **Keep** (`k`) or **Delete** (`d`).

The active filter and sort mode are shown in the panel header:
`── Notes (5) [work] [priority] ──────`

> **Log worklogs from Board**: Time logs are entered from the Sprint Board (`l` key) and are stored in Jira. The `l` key opens a single-digit hours input modal; the new log starts immediately after the last existing log for the current day (preventing overlap).

---

## Theming

jirlab ships with a **Catppuccin Mocha** colour palette baked in. All foreground and
background colours are taken from the official Mocha palette, making it harmonise
naturally with any Catppuccin-themed terminal emulator, shell prompt, or editor.

| Element | Colour |
|---------|--------|
| Active tab / borders | Mauve `#cba6f7` |
| Focused selection row | Dark mauve `#3d2452` |
| Unfocused selection row | Surface0 `#313244` |
| Column headers | Blue `#89b4fa` |
| Status bar / footer | Subtext1 `#bac2de` on Surface0 |
| Error / high priority | Red `#f38ba8` |
| Warning / medium priority | Yellow `#f9e2af` |
| Success / low priority | Green `#a6e3a1` |
| New MR comment (24 h) | Pink `#f5c2e7` |

No runtime configuration is needed — the theme is compiled in.

---

## CLI Reference

```
jirlab [flags]

Flags:
  --debug     Write debug log to /tmp/jirlab-debug.log
  -h, --help  Show help
```

jirlab reads configuration from environment variables or a `.env` file located in `$PWD` or `$HOME`. No subcommands are needed — the TUI covers all functionality.

---

## Further Reading

- [Architecture & technical design](docs/architecture.md)
- [ADR 001 — Technology choices](docs/adr/001-technology-choices.md)
- [ADR 002 — Integration layer pattern](docs/adr/002-integration-layer.md)
- [ADR 003 — Kubernetes integration design](docs/adr/003-kubernetes-integration.md)

