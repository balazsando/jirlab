package git

import (
	"fmt"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// CreateBranchFromIssue creates a remote GitLab branch linked to the given Jira issue.
// It uses the project's default branch as the base unless baseBranch is explicitly provided.
func CreateBranchFromIssue(
	gitlab integration.GitLabService,
	issue service.Issue,
	projectID int,
	baseBranch string,
) (*service.Branch, error) {
	if baseBranch == "" {
		project, err := gitlab.GetProject(projectID)
		if err != nil {
			return nil, fmt.Errorf("fetch project %d: %w", projectID, err)
		}
		baseBranch = project.DefaultBranch
		if baseBranch == "" {
			baseBranch = "develop"
		}
	}

	branch, err := gitlab.CreateBranchFromIssue(issue, projectID, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("create branch for %s: %w", issue.Key, err)
	}

	return branch, nil
}
