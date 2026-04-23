package service

import (
	"regexp"
	"strings"
	"time"
)

// IssueKeyRe matches a Jira issue key embedded in any string (e.g. branch name).
// Examples: PROJ-123, METAAPI-42
var IssueKeyRe = regexp.MustCompile(`(?i)([A-Z]+-\d+)`)

// --- Jira domain models ---

// Issue represents a Jira issue.
type Issue struct {
	Key               string
	Summary           string
	Status            string
	StatusCategoryKey string // new | indeterminate | done
	Assignee          string
	AssigneeKey       string // username / account id for comparison
	Priority          string
	IssueType         string
	ParentKey         string
	ParentSummary     string
	Description       string
	FixVersions       []string
	Comments          []IssueComment
	SprintName        string
	BranchName        string // local branch if exists
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// IssueComment is a single comment on a Jira issue.
type IssueComment struct {
	Author  string
	Body    string
	Created time.Time
}

// Transition represents a valid workflow transition for an issue.
type Transition struct {
	ID   string
	Name string
	To   string // target status name
}

// Sprint represents a Jira sprint.
type Sprint struct {
	ID    int
	Name  string
	State string // active, closed, future
}

// Filter holds optional filtering parameters when fetching board issues.
type Filter struct {
	Assignee   string
	SprintName string
	SprintID   int
}

// --- GitLab domain models ---

// Project represents a GitLab project.
type Project struct {
	ID                int
	Name              string
	NameWithNamespace string
	PathWithNamespace string
	DefaultBranch     string
	WebURL            string
}

// Repo represents a local git repository discovered on disk.
// GitLabProjectID is 0 when not yet resolved.
// DefaultBranch is populated after GitLab project resolution.

// Branch represents a GitLab branch.
type Branch struct {
	Name      string
	WebURL    string
	Protected bool
}

// Tag represents a GitLab repository tag.
type Tag struct {
	Name      string
	Message   string
	CreatedAt time.Time
}

// PipelineJob represents a single job within a GitLab CI/CD pipeline.
type PipelineJob struct {
	ID     int
	Name   string
	Status string // created, pending, running, failed, success, canceled, skipped, manual
	Stage  string
}

// PipelineVariable is a configurable CI variable defined in .gitlab-ci.yml.
// Only variables that have a non-empty description field are shown, mirroring
// the "Run Pipeline" dialog in the GitLab browser UI.
type PipelineVariable struct {
	Key         string
	Value       string // default value from the YAML definition
	Description string
}

// Pipeline represents a GitLab CI/CD pipeline.
type Pipeline struct {
	ID          int
	Status      string // running, pending, success, failed, canceled, skipped
	Ref         string // branch or tag name
	WebURL      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ProjectID   int
	RepoName    string
	JobsSuccess int // number of jobs in "success" state (0 = not yet fetched)
	JobsTotal   int // total number of jobs (0 = not yet fetched)
}

// MergeRequest represents a GitLab merge request.
type MergeRequest struct {
	ID               int
	IID              int // project-scoped MR number (used for API calls)
	ProjectID        int // GitLab project ID (used for API calls)
	Repository       string
	Title            string
	Author           string
	Status           string // opened, merged, closed
	WebURL           string
	Approved         bool
	IsMine           bool
	HasRecentComment bool      // true if a non-system note was posted in the last 24 h
	IssueKey         string    // linked Jira issue key if any
	SourceBranch     string    // git source branch of the MR
	UpdatedAt        time.Time // last update time from GitLab
}

// --- Repository model ---

// GitStats holds uncommitted change counts mirroring the oh-my-zsh git plugin status.
type GitStats struct {
	Staged    int // files with index changes (A/M/D/R/C in column 1)
	Unstaged  int // files with worktree changes (M/D/T in column 2)
	Untracked int // untracked files (??)
}

// IsClean reports whether the working tree has no uncommitted changes.
func (g GitStats) IsClean() bool {
	return g.Staged == 0 && g.Unstaged == 0 && g.Untracked == 0
}

// Repo represents a local git repository discovered on disk.
type Repo struct {
	Name               string
	Path               string
	CurrentBranch      string
	RemoteURL          string    // git remote origin URL
	GitLabProjectID    int       // resolved GitLab project ID (0 = not resolved)
	DefaultBranch      string    // e.g. "develop" or "main", from GitLab project metadata
	WebURL             string    // GitLab project web URL
	GitStats           GitStats  // uncommitted changes (populated by ScanRepos)
	Version            string    // Maven revision from .mvn/maven.config (-Drevision=...)
	LatestTagVersion   string    // newest GitLab tag name; empty if not yet fetched
	LatestTagDate      time.Time // creation date of the newest tag
	CurrentVersionDate time.Time // creation date of the tag matching Version
}

// --- Time Tracker model ---

// TimeLog represents a worklog entry for the time tracker section.
type TimeLog struct {
	IssueKey    string
	Description string // issue summary
	Comment     string // worklog comment text
	From        time.Time
	To          time.Time
	Hours       float64
}

// --- Helpers ---

// AssigneeInitials returns up-to-2-character initials from a display name or email.
// "John Doe" → "JD", "Alice" → "A", "john.doe" → "JD", "" → "".
func AssigneeInitials(name string) string {
	if name == "" {
		return ""
	}
	// email-like / dot-separated case
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		if len(parts) == 1 {
			return strings.ToUpper(string([]rune(parts[0])[:1]))
		}
		first := []rune(parts[0])
		second := []rune(parts[1])
		return strings.ToUpper(string(first[:1]) + string(second[:1]))
	}
	// Regular display name: first and last word initials.
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return strings.ToUpper(string([]rune(parts[0])[:1]))
	default:
		first := []rune(parts[0])
		last := []rune(parts[len(parts)-1])
		return strings.ToUpper(string(first[:1]) + string(last[:1]))
	}
}
