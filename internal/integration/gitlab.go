package integration

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/andob/jirlab/internal/service"
	"gopkg.in/yaml.v3"
)

// GitLabService provides higher-level GitLab use-cases on top of GitLabClient.
type GitLabService interface {
	// CreateBranchFromIssue creates a remote branch named after the given issue.
	CreateBranchFromIssue(issue service.Issue, projectID int, baseBranch string) (*service.Branch, error)
	// ListProjects returns all accessible projects.
	ListProjects() ([]service.Project, error)
	// GetProject returns a single project by ID.
	GetProject(id int) (*service.Project, error)
	// GetMergeRequests returns open MRs created by or assigned to the authenticated user.
	GetMergeRequests() ([]service.MergeRequest, error)
	// GetAllMRsForProjects returns all open MRs for the given GitLab project IDs.
	GetAllMRsForProjects(projectIDs []int) ([]service.MergeRequest, error)
	// MergeMR merges a merge request.
	MergeMR(projectID, mrIID int) error
	// CloseMR closes (declines) a merge request.
	CloseMR(projectID, mrIID int) error
	// GetProjectBranches returns branches for a project, sorted default-first then alpha.
	GetProjectBranches(projectID int, defaultBranch string) ([]service.Branch, error)
	// GetProjectTags returns tags for a project, newest first.
	GetProjectTags(projectID int) ([]service.Tag, error)
	// GetProjectMRs returns open MRs for a project.
	GetProjectMRs(projectID int) ([]service.MergeRequest, error)
	// GetAllActivePipelines returns running/pending pipelines across the given projects.
	GetAllActivePipelines(projectIDs []int, repoNames map[int]string) ([]service.Pipeline, error)
	// GetProjectPipelines returns pipelines for a single project filtered by the given statuses.
	GetProjectPipelines(projectID int, repoName string, statuses []string) ([]service.Pipeline, error)
	// CreateMR opens a new merge request on the given project.
	CreateMR(projectID int, sourceBranch, targetBranch, title string) (*service.MergeRequest, error)
	// DownloadMRPatch fetches the raw .patch bytes for a merge request via the
	// GitLab API. Returns clear errors for 401/403 auth failures.
	DownloadMRPatch(projectID, mrIID int) ([]byte, error)
	// GetCIVariables parses the resolved CI config for variables with a description field.
	GetCIVariables(projectID int, ref string) ([]service.PipelineVariable, error)
	// TriggerPipeline creates a new pipeline run on the given project/ref.
	TriggerPipeline(projectID int, ref string, variables map[string]string) (*service.Pipeline, error)
}

type gitLabService struct {
	client service.GitLabClient
}

// NewGitLabService creates a GitLabService backed by the given GitLabClient.
func NewGitLabService(client service.GitLabClient) GitLabService {
	return &gitLabService{client: client}
}

// nonAlphanumRe matches any character that is not alphanumeric, dash, or underscore.
var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// BuildBranchName formats a branch name from a Jira issue.
// Result format: feature/<KEY>-<slugified-summary>
// e.g. "feature/PROJ-123-implement-login"
func BuildBranchName(issue service.Issue) string {
	slug := strings.ToLower(issue.Summary)
	slug = nonAlphanumRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	// Limit total slug length to keep branch names manageable
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return fmt.Sprintf("feature/%s-%s", issue.Key, slug)
}

func (s *gitLabService) CreateBranchFromIssue(issue service.Issue, projectID int, baseBranch string) (*service.Branch, error) {
	branchName := BuildBranchName(issue)
	return s.client.CreateBranch(projectID, branchName, baseBranch)
}

func (s *gitLabService) ListProjects() ([]service.Project, error) {
	return s.client.ListProjects()
}

func (s *gitLabService) GetProject(id int) (*service.Project, error) {
	return s.client.GetProject(id)
}

func (s *gitLabService) GetMergeRequests() ([]service.MergeRequest, error) {
	mrs, err := s.client.ListMergeRequests("opened")
	if err != nil {
		return nil, err
	}
	return s.enrichMRs(mrs), nil
}

// GetAllMRsForProjects fetches all open MRs for the given project IDs and
// merges them with the user-scoped MRs, deduplicating by ID.
func (s *gitLabService) GetAllMRsForProjects(projectIDs []int) ([]service.MergeRequest, error) {
	// Start with user-scoped MRs (created_by_me + assigned_to_me)
	scoped, err := s.client.ListMergeRequests("opened")
	if err != nil {
		return nil, err
	}

	seen := make(map[int]bool, len(scoped))
	result := make([]service.MergeRequest, len(scoped))
	copy(result, scoped)
	for _, mr := range scoped {
		seen[mr.ID] = true
	}

	for _, pid := range projectIDs {
		if pid == 0 {
			continue
		}
		projMRs, err := s.client.ListProjectMergeRequests(pid, "opened")
		if err != nil {
			continue // skip projects we can't read
		}
		for _, mr := range projMRs {
			if !seen[mr.ID] {
				seen[mr.ID] = true
				result = append(result, mr)
			}
		}
	}
	return s.enrichMRs(result), nil
}

func (s *gitLabService) MergeMR(projectID, mrIID int) error {
	return s.client.MergeMR(projectID, mrIID)
}

func (s *gitLabService) CloseMR(projectID, mrIID int) error {
	return s.client.CloseMR(projectID, mrIID)
}

func (s *gitLabService) GetProjectBranches(projectID int, defaultBranch string) ([]service.Branch, error) {
	branches, err := s.client.ListProjectBranches(projectID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(branches, func(i, j int) bool {
		ni, nj := branches[i].Name, branches[j].Name
		if ni == defaultBranch {
			return true
		}
		if nj == defaultBranch {
			return false
		}
		return strings.ToLower(ni) < strings.ToLower(nj)
	})
	return branches, nil
}

func (s *gitLabService) GetProjectTags(projectID int) ([]service.Tag, error) {
	return s.client.ListProjectTags(projectID)
}

func (s *gitLabService) GetProjectMRs(projectID int) ([]service.MergeRequest, error) {
	mrs, err := s.client.ListProjectMergeRequests(projectID, "opened")
	if err != nil {
		return nil, err
	}
	return s.enrichMRs(mrs), nil
}

func (s *gitLabService) GetProjectPipelines(projectID int, repoName string, statuses []string) ([]service.Pipeline, error) {
	pipelines, err := s.client.ListProjectPipelines(projectID, statuses)
	if err != nil {
		return nil, err
	}
	for i := range pipelines {
		pipelines[i].RepoName = repoName
	}
	return s.enrichPipelinesWithJobs(pipelines), nil
}

func (s *gitLabService) GetAllActivePipelines(projectIDs []int, repoNames map[int]string) ([]service.Pipeline, error) {
	statuses := []string{"running", "pending"}
	var result []service.Pipeline
	for _, pid := range projectIDs {
		if pid == 0 {
			continue
		}
		pipelines, err := s.client.ListProjectPipelines(pid, statuses)
		if err != nil {
			continue
		}
		repoName := repoNames[pid]
		for i := range pipelines {
			pipelines[i].RepoName = repoName
		}
		result = append(result, pipelines...)
	}
	// sort newest first
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return s.enrichPipelinesWithJobs(result), nil
}

func (s *gitLabService) CreateMR(projectID int, sourceBranch, targetBranch, title string) (*service.MergeRequest, error) {
	return s.client.CreateMR(projectID, sourceBranch, targetBranch, title)
}

func (s *gitLabService) DownloadMRPatch(projectID, mrIID int) ([]byte, error) {
	return s.client.DownloadMRPatch(projectID, mrIID)
}

// GetCIVariables calls the CI lint endpoint (which resolves all !include directives)
// and returns only the variables that have a non-empty description field,
// mirroring the GitLab browser "Run Pipeline" dialog.
func (s *gitLabService) GetCIVariables(projectID int, ref string) ([]service.PipelineVariable, error) {
	raw, err := s.client.GetCILint(projectID, ref)
	if err != nil {
		return nil, err
	}

	var doc struct {
		Variables map[string]interface{} `yaml:"variables"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse .gitlab-ci.yml: %w", err)
	}

	var result []service.PipelineVariable
	for key, val := range doc.Variables {
		switch v := val.(type) {
		case map[string]interface{}:
			desc, _ := v["description"].(string)
			if desc == "" {
				continue // only expose variables that have a description
			}
			defVal, _ := v["value"].(string)
			result = append(result, service.PipelineVariable{Key: key, Value: defVal, Description: desc})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

// TriggerPipeline creates a new pipeline run for the given project/ref.
func (s *gitLabService) TriggerPipeline(projectID int, ref string, variables map[string]string) (*service.Pipeline, error) {
	return s.client.TriggerPipelineRun(projectID, ref, variables)
}

// enrichPipelinesWithJobs concurrently fetches job counts for each pipeline,
// populating JobsSuccess and JobsTotal. Errors are silently swallowed (best-effort).
func (s *gitLabService) enrichPipelinesWithJobs(pipelines []service.Pipeline) []service.Pipeline {
	if len(pipelines) == 0 {
		return pipelines
	}
	type patch struct {
		idx         int
		jobsSuccess int
		jobsTotal   int
	}
	ch := make(chan patch, len(pipelines))
	var wg sync.WaitGroup
	for i, p := range pipelines {
		wg.Add(1)
		go func(i int, p service.Pipeline) {
			defer wg.Done()
			jobs, err := s.client.ListPipelineJobs(p.ProjectID, p.ID)
			if err != nil {
				return
			}
			success := 0
			for _, j := range jobs {
				if j.Status == "success" {
					success++
				}
			}
			ch <- patch{idx: i, jobsSuccess: success, jobsTotal: len(jobs)}
		}(i, p)
	}
	wg.Wait()
	close(ch)

	enriched := make([]service.Pipeline, len(pipelines))
	copy(enriched, pipelines)
	for pt := range ch {
		enriched[pt.idx].JobsSuccess = pt.jobsSuccess
		enriched[pt.idx].JobsTotal = pt.jobsTotal
	}
	return enriched
}

// enrichMRs concurrently fetches approval status and recent-comment status for
// each MR, populating the Approved and HasRecentComment fields.
// Errors are silently swallowed per MR (best-effort, non-critical enrichment).
func (s *gitLabService) enrichMRs(mrs []service.MergeRequest) []service.MergeRequest {
	if len(mrs) == 0 {
		return mrs
	}
	type patch struct {
		idx      int
		approved bool
		recent   bool
	}
	ch := make(chan patch, len(mrs))
	var wg sync.WaitGroup
	for i, mr := range mrs {
		wg.Add(1)
		go func(i int, mr service.MergeRequest) {
			defer wg.Done()
			approved, _ := s.client.GetMRApprovals(mr.ProjectID, mr.IID)
			recent, _ := s.client.GetMRRecentComment(mr.ProjectID, mr.IID)
			ch <- patch{idx: i, approved: approved, recent: recent}
		}(i, mr)
	}
	wg.Wait()
	close(ch)

	enriched := make([]service.MergeRequest, len(mrs))
	copy(enriched, mrs)
	for p := range ch {
		enriched[p.idx].Approved = p.approved
		enriched[p.idx].HasRecentComment = p.recent
	}
	return enriched
}
