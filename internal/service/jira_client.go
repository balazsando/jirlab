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

// JiraClient defines operations against the Jira REST API.
type JiraClient interface {
	GetBoardIssues(boardID string, filter Filter) ([]Issue, error)
	GetIssue(key string) (*Issue, error)
	GetTransitions(issueKey string) ([]Transition, error)
	TransitionIssue(issueKey, transitionID string) error
	AddComment(issueKey, body string) error
	GetWorklogsForDate(accountID string, date time.Time) ([]TimeLog, error)
	// AssignIssue sets the assignee to the given accountID. Pass empty string to unassign.
	AssignIssue(issueKey, accountID string) error
	// AddWorklog posts a worklog entry starting at 'started' for 'seconds' seconds.
	AddWorklog(issueKey string, started time.Time, seconds int) error
	// GetCurrentUserAccountID returns the accountId of the authenticated user via /3/myself.
	GetCurrentUserAccountID() (string, error)
	// GetBaseURL returns the Jira base URL (e.g. https://company.atlassian.net).
	GetBaseURL() string
}

// jiraTimeFormats lists the timestamp layouts Jira uses in descending specificity.
// Jira omits the colon in timezone offsets (+0100 instead of +01:00),
// which is not valid RFC 3339 and causes Go's standard time.Time decoder to fail.
var jiraTimeFormats = []string{
	"2006-01-02T15:04:05.999-0700",
	"2006-01-02T15:04:05.999Z0700",
	"2006-01-02T15:04:05-0700",
	time.RFC3339Nano,
	time.RFC3339,
}

// JiraTime wraps time.Time with a JSON decoder that handles Jira's timestamp format.
type JiraTime struct{ time.Time }

func (jt *JiraTime) UnmarshalJSON(data []byte) error {
	// Strip surrounding quotes
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" {
		return nil
	}
	for _, layout := range jiraTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			jt.Time = t
			return nil
		}
	}
	return fmt.Errorf("jira: cannot parse time %q", s)
}

// parseJiraTime parses a bare string (not JSON-encoded) using the same formats.
func parseJiraTime(s string) (time.Time, error) {
	for _, layout := range jiraTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("jira: cannot parse time %q", s)
}

// extractADFText walks an Atlassian Document Format (ADF) JSON blob and
// returns its plain-text content. Also handles plain string comments returned
// by Jira API v2.
func extractADFText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first (API v2 worklogs often return plain text).
	var plainStr string
	if err := json.Unmarshal(raw, &plainStr); err == nil {
		return strings.TrimSpace(plainStr)
	}
	// Try ADF format (API v3).
	var doc struct {
		Content []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range doc.Content {
		for _, inline := range block.Content {
			if inline.Type == "text" {
				sb.WriteString(inline.Text)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

type jiraClient struct {
	baseURL    string
	apiURL     string
	email      string
	token      string
	httpClient *http.Client
}

func NewJiraClient(baseURL, apiURL, email, token string) JiraClient {
	return &jiraClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiURL:     strings.TrimRight(apiURL, "/"),
		email:      email,
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *jiraClient) GetBaseURL() string { return c.baseURL }

func (c *jiraClient) newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *jiraClient) do(req *http.Request, dest interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira API error %d: %s", resp.StatusCode, string(b))
	}
	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("jira response decode error: %w", err)
		}
	}
	return nil
}

// getActiveSprintID returns the first active sprint ID and name for a board.
func (c *jiraClient) getActiveSprintID(boardID string) (int, string, error) {
	url := fmt.Sprintf("%s/rest/agile/1.0/board/%s/sprint?state=active", c.baseURL, boardID)
	req, err := c.newRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	var resp struct {
		Values []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := c.do(req, &resp); err != nil {
		return 0, "", err
	}
	if len(resp.Values) == 0 {
		return 0, "", fmt.Errorf("no active sprint found for board %s", boardID)
	}
	return resp.Values[0].ID, resp.Values[0].Name, nil
}

// jiraIssueFields covers all fields we request from Jira API v2.
type jiraIssueFields struct {
	Summary string `json:"summary"`
	Status  struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"status"`
	Assignee *struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Priority *struct {
		Name string `json:"name"`
	} `json:"priority"`
	IssueType struct {
		Name string `json:"name"`
	} `json:"issuetype"`
	Parent *struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
	} `json:"parent"`
	Description string   `json:"description"`
	Created     JiraTime `json:"created"`
	Updated     JiraTime `json:"updated"`
	FixVersions []struct {
		Name string `json:"name"`
	} `json:"fixVersions"`
	Comment struct {
		Comments []struct {
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body    string   `json:"body"`
			Created JiraTime `json:"created"`
		} `json:"comments"`
	} `json:"comment"`
	// customfield_10020 = sprint (array in agile context)
	Sprint []struct {
		Name string `json:"name"`
	} `json:"customfield_10020"`
}

const fieldsList = "summary,status,assignee,priority,issuetype,parent,subtasks,description,comment,fixVersions,customfield_10020,created,updated"

func fieldsToIssue(key string, f jiraIssueFields) Issue {
	issue := Issue{
		Key:               key,
		Summary:           f.Summary,
		Status:            f.Status.Name,
		StatusCategoryKey: f.Status.StatusCategory.Key,
		IssueType:         f.IssueType.Name,
		Description:       f.Description,
		CreatedAt:         f.Created.Time,
		UpdatedAt:         f.Updated.Time,
	}
	if f.Assignee != nil {
		issue.Assignee = f.Assignee.DisplayName
		issue.AssigneeKey = f.Assignee.AccountID
	}
	if f.Priority != nil {
		issue.Priority = f.Priority.Name
	}
	if f.Parent != nil {
		issue.ParentKey = f.Parent.Key
		issue.ParentSummary = f.Parent.Fields.Summary
	}
	if len(f.Sprint) > 0 {
		issue.SprintName = f.Sprint[0].Name
	}
	for _, fv := range f.FixVersions {
		if fv.Name != "" {
			issue.FixVersions = append(issue.FixVersions, fv.Name)
		}
	}
	for _, cm := range f.Comment.Comments {
		issue.Comments = append(issue.Comments, IssueComment{
			Author:  cm.Author.DisplayName,
			Body:    cm.Body,
			Created: cm.Created.Time,
		})
	}
	return issue
}

func (c *jiraClient) GetBoardIssues(boardID string, filter Filter) ([]Issue, error) {
	sprintID, _, err := c.getActiveSprintID(boardID)
	if err != nil {
		return nil, err
	}

	var all []Issue
	startAt, maxResults := 0, 100

	for {
		url := fmt.Sprintf(
			"%s/rest/agile/1.0/sprint/%d/issue?startAt=%d&maxResults=%d&fields=%s",
			c.baseURL, sprintID, startAt, maxResults, fieldsList,
		)
		req, err := c.newRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Total  int `json:"total"`
			Issues []struct {
				Key    string          `json:"key"`
				Fields jiraIssueFields `json:"fields"`
			} `json:"issues"`
		}
		if err := c.do(req, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Issues {
			issue := fieldsToIssue(r.Key, r.Fields)
			if filter.Assignee != "" && !strings.EqualFold(issue.Assignee, filter.Assignee) {
				continue
			}
			if filter.SprintName != "" && !strings.Contains(strings.ToLower(issue.SprintName), strings.ToLower(filter.SprintName)) {
				continue
			}
			all = append(all, issue)
		}
		startAt += len(page.Issues)
		if startAt >= page.Total {
			break
		}
	}
	return all, nil
}

func (c *jiraClient) GetIssue(key string) (*Issue, error) {
	url := fmt.Sprintf("%s/2/issue/%s?fields=%s", c.apiURL, key, fieldsList)
	req, err := c.newRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Key    string          `json:"key"`
		Fields jiraIssueFields `json:"fields"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	issue := fieldsToIssue(raw.Key, raw.Fields)
	return &issue, nil
}

func (c *jiraClient) GetTransitions(issueKey string) ([]Transition, error) {
	url := fmt.Sprintf("%s/3/issue/%s/transitions", c.apiURL, issueKey)
	req, err := c.newRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	transitions := make([]Transition, len(raw.Transitions))
	for i, t := range raw.Transitions {
		transitions[i] = Transition{ID: t.ID, Name: t.Name, To: t.To.Name}
	}
	return transitions, nil
}

func (c *jiraClient) TransitionIssue(issueKey, transitionID string) error {
	url := fmt.Sprintf("%s/3/issue/%s/transitions", c.apiURL, issueKey)
	payload := fmt.Sprintf(`{"transition":{"id":"%s"}}`, transitionID)
	req, err := c.newRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *jiraClient) AddComment(issueKey, body string) error {
	url := fmt.Sprintf("%s/2/issue/%s/comment", c.apiURL, issueKey)
	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := c.newRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// GetWorklogsForDate returns all worklogs logged by the given Jira accountID on the given date.
// If accountID is empty, all worklogs for that date are returned (scoped to currentUser() via JQL).
func (c *jiraClient) GetWorklogsForDate(accountID string, date time.Time) ([]TimeLog, error) {
	dateStr := date.Format("2006-01-02")
	jql := fmt.Sprintf(`worklogAuthor = currentUser() AND worklogDate = "%s"`, dateStr)
	searchURL := fmt.Sprintf("%s/3/search/jql?jql=%s&fields=summary&maxResults=50",
		c.apiURL, url.QueryEscape(jql))

	req, err := c.newRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("worklog search request: %w", err)
	}

	var searchResp struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := c.do(req, &searchResp); err != nil {
		return nil, fmt.Errorf("worklog search failed: %w", err)
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var all []TimeLog
	for _, issue := range searchResp.Issues {
		wlURL := fmt.Sprintf("%s/2/issue/%s/worklog", c.apiURL, issue.Key)
		wlReq, err := c.newRequest(http.MethodGet, wlURL, nil)
		if err != nil {
			continue
		}
		var wlResp struct {
			Worklogs []struct {
				Author struct {
					AccountID string `json:"accountId"`
				} `json:"author"`
				TimeSpentSeconds int             `json:"timeSpentSeconds"`
				Started          string          `json:"started"`
				Comment          json.RawMessage `json:"comment"`
			} `json:"worklogs"`
		}
		if err := c.do(wlReq, &wlResp); err != nil {
			continue
		}
		for _, wl := range wlResp.Worklogs {
			if accountID != "" && wl.Author.AccountID != accountID {
				continue
			}
			started, err := parseJiraTime(wl.Started)
			if err != nil {
				continue
			}
			if started.Before(startOfDay) || !started.Before(endOfDay) {
				continue
			}
			duration := time.Duration(wl.TimeSpentSeconds) * time.Second
			all = append(all, TimeLog{
				IssueKey:    issue.Key,
				Description: issue.Fields.Summary,
				Comment:     extractADFText(wl.Comment),
				From:        started,
				To:          started.Add(duration),
				Hours:       float64(wl.TimeSpentSeconds) / 3600.0,
			})
		}
	}
	return all, nil
}

func (c *jiraClient) AssignIssue(issueKey, accountID string) error {
	u := fmt.Sprintf("%s/2/issue/%s/assignee", c.apiURL, issueKey)
	var payload string
	if accountID == "" {
		payload = `{"accountId":null}`
	} else {
		payload = fmt.Sprintf(`{"accountId":%q}`, accountID)
	}
	req, err := c.newRequest(http.MethodPut, u, strings.NewReader(payload))
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *jiraClient) AddWorklog(issueKey string, started time.Time, seconds int) error {
	u := fmt.Sprintf("%s/2/issue/%s/worklog", c.apiURL, issueKey)
	startedStr := started.Format("2006-01-02T15:04:05.000-0700")
	payload := fmt.Sprintf(`{"started":%q,"timeSpentSeconds":%d,"comment":%q}`, startedStr, seconds, "implementation")
	req, err := c.newRequest(http.MethodPost, u, strings.NewReader(payload))
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// GetCurrentUserAccountID returns the accountId of the authenticated Jira user
// by calling the /3/myself endpoint (same approach as jirapick.zsh).
func (c *jiraClient) GetCurrentUserAccountID() (string, error) {
	u := fmt.Sprintf("%s/3/myself", c.apiURL)
	req, err := c.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	var raw struct {
		AccountID string `json:"accountId"`
	}
	if err := c.do(req, &raw); err != nil {
		return "", fmt.Errorf("get myself: %w", err)
	}
	if raw.AccountID == "" {
		return "", fmt.Errorf("get myself: empty accountId in response")
	}
	return raw.AccountID, nil
}
