package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// repoDefaultBranch returns the default branch for a repo.
// Uses DefaultBranch if set (from GitLab metadata), otherwise falls back to "develop".
func repoDefaultBranch(repo service.Repo) string {
	if repo.DefaultBranch != "" {
		return repo.DefaultBranch
	}
	return "develop"
}

// navigateCmd writes the path for the jirlab shell wrapper and quits the TUI.
func navigateCmd(git integration.GitCmdService, repoPath string) tea.Cmd {
	return func() tea.Msg {
		_ = git.WriteNavPath(repoPath)
		return tea.QuitMsg{}
	}
}

// checkoutWithFallback checks out branch; falls back to "main" if the named branch
// does not exist. Returns the branch that was actually checked out.
func checkoutWithFallback(git integration.GitCmdService, repoPath, branch string) (string, error) {
	if err := git.Checkout(repoPath, branch); err != nil {
		if branch != "main" {
			if err2 := git.Checkout(repoPath, "main"); err2 == nil {
				return "main", nil
			}
		}
		return "", fmt.Errorf("checkout %s failed: %w", branch, err)
	}
	return branch, nil
}

// checkoutDevelopCmd stashes any changes, checks out the repo's default branch,
// pulls latest, then re-applies the stash (drops on conflict).
func checkoutDevelopCmd(git integration.GitCmdService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		repoPath := repo.Path
		baseBranch := repoDefaultBranch(repo)

		stashed, _ := git.Stash(repoPath)

		checkedOut, err := checkoutWithFallback(git, repoPath, baseBranch)
		if err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: err.Error()}
		}

		_ = git.Pull(repoPath)

		if stashed {
			if popErr := git.StashPop(repoPath); popErr != nil {
				_ = git.StashDrop(repoPath)
				return repoActionDoneMsg{
					message: fmt.Sprintf("On %s (stash conflict – changes dropped)", checkedOut),
					rescan:  true,
				}
			}
		}

		return repoActionDoneMsg{
			message: fmt.Sprintf("Checked out %s in %s", checkedOut, repoPath),
			rescan:  true,
		}
	}
}

// checkoutNewBranchCmd stashes changes, updates the default branch, then creates
// and checks out a new feature branch. Stash is re-applied afterwards.
// On success it saves the ticket description to .jirlab/ticket/<key>.md and copies
// the file path to the clipboard (best-effort; errors are silently ignored).
func checkoutNewBranchCmd(git integration.GitCmdService, repo service.Repo, branchName string, issue service.Issue, fs integration.FilesystemService, shell integration.ShellService) tea.Cmd {
	return func() tea.Msg {
		repoPath := repo.Path
		baseBranch := repoDefaultBranch(repo)

		stashed, _ := git.Stash(repoPath)

		checkedOut, err := checkoutWithFallback(git, repoPath, baseBranch)
		if err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return boardActionDoneMsg{message: fmt.Sprintf("Failed to checkout base: %v", err)}
		}

		_ = git.Pull(repoPath)

		if err := git.CreateAndCheckoutBranch(repoPath, branchName); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return boardActionDoneMsg{message: fmt.Sprintf("Failed to create branch: %v", err)}
		}

		if stashed {
			_ = git.StashPop(repoPath)
		}

		if fs != nil && shell != nil {
			if filePath, err := SaveTicketDescription(fs, repoPath, issue); err == nil {
				_ = shell.CopyToClipboard(filePath)
			}
		}

		return boardActionDoneMsg{
			message: fmt.Sprintf("Created branch %s from %s in %s", branchName, checkedOut, repoPath),
		}
	}
}

// checkoutMRBranchCmd finds the local repo matching the MR's project ID and
// checks out the MR's source branch with safe stash/pull handling.
func checkoutMRBranchCmd(git integration.GitCmdService, repos []service.Repo, mr service.MergeRequest) tea.Cmd {
	return func() tea.Msg {
		if mr.SourceBranch == "" {
			return repoActionDoneMsg{message: "MR has no source branch info"}
		}
		var repo *service.Repo
		for i := range repos {
			if repos[i].GitLabProjectID == mr.ProjectID {
				repo = &repos[i]
				break
			}
		}
		if repo == nil {
			return repoActionDoneMsg{message: fmt.Sprintf("No local repo found for project %d", mr.ProjectID)}
		}

		repoPath := repo.Path
		stashed, _ := git.Stash(repoPath)

		_ = git.Fetch(repoPath, "origin", mr.SourceBranch+":"+mr.SourceBranch)

		if err := git.Checkout(repoPath, mr.SourceBranch); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("checkout %s failed: %v", mr.SourceBranch, err)}
		}

		_ = git.Pull(repoPath)

		if stashed {
			if popErr := git.StashPop(repoPath); popErr != nil {
				_ = git.StashDrop(repoPath)
				return repoActionDoneMsg{message: fmt.Sprintf("On %s (stash conflict – changes dropped)", mr.SourceBranch), rescan: true}
			}
		}

		return repoActionDoneMsg{
			message: fmt.Sprintf("Checked out %s in %s", mr.SourceBranch, repo.Name),
			rescan:  true,
		}
	}
}

// createLocalBranchCmd stashes changes, updates the default branch, then creates
// and checks out a new local feature branch. Stash is re-applied afterwards.
func createLocalBranchCmd(git integration.GitCmdService, repo service.Repo, branchName string) tea.Cmd {
	return func() tea.Msg {
		repoPath := repo.Path
		baseBranch := repoDefaultBranch(repo)

		stashed, _ := git.Stash(repoPath)

		checkedOut, err := checkoutWithFallback(git, repoPath, baseBranch)
		if err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("Failed to checkout base: %v", err)}
		}

		_ = git.Pull(repoPath)

		if err := git.CreateAndCheckoutBranch(repoPath, branchName); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("Failed to create branch %s: %v", branchName, err)}
		}

		if stashed {
			_ = git.StashPop(repoPath)
		}

		return repoActionDoneMsg{
			message: fmt.Sprintf("Created branch %s from %s", branchName, checkedOut),
			rescan:  true,
		}
	}
}

// checkoutRepoBranchCmd checks out an existing branch in a repo with safe stash handling.
func checkoutRepoBranchCmd(git integration.GitCmdService, repo service.Repo, branchName string) tea.Cmd {
	return func() tea.Msg {
		repoPath := repo.Path
		stashed, _ := git.Stash(repoPath)

		if err := git.Checkout(repoPath, branchName); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("checkout %s failed: %v", branchName, err)}
		}
		_ = git.Pull(repoPath)

		if stashed {
			if popErr := git.StashPop(repoPath); popErr != nil {
				_ = git.StashDrop(repoPath)
				return repoActionDoneMsg{message: fmt.Sprintf("On %s (stash conflict – changes dropped)", branchName), rescan: true}
			}
		}
		return repoActionDoneMsg{message: fmt.Sprintf("Checked out %s in %s", branchName, repo.Name), rescan: true}
	}
}

// checkoutRepoTagCmd checks out a tag as detached HEAD with safe stash handling.
func checkoutRepoTagCmd(git integration.GitCmdService, repo service.Repo, tagName string) tea.Cmd {
	return func() tea.Msg {
		repoPath := repo.Path
		stashed, _ := git.Stash(repoPath)

		if err := git.Checkout(repoPath, tagName); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("checkout tag %s failed: %v", tagName, err)}
		}

		if stashed {
			if popErr := git.StashPop(repoPath); popErr != nil {
				_ = git.StashDrop(repoPath)
				return repoActionDoneMsg{message: fmt.Sprintf("On tag %s (stash conflict – changes dropped)", tagName), rescan: true}
			}
		}
		return repoActionDoneMsg{message: fmt.Sprintf("Checked out tag %s in %s (detached HEAD)", tagName, repo.Name), rescan: true}
	}
}

// checkoutRepoMRBranchCmd checks out an MR source branch in a specific repo.
func checkoutRepoMRBranchCmd(git integration.GitCmdService, repo service.Repo, mr service.MergeRequest) tea.Cmd {
	return func() tea.Msg {
		if mr.SourceBranch == "" {
			return repoActionDoneMsg{message: "MR has no source branch info"}
		}
		repoPath := repo.Path
		stashed, _ := git.Stash(repoPath)

		_ = git.Fetch(repoPath, "origin", mr.SourceBranch+":"+mr.SourceBranch)

		if err := git.Checkout(repoPath, mr.SourceBranch); err != nil {
			if stashed {
				_ = git.StashPop(repoPath)
			}
			return repoActionDoneMsg{message: fmt.Sprintf("checkout %s failed: %v", mr.SourceBranch, err)}
		}
		_ = git.Pull(repoPath)

		if stashed {
			if popErr := git.StashPop(repoPath); popErr != nil {
				_ = git.StashDrop(repoPath)
				return repoActionDoneMsg{message: fmt.Sprintf("On %s (stash conflict – changes dropped)", mr.SourceBranch), rescan: true}
			}
		}
		return repoActionDoneMsg{message: fmt.Sprintf("Checked out %s in %s", mr.SourceBranch, repo.Name), rescan: true}
	}
}

// stageAllCommitPushCmd opens a commit-message modal. On confirm, it stages all
// changes, commits with the provided message, and pushes to origin (setting
// --set-upstream automatically when the branch has no tracking ref).
func stageAllCommitPushCmd(git integration.GitCmdService, repo service.Repo) tea.Cmd {
	r := repo
	return func() tea.Msg {
		return OpenModalMsg{M: NewInputModal(
			fmt.Sprintf("Commit & push — %s @ %s", r.Name, r.CurrentBranch),
			"feat-describe your change",
			func(message string) tea.Cmd {
				return func() tea.Msg {
					if err := git.StageAllCommitPush(r.Path, message); err != nil {
						return errMsg{source: "repos", err: err}
					}
					return repoActionDoneMsg{message: fmt.Sprintf("Pushed: %s", message), rescan: true}
				}
			},
		)}
	}
}
