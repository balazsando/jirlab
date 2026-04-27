package integration

import (
	"errors"
	"strings"
	"testing"

	"github.com/andob/jirlab/internal/service"
)

// ---------------------------------------------------------------------------
// mockGitLabClient — minimal stub implementing service.GitLabClient
// ---------------------------------------------------------------------------

type mockGitLabClient struct {
	// Recorded call args
	createMRProjectID  int
	createMRSource     string
	createMRTarget     string
	createMRTitle      string
	downloadPatchProjectID int
	downloadPatchMRIID     int
	listJobsProjectID  int
	listJobsPipelineID int

	// Return values
	createMRResult     *service.MergeRequest
	createMRErr        error
	downloadPatchBytes []byte
	downloadPatchErr   error
	listJobsResult     []service.PipelineJob
	listJobsErr        error
}

func (m *mockGitLabClient) CreateBranch(int, string, string) (*service.Branch, error) {
	return nil, nil
}
func (m *mockGitLabClient) ListProjects() ([]service.Project, error)                 { return nil, nil }
func (m *mockGitLabClient) GetProject(int) (*service.Project, error)                 { return nil, nil }
func (m *mockGitLabClient) GetProjectByPath(string) (*service.Project, error)        { return nil, nil }
func (m *mockGitLabClient) ListMergeRequests(string) ([]service.MergeRequest, error) { return nil, nil }
func (m *mockGitLabClient) ListProjectMergeRequests(int, string) ([]service.MergeRequest, error) {
	return nil, nil
}
func (m *mockGitLabClient) MergeMR(int, int) error                            { return nil }
func (m *mockGitLabClient) CloseMR(int, int) error                            { return nil }
func (m *mockGitLabClient) GetCurrentUsername() (string, error)               { return "", nil }
func (m *mockGitLabClient) ListProjectBranches(int) ([]service.Branch, error) { return nil, nil }
func (m *mockGitLabClient) ListProjectTags(int) ([]service.Tag, error)        { return nil, nil }
func (m *mockGitLabClient) ListProjectPipelines(int, []string) ([]service.Pipeline, error) {
	return nil, nil
}
func (m *mockGitLabClient) GetMRApprovals(int, int) (bool, error)     { return false, nil }
func (m *mockGitLabClient) GetMRRecentComment(int, int) (bool, error) { return false, nil }

func (m *mockGitLabClient) CreateMR(projectID int, sourceBranch, targetBranch, title string) (*service.MergeRequest, error) {
	m.createMRProjectID = projectID
	m.createMRSource = sourceBranch
	m.createMRTarget = targetBranch
	m.createMRTitle = title
	return m.createMRResult, m.createMRErr
}

func (m *mockGitLabClient) DownloadMRPatch(projectID, mrIID int) ([]byte, error) {
	m.downloadPatchProjectID = projectID
	m.downloadPatchMRIID = mrIID
	return m.downloadPatchBytes, m.downloadPatchErr
}

func (m *mockGitLabClient) ListPipelineJobs(projectID, pipelineID int) ([]service.PipelineJob, error) {
	m.listJobsProjectID = projectID
	m.listJobsPipelineID = pipelineID
	return m.listJobsResult, m.listJobsErr
}

func (m *mockGitLabClient) GetCILint(int, string) ([]byte, error) { return nil, nil }
func (m *mockGitLabClient) TriggerPipelineRun(int, string, map[string]string) (*service.Pipeline, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// New tests
// ---------------------------------------------------------------------------

func TestCreateMRDelegates(t *testing.T) {
	want := &service.MergeRequest{ID: 42, WebURL: "https://gitlab.example.com/-/merge_requests/42"}
	mock := &mockGitLabClient{createMRResult: want}
	svc := NewGitLabService(mock)

	got, err := svc.CreateMR(10, "feature/my-branch", "develop", "My MR Title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID: want %d, got %d", want.ID, got.ID)
	}
	if mock.createMRProjectID != 10 {
		t.Errorf("projectID: want 10, got %d", mock.createMRProjectID)
	}
	if mock.createMRSource != "feature/my-branch" {
		t.Errorf("sourceBranch: want feature/my-branch, got %q", mock.createMRSource)
	}
	if mock.createMRTarget != "develop" {
		t.Errorf("targetBranch: want develop, got %q", mock.createMRTarget)
	}
	if mock.createMRTitle != "My MR Title" {
		t.Errorf("title: want 'My MR Title', got %q", mock.createMRTitle)
	}
}

func TestCreateMRPropagatesError(t *testing.T) {
	mock := &mockGitLabClient{createMRErr: errors.New("network error")}
	svc := NewGitLabService(mock)
	_, err := svc.CreateMR(1, "feat", "main", "title")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDownloadMRPatchDelegates(t *testing.T) {
	want := []byte("patch content")
	mock := &mockGitLabClient{downloadPatchBytes: want}
	svc := NewGitLabService(mock)

	got, err := svc.DownloadMRPatch(99, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("patch bytes: want %q, got %q", want, got)
	}
	if mock.downloadPatchProjectID != 99 {
		t.Errorf("projectID: want 99, got %d", mock.downloadPatchProjectID)
	}
	if mock.downloadPatchMRIID != 7 {
		t.Errorf("mrIID: want 7, got %d", mock.downloadPatchMRIID)
	}
}

func TestEnrichPipelinesWithJobs(t *testing.T) {
	jobs := []service.PipelineJob{
		{ID: 1, Name: "build", Status: "success"},
		{ID: 2, Name: "test", Status: "success"},
		{ID: 3, Name: "deploy", Status: "running"},
	}
	mock := &mockGitLabClient{listJobsResult: jobs}
	svc := &gitLabService{client: mock}

	pipelines := []service.Pipeline{
		{ID: 100, ProjectID: 5, Status: "running"},
	}
	enriched := svc.enrichPipelinesWithJobs(pipelines)

	if len(enriched) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(enriched))
	}
	if enriched[0].JobsTotal != 3 {
		t.Errorf("JobsTotal: want 3, got %d", enriched[0].JobsTotal)
	}
	if enriched[0].JobsSuccess != 2 {
		t.Errorf("JobsSuccess: want 2, got %d", enriched[0].JobsSuccess)
	}
	if mock.listJobsProjectID != 5 {
		t.Errorf("projectID passed to ListPipelineJobs: want 5, got %d", mock.listJobsProjectID)
	}
	if mock.listJobsPipelineID != 100 {
		t.Errorf("pipelineID passed to ListPipelineJobs: want 100, got %d", mock.listJobsPipelineID)
	}
}

func TestEnrichPipelinesWithJobsEmptySlice(t *testing.T) {
	mock := &mockGitLabClient{}
	svc := &gitLabService{client: mock}
	result := svc.enrichPipelinesWithJobs(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestBuildBranchNameBasic(t *testing.T) {
	issue := service.Issue{Key: "PROJ-123", Summary: "Implement login"}
	got := BuildBranchName(issue)
	want := "feature/PROJ-123-implement-login"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildBranchNameSpecialChars(t *testing.T) {
	issue := service.Issue{Key: "PROJ-42", Summary: "Fix bug: don't crash on nil!"}
	got := BuildBranchName(issue)
	// Should not contain colons, apostrophes, exclamation marks
	for _, bad := range []string{":", "'", "!", " "} {
		if strings.Contains(got, bad) {
			t.Errorf("branch name %q contains disallowed char %q", got, bad)
		}
	}
	if !strings.HasPrefix(got, "feature/PROJ-42-") {
		t.Errorf("expected prefix feature/PROJ-42-, got %q", got)
	}
}

func TestBuildBranchNameTruncation(t *testing.T) {
	issue := service.Issue{
		Key:     "PROJ-1",
		Summary: "This is a very long summary that exceeds the fifty character limit for slug generation in branch names",
	}
	got := BuildBranchName(issue)
	// Slug portion is after "feature/PROJ-1-"
	prefix := "feature/PROJ-1-"
	if !strings.HasPrefix(got, prefix) {
		t.Errorf("expected prefix %q, got %q", prefix, got)
	}
	slug := got[len(prefix):]
	if len(slug) > 50 {
		t.Errorf("slug too long: %d chars (max 50), slug=%q", len(slug), slug)
	}
}

func TestBuildBranchNameLowercase(t *testing.T) {
	issue := service.Issue{Key: "PROJ-5", Summary: "Add UPPERCASE Feature"}
	got := BuildBranchName(issue)
	// The slug part (after the key) must be lowercase; the key itself stays uppercase
	prefix := "feature/PROJ-5-"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("expected prefix %q, got %q", prefix, got)
	}
	slug := got[len(prefix):]
	if slug != strings.ToLower(slug) {
		t.Errorf("slug is not lowercase: %q", slug)
	}
}
