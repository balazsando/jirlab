package actions

import (
	"fmt"

	"github.com/andob/jirlab/internal/integration"
)

const (
	StatusInProgress       = "In Progress"
	StatusTesting          = "Testing"
	StatusDone             = "Done"
	StatusToDo             = "To Do"
	TransitionIDTestEnv    = "541"
)

// TransitionIssue moves a Jira issue using a direct transition ID.
func TransitionIssue(jira integration.JiraService, issueKey, transitionID string) error {
	if err := jira.TransitionByID(issueKey, transitionID); err != nil {
		return fmt.Errorf("transition issue %s (id=%s): %w", issueKey, transitionID, err)
	}
	return nil
}

// PickupIssue moves an issue to "In Progress".
func PickupIssue(jira integration.JiraService, issueKey string) error {
	return jira.TransitionToStatus(issueKey, StatusInProgress)
}

// MoveToTestEnv moves an issue to "TEST ENV" (transition 541, falls back to name-based lookup Testing).
func MoveToTestEnv(jira integration.JiraService, issueKey string) error {
	if err := TransitionIssue(jira, issueKey, TransitionIDTestEnv); err == nil {
		return nil
	}
	return jira.TransitionToStatus(issueKey, StatusTesting)
}

// MarkDone moves an issue to "Done".
func MarkDone(jira integration.JiraService, issueKey string) error {
	return jira.TransitionToStatus(issueKey, StatusDone)
}
