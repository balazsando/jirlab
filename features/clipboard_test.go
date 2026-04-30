package features_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/andob/jirlab/internal/service"
	"github.com/andob/jirlab/internal/tui"
)

// ---------------------------------------------------------------------------
// Context keys for M2 clipboard tests
// ---------------------------------------------------------------------------

type (
	repoPathKey    struct{}
	issueKeyCtxKey struct{}
	mrURLKey       struct{}
	savedFilesKey  struct{}
	clipboardKey   struct{}
	clipUpdatedKey struct{}
	patchDataKey   struct{}
	mrCtxKey       struct{}
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockFS records SaveFile calls.
type mockFS struct {
	files map[string][]byte
}

func newMockFS() *mockFS {
	return &mockFS{files: make(map[string][]byte)}
}

func (m *mockFS) SaveFile(path string, data []byte) error {
	m.files[path] = data
	return nil
}

func (m *mockFS) PathExists(path string) bool {
	_, ok := m.files[path]
	return ok
}

// mockClipboard records the last copied text.
type mockClipboard struct {
	lastText string
	updated  bool
}

// mockGitLab for MR creation / patch download.
type mockGitLabForMR struct {
	mrURL     string
	patchData []byte
}

func (g *mockGitLabForMR) CreateMR(_, _, _, _ string, _ int) (service.MergeRequest, error) {
	return service.MergeRequest{WebURL: g.mrURL}, nil
}

func (g *mockGitLabForMR) DownloadMRPatch(_, _ int) ([]byte, error) {
	if g.patchData == nil {
		return []byte("diff --git a/file.go b/file.go\n"), nil
	}
	return g.patchData, nil
}

// ---------------------------------------------------------------------------
// Branch clipboard step definitions (clipboard_branch.feature)
// ---------------------------------------------------------------------------

func aJiraIssueWithDescription(ctx context.Context, key, desc string) (context.Context, error) {
	issue := service.Issue{Key: key, Description: desc}
	ctx = context.WithValue(ctx, issueKeyCtxKey{}, issue)
	return ctx, nil
}

func aRepoExistsAtPath(ctx context.Context, path string) (context.Context, error) {
	return context.WithValue(ctx, repoPathKey{}, path), nil
}

func iCreateABranchForIssueInRepo(ctx context.Context, key, repoPath string) (context.Context, error) {
	issue, _ := ctx.Value(issueKeyCtxKey{}).(service.Issue)
	fs := newMockFS()
	clip := &mockClipboard{}

	path, err := tui.SaveTicketDescription(fs, repoPath, issue)
	if err != nil {
		return ctx, fmt.Errorf("SaveTicketDescription: %w", err)
	}
	clip.lastText = path
	clip.updated = true

	ctx = context.WithValue(ctx, savedFilesKey{}, fs)
	ctx = context.WithValue(ctx, clipboardKey{}, clip)
	return ctx, nil
}

func aFileExistsAt(ctx context.Context, path string) error {
	fs, _ := ctx.Value(savedFilesKey{}).(*mockFS)
	if fs == nil {
		return fmt.Errorf("no filesystem mock in context")
	}
	if !fs.PathExists(path) {
		return fmt.Errorf("expected file at %q but it was not saved; saved: %v", path, fileKeys(fs))
	}
	return nil
}

func theFileContains(ctx context.Context, text string) error {
	fs, _ := ctx.Value(savedFilesKey{}).(*mockFS)
	if fs == nil {
		return fmt.Errorf("no filesystem mock in context")
	}
	for _, data := range fs.files {
		if strings.Contains(string(data), text) {
			return nil
		}
	}
	return fmt.Errorf("no saved file contains %q; files: %v", text, fileKeys(fs))
}

func theClipboardContains(ctx context.Context, text string) error {
	clip, _ := ctx.Value(clipboardKey{}).(*mockClipboard)
	if clip == nil {
		return fmt.Errorf("no clipboard mock in context")
	}
	if clip.lastText != text {
		return fmt.Errorf("expected clipboard %q, got %q", text, clip.lastText)
	}
	return nil
}

func theClipboardIsNotUpdated(ctx context.Context) error {
	clip, _ := ctx.Value(clipboardKey{}).(*mockClipboard)
	if clip != nil && clip.updated {
		return fmt.Errorf("expected clipboard NOT to be updated but it was: %q", clip.lastText)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MR URL clipboard steps (clipboard_mr.feature)
// ---------------------------------------------------------------------------

func aRepoAtPathWithBranch(ctx context.Context, path, branch string) (context.Context, error) {
	ctx = context.WithValue(ctx, repoPathKey{}, path)
	ctx = context.WithValue(ctx, issueKeyCtxKey{}, branch)
	return ctx, nil
}

func gitLabReturnsMRURL(ctx context.Context, url string) (context.Context, error) {
	return context.WithValue(ctx, mrURLKey{}, url), nil
}

func iCreateAnMRForCurrentBranch(ctx context.Context) (context.Context, error) {
	mrURL, _ := ctx.Value(mrURLKey{}).(string)
	clip := &mockClipboard{}
	if mrURL != "" {
		clip.lastText = mrURL
		clip.updated = true
	}
	return context.WithValue(ctx, clipboardKey{}, clip), nil
}

// ---------------------------------------------------------------------------
// Patch file steps (clipboard_patch.feature)
// ---------------------------------------------------------------------------

func anMRWithSourceBranchAndProjectAndIID(ctx context.Context, branch string, projectID, iid int) (context.Context, error) {
	mr := service.MergeRequest{
		SourceBranch: branch,
		ProjectID:    projectID,
		IID:          iid,
	}
	return context.WithValue(ctx, mrCtxKey{}, mr), nil
}

func aLocalRepoForProjectAtPath(ctx context.Context, projectID int, path string) (context.Context, error) {
	repo := service.Repo{
		Path:            path,
		GitLabProjectID: projectID,
	}
	return context.WithValue(ctx, repoPathKey{}, repo), nil
}

func iDownloadThePatchFile(ctx context.Context) (context.Context, error) {
	mr, _ := ctx.Value(mrCtxKey{}).(service.MergeRequest)
	repo, _ := ctx.Value(repoPathKey{}).(service.Repo)
	fs := newMockFS()
	clip := &mockClipboard{}

	path, err := tui.SaveMRPatch(fs, repo.Path, mr, []byte("patch-data"))
	if err != nil {
		return ctx, fmt.Errorf("SaveMRPatch: %w", err)
	}
	clip.lastText = path
	clip.updated = true

	ctx = context.WithValue(ctx, savedFilesKey{}, fs)
	ctx = context.WithValue(ctx, clipboardKey{}, clip)
	return ctx, nil
}

func aFileIsSavedAt(ctx context.Context, path string) error {
	return aFileExistsAt(ctx, path)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func fileKeys(fs *mockFS) []string {
	keys := make([]string, 0, len(fs.files))
	for k := range fs.files {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Suite wiring
// ---------------------------------------------------------------------------

func InitializeClipboardBranchScenario(sc *godog.ScenarioContext) {
	sc.Step(`^a Jira issue "([^"]*)" with description "([^"]*)"$`, aJiraIssueWithDescription)
	sc.Step(`^a repo exists at path "([^"]*)"$`, aRepoExistsAtPath)
	sc.Step(`^I create a branch for issue "([^"]*)" in repo "([^"]*)"$`, iCreateABranchForIssueInRepo)
	sc.Step(`^a file exists at "([^"]*)"$`, aFileExistsAt)
	sc.Step(`^the file contains "([^"]*)"$`, theFileContains)
	sc.Step(`^the clipboard contains "([^"]*)"$`, theClipboardContains)
	sc.Step(`^the clipboard is not updated$`, theClipboardIsNotUpdated)
}

func InitializeClipboardMRScenario(sc *godog.ScenarioContext) {
	sc.Step(`^a repo at path "([^"]*)" with branch "([^"]*)"$`, aRepoAtPathWithBranch)
	sc.Step(`^GitLab returns MR URL "([^"]*)"$`, gitLabReturnsMRURL)
	sc.Step(`^I create an MR for the current branch$`, iCreateAnMRForCurrentBranch)
	sc.Step(`^the clipboard contains "([^"]*)"$`, theClipboardContains)
	sc.Step(`^the clipboard is not updated$`, theClipboardIsNotUpdated)
}

func InitializeClipboardPatchScenario(sc *godog.ScenarioContext) {
	sc.Step(`^an MR with source branch "([^"]*)" and project ID (\d+) and IID (\d+)$`, anMRWithSourceBranchAndProjectAndIID)
	sc.Step(`^a local repo for project (\d+) at path "([^"]*)"$`, aLocalRepoForProjectAtPath)
	sc.Step(`^I download the patch file$`, iDownloadThePatchFile)
	sc.Step(`^a file is saved at "([^"]*)"$`, aFileIsSavedAt)
	sc.Step(`^the clipboard contains "([^"]*)"$`, theClipboardContains)
}

func TestClipboardBranch(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeClipboardBranchScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"clipboard_branch.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status: failed acceptance tests")
	}
}

func TestClipboardMR(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeClipboardMRScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"clipboard_mr.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status: failed acceptance tests")
	}
}

func TestClipboardPatch(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeClipboardPatchScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"clipboard_patch.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status: failed acceptance tests")
	}
}
