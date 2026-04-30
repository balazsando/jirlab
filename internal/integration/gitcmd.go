package integration

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/andob/jirlab/internal/service"
)

// GitCmdService abstracts all git subprocess invocations so the TUI and service
// layers are not directly coupled to the git binary.
type GitCmdService interface {
	// GetCurrentBranch returns the current checked-out branch in repoPath.
	GetCurrentBranch(repoPath string) (string, error)
	// GetStats returns staged/unstaged/untracked counts for repoPath.
	GetStats(repoPath string) (service.GitStats, error)
	// GetRemoteURL returns the remote origin URL for repoPath.
	GetRemoteURL(repoPath string) (string, error)
	// Stash runs `git stash push` and reports whether anything was stashed.
	Stash(repoPath string) (bool, error)
	// StashPop runs `git stash pop`. Returns an error when there are conflicts.
	StashPop(repoPath string) error
	// StashDrop runs `git stash drop`.
	StashDrop(repoPath string) error
	// Checkout checks out the given branch in repoPath.
	Checkout(repoPath, branch string) error
	// Pull runs `git pull --ff-only` in repoPath.
	Pull(repoPath string) error
	// CreateAndCheckoutBranch creates and checks out a new branch in repoPath.
	CreateAndCheckoutBranch(repoPath, branch string) error
	// Fetch runs `git fetch <remote> <ref>` in repoPath.
	Fetch(repoPath, remote, ref string) error
	// FetchAll runs `git fetch --all --prune` in repoPath to update all remotes.
	FetchAll(repoPath string) error
	// WriteNavPath writes the path to /tmp/jirlab_nav for the shell wrapper cd.
	WriteNavPath(repoPath string) error
	// StageAllCommitPush stages all changes, commits with message, and pushes.
	// If the branch has no upstream tracking ref, --set-upstream origin is used.
	StageAllCommitPush(repoPath, message string) error
}

type gitCmdService struct{}

// NewGitCmdService returns a GitCmdService backed by the local git binary.
func NewGitCmdService() GitCmdService {
	return &gitCmdService{}
}

func (g *gitCmdService) GetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *gitCmdService) GetStats(repoPath string) (service.GitStats, error) {
	out, err := exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	if err != nil {
		return service.GitStats{}, fmt.Errorf("git status: %w", err)
	}
	var stats service.GitStats
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			stats.Untracked++
			continue
		}
		if x != ' ' {
			stats.Staged++
		}
		if y == 'M' || y == 'D' || y == 'T' {
			stats.Unstaged++
		}
	}
	return stats, nil
}

func (g *gitCmdService) GetRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *gitCmdService) Stash(repoPath string) (bool, error) {
	out, err := exec.Command("git", "-C", repoPath, "stash", "push", "-m", "jirlab-auto").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git stash push: %w", err)
	}
	return !strings.Contains(string(out), "No local changes to save"), nil
}

func (g *gitCmdService) StashPop(repoPath string) error {
	if err := exec.Command("git", "-C", repoPath, "stash", "pop").Run(); err != nil {
		return fmt.Errorf("git stash pop: %w", err)
	}
	return nil
}

func (g *gitCmdService) StashDrop(repoPath string) error {
	return exec.Command("git", "-C", repoPath, "stash", "drop").Run()
}

func (g *gitCmdService) Checkout(repoPath, branch string) error {
	if err := exec.Command("git", "-C", repoPath, "checkout", branch).Run(); err != nil {
		return fmt.Errorf("git checkout %s: %w", branch, err)
	}
	return nil
}

func (g *gitCmdService) Pull(repoPath string) error {
	return exec.Command("git", "-C", repoPath, "pull", "--ff-only").Run()
}

func (g *gitCmdService) CreateAndCheckoutBranch(repoPath, branch string) error {
	if err := exec.Command("git", "-C", repoPath, "checkout", "-b", branch).Run(); err != nil {
		return fmt.Errorf("git checkout -b %s: %w", branch, err)
	}
	return nil
}

func (g *gitCmdService) Fetch(repoPath, remote, ref string) error {
	if err := exec.Command("git", "-C", repoPath, "fetch", remote, ref).Run(); err != nil {
		return fmt.Errorf("git fetch %s %s: %w", remote, ref, err)
	}
	return nil
}

func (g *gitCmdService) FetchAll(repoPath string) error {
	if err := exec.Command("git", "-C", repoPath, "fetch", "--all", "--prune").Run(); err != nil {
		return fmt.Errorf("git fetch --all --prune: %w", err)
	}
	return nil
}

func (g *gitCmdService) StageAllCommitPush(repoPath, message string) error {
	if err := exec.Command("git", "-C", repoPath, "add", "-A").Run(); err != nil {
		return fmt.Errorf("git add -A: %w", err)
	}
	if err := exec.Command("git", "-C", repoPath, "commit", "-m", message).Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	out, err := exec.Command("git", "-C", repoPath, "push").CombinedOutput()
	if err == nil {
		return nil
	}
	// Branch has no upstream tracking ref — set it automatically.
	if strings.Contains(string(out), "no upstream") ||
		strings.Contains(string(out), "has no upstream") ||
		strings.Contains(string(out), "The current branch") {
		branch, brErr := g.GetCurrentBranch(repoPath)
		if brErr != nil {
			return fmt.Errorf("git push (no upstream, branch unknown): %w", err)
		}
		if err2 := exec.Command("git", "-C", repoPath, "push", "--set-upstream", "origin", branch).Run(); err2 != nil {
			return fmt.Errorf("git push --set-upstream origin %s: %w", branch, err2)
		}
		return nil
	}
	return fmt.Errorf("git push: %w", err)
}
