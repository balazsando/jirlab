package integration

import (
	"fmt"
	"sort"
	"time"

	"github.com/andob/jirlab/internal/service"
)

// JiraService provides higher-level Jira use-cases on top of JiraClient.
type JiraService interface {
	// GetAllSprintIssues returns all issues in the active sprint, sorted, unfiltered.
	GetAllSprintIssues(boardID string) ([]service.Issue, error)
	// GetIssueDetails fetches full details (description, fix versions, comments) for one issue.
	GetIssueDetails(key string) (*service.Issue, error)
	// GetTransitions returns valid workflow transitions for an issue.
	GetTransitions(issueKey string) ([]service.Transition, error)
	// TransitionToStatus moves an issue to the given status name (resolves transition ID).
	TransitionToStatus(issueKey, targetStatus string) error
	// TransitionByID moves an issue using a known transition ID directly (no lookup needed).
	TransitionByID(issueKey, transitionID string) error
	// AddComment posts a comment to an issue.
	AddComment(issueKey, body string) error
	// AssignToMe assigns the issue to the currently authenticated user (resolved via /3/myself).
	AssignToMe(issueKey string) error
	// GetMyAccountID returns the accountId of the authenticated Jira user.
	GetMyAccountID() (string, error)
	// Unassign removes the assignee from an issue.
	Unassign(issueKey string) error
	// LogWork logs time for a given issue between from and to.
	LogWork(issueKey string, from, to time.Time) error
	// GetWorklogsForDate returns time logs for the given accountID on the given date.
	GetWorklogsForDate(accountID string, date time.Time) ([]service.TimeLog, error)
	// GetBaseURL returns the Jira base URL configured for this service.
	GetBaseURL() string
}

type jiraService struct {
	client service.JiraClient
}

// NewJiraService creates a JiraService backed by the given JiraClient.
func NewJiraService(client service.JiraClient) JiraService {
	return &jiraService{client: client}
}

func (s *jiraService) GetIssueDetails(key string) (*service.Issue, error) {
	return s.client.GetIssue(key)
}

func (s *jiraService) GetTransitions(issueKey string) ([]service.Transition, error) {
	return s.client.GetTransitions(issueKey)
}

func (s *jiraService) TransitionByID(issueKey, transitionID string) error {
	return s.client.TransitionIssue(issueKey, transitionID)
}

func (s *jiraService) TransitionToStatus(issueKey, targetStatus string) error {
	transitions, err := s.client.GetTransitions(issueKey)
	if err != nil {
		return err
	}
	for _, t := range transitions {
		if t.To == targetStatus {
			return s.client.TransitionIssue(issueKey, t.ID)
		}
	}
	return fmt.Errorf("no transition to %q available for %s (may already be in that state)", targetStatus, issueKey)
}

func (s *jiraService) AddComment(issueKey, body string) error {
	return s.client.AddComment(issueKey, body)
}

func (s *jiraService) GetWorklogsForDate(accountID string, date time.Time) ([]service.TimeLog, error) {
	return s.client.GetWorklogsForDate(accountID, date)
}

func (s *jiraService) AssignToMe(issueKey string) error {
	accountID, err := s.client.GetCurrentUserAccountID()
	if err != nil {
		return fmt.Errorf("resolve my account: %w", err)
	}
	return s.client.AssignIssue(issueKey, accountID)
}

func (s *jiraService) GetMyAccountID() (string, error) {
	return s.client.GetCurrentUserAccountID()
}

func (s *jiraService) Unassign(issueKey string) error {
	return s.client.AssignIssue(issueKey, "")
}

func (s *jiraService) GetBaseURL() string {
	return s.client.GetBaseURL()
}

func (s *jiraService) LogWork(issueKey string, from, to time.Time) error {
	seconds := int(to.Sub(from).Seconds())
	if seconds <= 0 {
		return fmt.Errorf("invalid time range: from=%v to=%v", from, to)
	}
	return s.client.AddWorklog(issueKey, from, seconds)
}

// GetAllSprintIssues returns all issues in the active sprint, sorted by status order then key.
func (s *jiraService) GetAllSprintIssues(boardID string) ([]service.Issue, error) {
	issues, err := s.client.GetBoardIssues(boardID, service.Filter{})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(issues, func(i, j int) bool {
		oi := statusSortOrder(issues[i].Status)
		oj := statusSortOrder(issues[j].Status)
		if oi != oj {
			return oi < oj
		}
		return issues[i].Key < issues[j].Key
	})
	return issues, nil
}

// statusSortOrder assigns a numeric rank to a Jira status for sorting.
func statusSortOrder(status string) int {
	switch status {
	case "In Backlog", "Backlog", "Open", "On Hold":
		return 0
	case "Prio 1", "To Dev", "To Do":
		return 1
	case "In Progress", "Review":
		return 2
	case "TEST ENV", "PREPROD ENV", "Testing":
		return 3
	case "Done":
		return 4
	default:
		return 99
	}
}
