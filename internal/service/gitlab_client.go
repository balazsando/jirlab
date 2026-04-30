package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitLabClient defines operations against the GitLab REST API.
type GitLabClient interface {
	CreateBranch(projectID int, branchName, baseBranch string) (*Branch, error)
	ListProjects() ([]Project, error)
	GetProject(id int) (*Project, error)
	GetProjectByPath(pathWithNamespace string) (*Project, error)
	ListMergeRequests(state string) ([]MergeRequest, error)
	ListProjectMergeRequests(projectID int, state string) ([]MergeRequest, error)
	MergeMR(projectID, mrIID int) error
	CloseMR(projectID, mrIID int) error
	GetCurrentUsername() (string, error)
	// DownloadMRPatch fetches all file diffs for a merge request and formats them
	// as a unified diff patch file using the GitLab API /diffs endpoint.
	DownloadMRPatch(projectID, mrIID int) ([]byte, error)
	// ListProjectBranches returns all branches for the given GitLab project.
	ListProjectBranches(projectID int) ([]Branch, error)
	// ListProjectTags returns all tags for the given GitLab project, newest first.
	ListProjectTags(projectID int) ([]Tag, error)
	// ListProjectPipelines returns pipelines for the given project filtered by status.
	ListProjectPipelines(projectID int, statuses []string) ([]Pipeline, error)
	// GetMRApprovals returns whether the given MR has been approved.
	GetMRApprovals(projectID, mrIID int) (approved bool, err error)
	// GetMRRecentComment returns true if a non-system MR note was created in the last 24 hours.
	GetMRRecentComment(projectID, mrIID int) (hasRecent bool, err error)
	// CreateMR opens a new merge request on the given project.
	CreateMR(projectID int, sourceBranch, targetBranch, title string) (*MergeRequest, error)
	// ListPipelineJobs returns all jobs for the given pipeline.
	ListPipelineJobs(projectID, pipelineID int) ([]PipelineJob, error)
	// GetCILint calls the GitLab CI lint endpoint and returns the merged YAML bytes,
	// resolving all !include directives — matching what the browser Run Pipeline dialog uses.
	GetCILint(projectID int, ref string) ([]byte, error)
	// TriggerPipelineRun creates a new pipeline on the given project/ref with optional variables.
	TriggerPipelineRun(projectID int, ref string, variables map[string]string) (*Pipeline, error)
}

type gitLabClient struct {
	apiURL     string
	token      string
	httpClient *http.Client
}

// NewGitLabClient constructs a GitLabClient authenticated with a personal access token.
func NewGitLabClient(apiURL, token string) GitLabClient {
	return &gitLabClient{
		apiURL: strings.TrimRight(apiURL, "/"),
		token:  token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// --- HTTP helpers ---

func (c *gitLabClient) newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *gitLabClient) do(req *http.Request, dest interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab API error %d: %s", resp.StatusCode, string(raw))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("gitlab response decode error: %w", err)
		}
	}
	return nil
}

// --- CreateBranch ---

func (c *gitLabClient) CreateBranch(projectID int, branchName, baseBranch string) (*Branch, error) {
	url := fmt.Sprintf("%s/projects/%d/repository/branches", c.apiURL, projectID)
	payload, _ := json.Marshal(map[string]string{
		"branch": branchName,
		"ref":    baseBranch,
	})

	req, err := c.newRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}

	var raw struct {
		Name      string `json:"name"`
		WebURL    string `json:"web_url"`
		Protected bool   `json:"protected"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}

	return &Branch{
		Name:      raw.Name,
		WebURL:    raw.WebURL,
		Protected: raw.Protected,
	}, nil
}

// --- ListProjects ---

func (c *gitLabClient) ListProjects() ([]Project, error) {
	var result []Project

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/projects?membership=true&per_page=100&page=%d", c.apiURL, page)
		req, err := c.newRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("gitlab request failed: %w", err)
		}
		nextPage := resp.Header.Get("X-Next-Page")

		var pageItems []struct {
			ID                int    `json:"id"`
			Name              string `json:"name"`
			NameWithNamespace string `json:"name_with_namespace"`
			PathWithNamespace string `json:"path_with_namespace"`
			DefaultBranch     string `json:"default_branch"`
			WebURL            string `json:"web_url"`
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("gitlab API error %d: %s", resp.StatusCode, string(raw))
		}
		if err := json.NewDecoder(resp.Body).Decode(&pageItems); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("gitlab response decode error: %w", err)
		}
		resp.Body.Close()

		for _, r := range pageItems {
			result = append(result, Project{
				ID:                r.ID,
				Name:              r.Name,
				NameWithNamespace: r.NameWithNamespace,
				PathWithNamespace: r.PathWithNamespace,
				DefaultBranch:     r.DefaultBranch,
				WebURL:            r.WebURL,
			})
		}

		if nextPage == "" || nextPage == "0" {
			break
		}
	}

	return result, nil
}

// --- GetProject ---

func (c *gitLabClient) GetProject(id int) (*Project, error) {
	url := fmt.Sprintf("%s/projects/%d", c.apiURL, id)
	req, err := c.newRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		NameWithNamespace string `json:"name_with_namespace"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}

	return &Project{
		ID:                raw.ID,
		Name:              raw.Name,
		NameWithNamespace: raw.NameWithNamespace,
		PathWithNamespace: raw.PathWithNamespace,
		DefaultBranch:     raw.DefaultBranch,
		WebURL:            raw.WebURL,
	}, nil
}

// --- ListMergeRequests ---

// ListMergeRequests returns open MRs created by the current token holder AND
// MRs assigned to them for review. Results are deduplicated by ID; own MRs
// (created_by_me) take precedence so IsMine is set correctly.
// state should be "opened", "merged", or "closed".
func (c *gitLabClient) ListMergeRequests(state string) ([]MergeRequest, error) {
	mine, err := c.fetchMRsByScope(state, "created_by_me")
	if err != nil {
		return nil, err
	}
	assigned, _ := c.fetchMRsByScope(state, "assigned_to_me")
	// Merge, preferring mine (IsMine=true) on duplicate IDs.
	seen := make(map[int]bool, len(mine))
	for _, mr := range mine {
		seen[mr.ID] = true
	}
	result := make([]MergeRequest, len(mine))
	copy(result, mine)
	for _, mr := range assigned {
		if !seen[mr.ID] {
			seen[mr.ID] = true
			result = append(result, mr)
		}
	}
	return result, nil
}

// fetchMRsByScope is the shared implementation for a single-scope MR list call.
func (c *gitLabClient) fetchMRsByScope(state, scope string) ([]MergeRequest, error) {
	apiURL := fmt.Sprintf("%s/merge_requests?state=%s&scope=%s&per_page=100", c.apiURL, state, scope)
	req, err := c.newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	var raw []struct {
		ID        int    `json:"id"`
		IID       int    `json:"iid"`
		ProjectID int    `json:"project_id"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Author    struct {
			Name string `json:"name"`
		} `json:"author"`
		WebURL       string `json:"web_url"`
		SourceBranch string `json:"source_branch"`
		References   struct {
			Full string `json:"full"`
		} `json:"references"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}

	isMine := scope == "created_by_me"
	result := make([]MergeRequest, 0, len(raw))
	for _, r := range raw {
		// Extract repo from "group/project!42" → "group/project"
		repo := r.References.Full
		if idx := strings.LastIndex(repo, "!"); idx >= 0 {
			repo = repo[:idx]
		}
		// Extract Jira issue key from source branch name
		issueKey := ""
		if m := IssueKeyRe.FindStringSubmatch(r.SourceBranch); len(m) >= 2 {
			issueKey = strings.ToUpper(m[1])
		}
		updatedAt, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		result = append(result, MergeRequest{
			ID:           r.ID,
			IID:          r.IID,
			ProjectID:    r.ProjectID,
			Title:        r.Title,
			Author:       r.Author.Name,
			Status:       r.State,
			WebURL:       r.WebURL,
			Repository:   repo,
			IsMine:       isMine,
			IssueKey:     issueKey,
			SourceBranch: r.SourceBranch,
			UpdatedAt:    updatedAt,
		})
	}
	return result, nil
}

// MergeMR accepts (merges) a merge request.
func (c *gitLabClient) MergeMR(projectID, mrIID int) error {
	url := fmt.Sprintf("%s/projects/%d/merge_requests/%d/merge", c.apiURL, projectID, mrIID)
	req, err := c.newRequest(http.MethodPut, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// CloseMR closes (declines) a merge request.
func (c *gitLabClient) CloseMR(projectID, mrIID int) error {
	payload, _ := json.Marshal(map[string]string{"state_event": "close"})
	url := fmt.Sprintf("%s/projects/%d/merge_requests/%d", c.apiURL, projectID, mrIID)
	req, err := c.newRequest(http.MethodPut, url, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// GetProjectByPath looks up a GitLab project by its path with namespace,
// e.g. "mygroup/myrepo". Slashes are percent-encoded as required by the API.
func (c *gitLabClient) GetProjectByPath(pathWithNamespace string) (*Project, error) {
	encoded := strings.ReplaceAll(pathWithNamespace, "/", "%2F")
	url := fmt.Sprintf("%s/projects/%s", c.apiURL, encoded)
	req, err := c.newRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		NameWithNamespace string `json:"name_with_namespace"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}

	return &Project{
		ID:                raw.ID,
		Name:              raw.Name,
		NameWithNamespace: raw.NameWithNamespace,
		PathWithNamespace: raw.PathWithNamespace,
		DefaultBranch:     raw.DefaultBranch,
		WebURL:            raw.WebURL,
	}, nil
}

// GetCurrentUsername returns the username of the authenticated GitLab user.
func (c *gitLabClient) GetCurrentUsername() (string, error) {
	u := fmt.Sprintf("%s/user", c.apiURL)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	var raw struct {
		Username string `json:"username"`
	}
	if err := c.do(req, &raw); err != nil {
		return "", err
	}
	return raw.Username, nil
}

// ListProjectMergeRequests fetches all open MRs for a specific GitLab project.
// IsMine is set by comparing the MR author's username to the authenticated user.
func (c *gitLabClient) ListProjectMergeRequests(projectID int, state string) ([]MergeRequest, error) {
	u := fmt.Sprintf("%s/projects/%d/merge_requests?state=%s&per_page=100", c.apiURL, projectID, state)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	var raw []struct {
		ID        int    `json:"id"`
		IID       int    `json:"iid"`
		ProjectID int    `json:"project_id"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Author    struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"author"`
		WebURL       string `json:"web_url"`
		SourceBranch string `json:"source_branch"`
		References   struct {
			Full string `json:"full"`
		} `json:"references"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}

	currentUser, _ := c.GetCurrentUsername()

	result := make([]MergeRequest, 0, len(raw))
	for _, r := range raw {
		repo := r.References.Full
		if idx := strings.LastIndex(repo, "!"); idx >= 0 {
			repo = repo[:idx]
		}
		issueKey := ""
		if m := IssueKeyRe.FindStringSubmatch(r.SourceBranch); len(m) >= 2 {
			issueKey = strings.ToUpper(m[1])
		}
		updatedAt, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		result = append(result, MergeRequest{
			ID:           r.ID,
			IID:          r.IID,
			ProjectID:    r.ProjectID,
			Title:        r.Title,
			Author:       r.Author.Name,
			Status:       r.State,
			WebURL:       r.WebURL,
			Repository:   repo,
			IsMine:       currentUser != "" && r.Author.Username == currentUser,
			IssueKey:     issueKey,
			SourceBranch: r.SourceBranch,
			UpdatedAt:    updatedAt,
		})
	}
	return result, nil
}

// mrDiffEntry represents one file's diff from the GitLab MR diffs API.
type mrDiffEntry struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// DownloadMRPatch fetches all file diffs for a merge request via the GitLab
// API /projects/:id/merge_requests/:iid/diffs endpoint (requires GitLab 15.7+)
// and formats them as a standard unified diff suitable for git-apply.
func (c *gitLabClient) DownloadMRPatch(projectID, mrIID int) ([]byte, error) {
	var allDiffs []mrDiffEntry
	for page := 1; page <= 10; page++ {
		u := fmt.Sprintf("%s/projects/%d/merge_requests/%d/diffs?per_page=100&page=%d",
			c.apiURL, projectID, mrIID, page)
		req, err := c.newRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download MR diffs: %w", err)
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("MR diffs: authentication required — check your GitLab token")
		case http.StatusForbidden:
			return nil, fmt.Errorf("MR diffs: access denied — token lacks read_api scope")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("MR diffs HTTP %d for project %d MR !%d", resp.StatusCode, projectID, mrIID)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read MR diffs: %w", err)
		}
		var page_diffs []mrDiffEntry
		if err := json.Unmarshal(body, &page_diffs); err != nil {
			return nil, fmt.Errorf("parse MR diffs: %w", err)
		}
		allDiffs = append(allDiffs, page_diffs...)
		if len(page_diffs) < 100 {
			break // last page
		}
	}
	return formatUnifiedDiff(allDiffs), nil
}

// formatUnifiedDiff converts GitLab mrDiffEntry records into a git-apply
// compatible unified diff.
func formatUnifiedDiff(diffs []mrDiffEntry) []byte {
	var sb strings.Builder
	for _, d := range diffs {
		switch {
		case d.NewFile:
			sb.WriteString("diff --git a/" + d.NewPath + " b/" + d.NewPath + "\n")
			sb.WriteString("new file mode 100644\n")
			sb.WriteString("--- /dev/null\n")
			sb.WriteString("+++ b/" + d.NewPath + "\n")
		case d.DeletedFile:
			sb.WriteString("diff --git a/" + d.OldPath + " b/" + d.OldPath + "\n")
			sb.WriteString("deleted file mode 100644\n")
			sb.WriteString("--- a/" + d.OldPath + "\n")
			sb.WriteString("+++ /dev/null\n")
		case d.RenamedFile:
			sb.WriteString("diff --git a/" + d.OldPath + " b/" + d.NewPath + "\n")
			sb.WriteString("rename from " + d.OldPath + "\n")
			sb.WriteString("rename to " + d.NewPath + "\n")
			sb.WriteString("--- a/" + d.OldPath + "\n")
			sb.WriteString("+++ b/" + d.NewPath + "\n")
		default:
			sb.WriteString("diff --git a/" + d.OldPath + " b/" + d.NewPath + "\n")
			sb.WriteString("--- a/" + d.OldPath + "\n")
			sb.WriteString("+++ b/" + d.NewPath + "\n")
		}
		sb.WriteString(d.Diff)
		if len(d.Diff) > 0 && d.Diff[len(d.Diff)-1] != '\n' {
			sb.WriteByte('\n')
		}
	}
	return []byte(sb.String())
}

// --- ListProjectBranches ---

func (c *gitLabClient) ListProjectBranches(projectID int) ([]Branch, error) {
	u := fmt.Sprintf("%s/projects/%d/repository/branches?per_page=100", c.apiURL, projectID)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name      string `json:"name"`
		WebURL    string `json:"web_url"`
		Protected bool   `json:"protected"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	result := make([]Branch, len(raw))
	for i, r := range raw {
		result[i] = Branch{Name: r.Name, WebURL: r.WebURL, Protected: r.Protected}
	}
	return result, nil
}

// --- ListProjectTags ---

func (c *gitLabClient) ListProjectTags(projectID int) ([]Tag, error) {
	u := fmt.Sprintf("%s/projects/%d/repository/tags?per_page=100&order_by=version&sort=desc", c.apiURL, projectID)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Commit  struct {
			CreatedAt string `json:"created_at"`
		} `json:"commit"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	result := make([]Tag, 0, len(raw))
	for _, r := range raw {
		t, _ := time.Parse(time.RFC3339, r.Commit.CreatedAt)
		result = append(result, Tag{Name: r.Name, Message: r.Message, CreatedAt: t})
	}
	return result, nil
}

// --- ListProjectPipelines ---

func (c *gitLabClient) ListProjectPipelines(projectID int, statuses []string) ([]Pipeline, error) {
	type rawPipeline struct {
		ID        int    `json:"id"`
		Status    string `json:"status"`
		Ref       string `json:"ref"`
		WebURL    string `json:"web_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		ProjectID int    `json:"project_id"`
	}

	// GitLab only accepts a single status per request, so issue one request per status.
	seen := make(map[int]bool)
	var result []Pipeline
	for _, status := range statuses {
		url := fmt.Sprintf("%s/projects/%d/pipelines?per_page=100&status=%s", c.apiURL, projectID, status)
		req, err := c.newRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		var raw []rawPipeline
		if err := c.do(req, &raw); err != nil {
			return nil, err
		}
		for _, r := range raw {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			createdAt, _ := time.Parse(time.RFC3339, r.CreatedAt)
			updatedAt, _ := time.Parse(time.RFC3339, r.UpdatedAt)
			pid := r.ProjectID
			if pid == 0 {
				pid = projectID
			}
			result = append(result, Pipeline{
				ID:        r.ID,
				Status:    r.Status,
				Ref:       r.Ref,
				WebURL:    r.WebURL,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				ProjectID: pid,
			})
		}
	}
	return result, nil
}

// --- GetMRApprovals ---

// GetMRApprovals calls the GitLab MR approvals endpoint and returns whether
// the merge request has been approved.
// Endpoint: GET /projects/{id}/merge_requests/{merge_request_iid}/approvals
func (c *gitLabClient) GetMRApprovals(projectID, mrIID int) (bool, error) {
	u := fmt.Sprintf("%s/projects/%d/merge_requests/%d/approvals", c.apiURL, projectID, mrIID)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	var raw struct {
		Approved bool `json:"approved"`
	}
	if err := c.do(req, &raw); err != nil {
		return false, err
	}
	return raw.Approved, nil
}

// --- GetMRRecentComment ---

// GetMRRecentComment fetches non-system notes for the MR and returns true if
// any was created within the last 24 hours.
// Endpoint: GET /projects/{id}/merge_requests/{merge_request_iid}/notes
func (c *gitLabClient) GetMRRecentComment(projectID, mrIID int) (bool, error) {
	u := fmt.Sprintf("%s/projects/%d/merge_requests/%d/notes?per_page=100", c.apiURL, projectID, mrIID)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	var raw []struct {
		System    bool   `json:"system"`
		CreatedAt string `json:"created_at"`
	}
	if err := c.do(req, &raw); err != nil {
		return false, err
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, n := range raw {
		if n.System {
			continue
		}
		t, err := time.Parse(time.RFC3339, n.CreatedAt)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			return true, nil
		}
	}
	return false, nil
}

// --- CreateMR ---

func (c *gitLabClient) CreateMR(projectID int, sourceBranch, targetBranch, title string) (*MergeRequest, error) {
	u := fmt.Sprintf("%s/projects/%d/merge_requests", c.apiURL, projectID)
	payload, _ := json.Marshal(map[string]string{
		"source_branch": sourceBranch,
		"target_branch": targetBranch,
		"title":         title,
	})
	req, err := c.newRequest(http.MethodPost, u, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID        int    `json:"id"`
		IID       int    `json:"iid"`
		ProjectID int    `json:"project_id"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Author    struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"author"`
		WebURL       string `json:"web_url"`
		SourceBranch string `json:"source_branch"`
		References   struct {
			Full string `json:"full"`
		} `json:"references"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	repo := raw.References.Full
	if idx := strings.LastIndex(repo, "!"); idx >= 0 {
		repo = repo[:idx]
	}
	issueKey := ""
	if m := IssueKeyRe.FindStringSubmatch(raw.SourceBranch); len(m) >= 2 {
		issueKey = strings.ToUpper(m[1])
	}
	updatedAt, _ := time.Parse(time.RFC3339, raw.UpdatedAt)
	return &MergeRequest{
		ID:           raw.ID,
		IID:          raw.IID,
		ProjectID:    raw.ProjectID,
		Title:        raw.Title,
		Author:       raw.Author.Name,
		Status:       raw.State,
		WebURL:       raw.WebURL,
		Repository:   repo,
		IsMine:       true, // we just created it
		IssueKey:     issueKey,
		SourceBranch: raw.SourceBranch,
		UpdatedAt:    updatedAt,
	}, nil
}

// --- ListPipelineJobs ---

func (c *gitLabClient) ListPipelineJobs(projectID, pipelineID int) ([]PipelineJob, error) {
	u := fmt.Sprintf("%s/projects/%d/pipelines/%d/jobs?per_page=100", c.apiURL, projectID, pipelineID)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Stage  string `json:"stage"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	result := make([]PipelineJob, len(raw))
	for i, r := range raw {
		result[i] = PipelineJob{ID: r.ID, Name: r.Name, Status: r.Status, Stage: r.Stage}
	}
	return result, nil
}

// --- GetCILint ---

// GetCILint calls GET /projects/:id/ci/lint?ref=:ref
// and returns the merged YAML after GitLab resolves all !include directives.
// This is the same data source the browser "Run Pipeline" dialog uses to list variables.
func (c *gitLabClient) GetCILint(projectID int, ref string) ([]byte, error) {
	u := fmt.Sprintf("%s/projects/%d/ci/lint?ref=%s", c.apiURL, projectID, url.QueryEscape(ref))
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		MergedYAML string `json:"merged_yaml"`
		Valid      bool   `json:"valid"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	if !raw.Valid || raw.MergedYAML == "" {
		return nil, fmt.Errorf("ci/lint returned invalid or empty config for project %d @ %s", projectID, ref)
	}
	return []byte(raw.MergedYAML), nil
}

// --- TriggerPipelineRun ---

// TriggerPipelineRun creates a new pipeline on the given project/ref.
// Endpoint: POST /projects/:id/pipeline
func (c *gitLabClient) TriggerPipelineRun(projectID int, ref string, variables map[string]string) (*Pipeline, error) {
	type varEntry struct {
		Key          string `json:"key"`
		Value        string `json:"value"`
		VariableType string `json:"variable_type"`
	}
	body := struct {
		Ref       string     `json:"ref"`
		Variables []varEntry `json:"variables,omitempty"`
	}{Ref: ref}
	for k, v := range variables {
		body.Variables = append(body.Variables, varEntry{Key: k, Value: v, VariableType: "env_var"})
	}
	payload, _ := json.Marshal(body)
	u := fmt.Sprintf("%s/projects/%d/pipeline", c.apiURL, projectID)
	req, err := c.newRequest(http.MethodPost, u, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID        int    `json:"id"`
		Status    string `json:"status"`
		Ref       string `json:"ref"`
		WebURL    string `json:"web_url"`
		CreatedAt string `json:"created_at"`
		ProjectID int    `json:"project_id"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	createdAt, _ := time.Parse(time.RFC3339, raw.CreatedAt)
	pid := raw.ProjectID
	if pid == 0 {
		pid = projectID
	}
	return &Pipeline{
		ID:        raw.ID,
		Status:    raw.Status,
		Ref:       raw.Ref,
		WebURL:    raw.WebURL,
		CreatedAt: createdAt,
		ProjectID: pid,
	}, nil
}
