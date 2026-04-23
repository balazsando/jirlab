package service

import "time"

// MockIssues returns a set of representative Jira issues covering all colour states.
func MockIssues() []Issue {
	now := time.Now()
	return []Issue{
		{
			Key: "PROJ-101", Summary: "Implement login page", Status: "In Progress",
			Assignee: "Ben Anderson", AssigneeKey: "banderson", Priority: "High",
			IssueType: "Development task", BranchName: "feature/PROJ-101-implement-login-page",
			SprintName: "Sprint 42", CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-102", Summary: "Fix password reset flow", Status: "To Do",
			Assignee: "Alice Smith", AssigneeKey: "asmith", Priority: "Medium",
			IssueType: "Development Sub-task", SprintName: "Sprint 42",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-103", Summary: "Add unit tests for auth module", Status: "To Do",
			Assignee: "", AssigneeKey: "", Priority: "Low",
			IssueType: "Task", SprintName: "Sprint 42",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-104", Summary: "Migrate users table to PostgreSQL", Status: "Prio 1",
			Assignee: "Ben Anderson", AssigneeKey: "banderson", Priority: "Critical",
			IssueType: "Development task", BranchName: "feature/PROJ-104-migrate-users",
			SprintName: "Sprint 42", CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-105", Summary: "Design new dashboard layout", Status: "To Dev",
			Assignee: "Carol Jones", AssigneeKey: "cjones", Priority: "Medium",
			IssueType: "Sub-task", SprintName: "Sprint 42",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-106", Summary: "API rate limiting middleware", Status: "Backlog",
			Assignee: "", AssigneeKey: "", Priority: "Low",
			IssueType: "Development Sub-task", SprintName: "Sprint 42",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-107", Summary: "Add dark mode support", Status: "To Do",
			Assignee: "Ben Anderson", AssigneeKey: "banderson", Priority: "Low",
			IssueType: "Task", SprintName: "Sprint 42",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			Key: "PROJ-108", Summary: "Fix mobile responsive issues", Status: "In Progress",
			Assignee: "Dave Lee", AssigneeKey: "dlee", Priority: "High",
			IssueType: "Development Sub-task", BranchName: "feature/PROJ-108-fix-mobile",
			SprintName: "Sprint 42", CreatedAt: now, UpdatedAt: now,
		},
	}
}

// MockMergeRequests returns representative MRs covering all colour states.
func MockMergeRequests() []MergeRequest {
	return []MergeRequest{
		{
			ID: 1, Repository: "backend-api", Title: "feat: implement login endpoint",
			Author: "banderson", Status: "opened", Approved: false, IsMine: true,
			IssueKey: "PROJ-101", WebURL: "https://gitlab.example.com/backend-api/-/merge_requests/1",
		},
		{
			ID: 2, Repository: "backend-api", Title: "feat: migrate users table",
			Author: "banderson", Status: "opened", Approved: true, IsMine: true,
			IssueKey: "PROJ-104", WebURL: "https://gitlab.example.com/backend-api/-/merge_requests/2",
		},
		{
			ID: 3, Repository: "frontend-app", Title: "fix: mobile responsive layout",
			Author: "dlee", Status: "opened", Approved: false, IsMine: false,
			IssueKey: "PROJ-108", WebURL: "https://gitlab.example.com/frontend-app/-/merge_requests/3",
		},
		{
			ID: 4, Repository: "frontend-app", Title: "chore: update dependencies",
			Author: "asmith", Status: "opened", Approved: true, IsMine: false,
			IssueKey: "", WebURL: "https://gitlab.example.com/frontend-app/-/merge_requests/4",
		},
	}
}

// MockTimeLogs returns sample time logs for today (partial day — tests color logic).
func MockTimeLogs() []TimeLog {
	today := time.Now().Truncate(24 * time.Hour)
	start := today.Add(8 * time.Hour)
	return []TimeLog{
		{
			IssueKey: "PROJ-101", Description: "Implement login page - backend",
			From: start, To: start.Add(2 * time.Hour), Hours: 2,
		},
		{
			IssueKey: "PROJ-104", Description: "Database migration script",
			From: start.Add(2 * time.Hour), To: start.Add(4 * time.Hour), Hours: 2,
		},
	}
}

// MockPipelines returns representative pipelines with job count data for tests.
func MockPipelines() []Pipeline {
	now := time.Now()
	return []Pipeline{
		{
			ID: 1001, ProjectID: 1, Status: "running", Ref: "feature/PROJ-101-login",
			WebURL:   "https://gitlab.example.com/backend-api/-/pipelines/1001",
			RepoName: "backend-api", JobsSuccess: 5, JobsTotal: 7,
			CreatedAt: now.Add(-10 * time.Minute),
		},
		{
			ID: 1002, ProjectID: 2, Status: "pending", Ref: "feature/PROJ-108-mobile",
			WebURL:   "https://gitlab.example.com/frontend-app/-/pipelines/1002",
			RepoName: "frontend-app", JobsSuccess: 0, JobsTotal: 0,
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID: 1003, ProjectID: 1, Status: "running", Ref: "feature/PROJ-104-migrate",
			WebURL:   "https://gitlab.example.com/backend-api/-/pipelines/1003",
			RepoName: "backend-api", JobsSuccess: 3, JobsTotal: 4,
			CreatedAt: now.Add(-2 * time.Minute),
		},
	}
}
