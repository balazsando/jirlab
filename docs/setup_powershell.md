# jirlab — PowerShell Setup Guide

This guide covers building and running **jirlab** on Windows using PowerShell 7+.
For the WSL/zsh setup see the [Shell Navigation (zsh)](../README.md#shell-navigation-zsh) section in the main README.

---

## Prerequisites

You can install all prerequisites from PowerShell without admin rights (no GUI/manual downloads needed):

### Git for Windows (portable, no admin)
```powershell
# Download the portable Git ZIP (change version as needed)
$gitVersion = "2.44.0.windows.1"
$gitZip = "PortableGit-$gitVersion-64-bit.7z.exe"
$url = "https://github.com/git-for-windows/git/releases/download/v2.44.0.windows.1/$gitZip"
Invoke-WebRequest -Uri $url -OutFile $gitZip
# Extract to $HOME\bin\git (7-Zip self-extractor, no admin needed)
& .\$gitZip -o"$HOME\bin\git" -y
Remove-Item $gitZip
# Add Git to PATH for current session
$env:PATH = "$HOME\bin\git\cmd;" + $env:PATH
# Add Git to PATH permanently (run once)
[System.Environment]::SetEnvironmentVariable(
    "PATH",
    "$HOME\bin\git\cmd;" + [System.Environment]::GetEnvironmentVariable("PATH", "User"),
    "User"
)
```

If you do not have 7-Zip, download the self-extracting `.exe` version as above and run it directly. All files are extracted to your user directory—no admin required.

### kubectl (user install, no admin)
```powershell
$kubectlVersion = (Invoke-RestMethod -Uri "https://dl.k8s.io/release/stable.txt")
$kubectlUrl = "https://dl.k8s.io/release/$kubectlVersion/bin/windows/amd64/kubectl.exe"
Invoke-WebRequest -Uri $kubectlUrl -OutFile "$HOME\bin\kubectl.exe"
# Add $HOME\bin to PATH if not already
[System.Environment]::SetEnvironmentVariable(
    "PATH",
    "$HOME\bin;" + [System.Environment]::GetEnvironmentVariable("PATH", "User"),
    "User"
)
```


### PowerShell 7+ (portable, no admin)
```powershell
# Download the latest PowerShell 7 ZIP (change version as needed)
$psVersion = "7.4.2"
$psZip = "PowerShell-$psVersion-win-x64.zip"
$url = "https://github.com/PowerShell/PowerShell/releases/download/v$psVersion/$psZip"
Invoke-WebRequest -Uri $url -OutFile $psZip
Expand-Archive $psZip -DestinationPath "$HOME\bin\pwsh" -Force
Remove-Item $psZip
# Launch portable PowerShell 7:
$HOME\bin\pwsh\pwsh.exe
```

You can create a shortcut or alias to `$HOME\bin\pwsh\pwsh.exe` for convenience. No admin rights are required.

---

## Install

### Option A — Download pre-built binary (recommended)

```powershell
# Download jirlab.exe directly (no Go required)
Invoke-WebRequest -Uri "https://github.com/balazsando/jirlab/raw/refs/heads/develop/bin/jirlab.exe" -OutFile "$HOME\bin\jirlab.exe"
```

Then continue to the **Add to PATH** section below.

### Option B — Build from source

> Requires **Go 1.22+** — install it first if you don't already have it:

```powershell
# Download Go (change version as needed)
$goVersion = "1.22.3"
$goZip = "go$goVersion.windows-amd64.zip"
Invoke-WebRequest -Uri "https://go.dev/dl/$goZip" -OutFile $goZip
Expand-Archive $goZip -DestinationPath "$HOME" -Force
Remove-Item $goZip
[System.Environment]::SetEnvironmentVariable(
    "PATH",
    "$HOME\go\bin;" + [System.Environment]::GetEnvironmentVariable("PATH", "User"),
    "User"
)
```

```powershell
git clone https://github.com/balazsando/jirlab
cd jirlab
go mod tidy
make build-powershell          # produces jirlab.exe
```

Or without make:
```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -tags powershell -o jirlab.exe .
```

---

## Add to PATH

Copy `jirlab.exe` to a directory that is on your `$PATH`. For example:

```powershell
# Create a personal bin dir if needed
New-Item -ItemType Directory -Force "$HOME\bin"
Copy-Item .\jirlab.exe "$HOME\bin\"

# Add to PATH permanently (run once)
[System.Environment]::SetEnvironmentVariable(
    "PATH",
    "$HOME\bin;" + [System.Environment]::GetEnvironmentVariable("PATH", "User"),
    "User"
)
```

Restart your terminal after updating `PATH`.

---

## Environment Variables



You can copy the provided `.env.example` file as a starting point:

```powershell
Copy-Item .env.example .env
```

Then edit `.env` with your own credentials. jirlab will load environment variables from this file (or from your PowerShell profile).

You can set environment variables in your PowerShell profile (`$PROFILE`) **or** by creating a `.env` file in your home directory or the directory you run jirlab from. Both methods are supported.

Add the following to your PowerShell profile (`$PROFILE`) or `.env` file:

```powershell
# Jira
$env:JIRA_URL       = "https://yourcompany.atlassian.net"
$env:JIRA_API_URL   = "https://yourcompany.atlassian.net/rest/api/3"
$env:JIRA_EMAIL     = "you@yourcompany.com"
$env:JIRA_TOKEN     = "your-jira-api-token"
$env:JIRA_BOARD_ID  = "123"

# GitLab
$env:GITLAB_TOKEN   = "glpat-xxxxxxxxxxxxxxxxxxxx"
$env:GITLAB_API_URL = "https://gitlab.yourcompany.com/api/v4"

# Optional: restrict repository scan to a specific directory (faster startup)
# Defaults to $HOME when not set
$env:REPOS_DIR      = "$HOME\projects"
```

Edit `$PROFILE` with:
```powershell
notepad $PROFILE
```
Then reload: `. $PROFILE`

---

## Shell Navigation Wrapper

**jirlab** writes the target path to `%TEMP%\jirlab_nav.txt` when you press `n` (navigate) or `enter` on a repo. To have PowerShell actually change directory after the TUI exits, add this function to your `$PROFILE`:

```powershell
function jirlab {
    $navFile = "$env:TEMP\jirlab_nav.txt"
    Remove-Item -ErrorAction SilentlyContinue $navFile
    & "$HOME\bin\jirlab.exe" @args
    if (Test-Path $navFile) {
        $dir = Get-Content $navFile -Raw
        $dir = $dir.Trim()
        Remove-Item $navFile -ErrorAction SilentlyContinue
        if ($dir -and (Test-Path $dir -PathType Container)) {
            Set-Location $dir
        }
    }
}
function jl {
    jirlab board
}
```

Reload your profile: `. $PROFILE`

---

## Repository Layout

Repositories must contain a `.gitlab-ci.yml` file. By default jirlab scans recursively under `$HOME`. Set `REPOS_DIR` to a narrower directory (e.g. `$HOME\projects`) for faster startup.

```
C:\Users\you\
  my-service\
    .gitlab-ci.yml
    ...
  another-service\
    .gitlab-ci.yml
    ...
```

---

## Kubernetes

Place kubeconfig files at:

```
%USERPROFILE%\.kube\config-test.yaml
%USERPROFILE%\.kube\config-preprod.yaml
%USERPROFILE%\.kube\config-prod.yaml
```

---

## Opening URLs and Clipboard

The PowerShell build uses:
- **Browser:** `explorer.exe <url>` (opens in the default Windows browser)
- **Clipboard:** `clip.exe` (built into Windows)

No additional tools are required.
