package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	gitpkg "github.com/andob/jirlab/internal/git"
	"github.com/andob/jirlab/internal/service"
)

var (
	gitProjectID  int
	gitBaseBranch string
)

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git-related commands (non-TUI)",
}

var gitBranchCmd = &cobra.Command{
	Use:   "branch <issue-key> <summary>",
	Short: "Create a GitLab branch for a Jira issue",
	Long: `Creates a GitLab branch named after the Jira issue.
The branch name is formatted as: feature/<KEY>-<slugified-summary>

Example:
  jirlab git branch PROJ-123 "Implement login" --project 42`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		summary := args[1]

		if gitProjectID == 0 {
			return fmt.Errorf("--project is required")
		}

		issue := service.Issue{
			Key:     issueKey,
			Summary: summary,
		}

		branch, err := gitpkg.CreateBranchFromIssue(gitlabSvc, issue, gitProjectID, gitBaseBranch)
		if err != nil {
			return err
		}

		fmt.Printf("Branch created: %s\n", branch.Name)
		fmt.Printf("URL: %s\n", branch.WebURL)
		return nil
	},
}

var gitListProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List accessible GitLab projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		projects, err := gitlabSvc.ListProjects()
		if err != nil {
			return err
		}

		for _, p := range projects {
			fmt.Printf("%s\t%s\n", strconv.Itoa(p.ID), p.NameWithNamespace)
		}
		return nil
	},
}

func init() {
	gitBranchCmd.Flags().IntVar(&gitProjectID, "project", 0, "GitLab project ID (required)")
	gitBranchCmd.Flags().StringVar(&gitBaseBranch, "base", "", "Base branch (defaults to project default branch)")

	gitCmd.AddCommand(gitBranchCmd)
	gitCmd.AddCommand(gitListProjectsCmd)
}
