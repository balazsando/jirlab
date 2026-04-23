package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssigneeInitialsEmpty(t *testing.T) {
	if got := AssigneeInitials(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestAssigneeInitialsSingleName(t *testing.T) {
	if got := AssigneeInitials("Alice"); got != "A" {
		t.Errorf("expected A, got %q", got)
	}
}

func TestAssigneeInitialsFullName(t *testing.T) {
	if got := AssigneeInitials("Ben Anderson"); got != "BA" {
		t.Errorf("expected BA, got %q", got)
	}
}

func TestAssigneeInitialsMultipleWords(t *testing.T) {
	if got := AssigneeInitials("John Paul Smith"); got != "JS" {
		t.Errorf("expected JS (first+last), got %q", got)
	}
}

func TestScanReposFindsRepo(t *testing.T) {
	root := t.TempDir()

	// Create a fake repo: dir with .gitlab-ci.yml and git init
	repoDir := filepath.Join(root, "my-service")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ciFile := filepath.Join(repoDir, ".gitlab-ci.yml")
	if err := os.WriteFile(ciFile, []byte("# ci"), 0o644); err != nil {
		t.Fatal(err)
	}
	// git init so readGitBranch returns something
	cmd := exec.Command("git", "-C", repoDir, "init")
	_ = cmd.Run()

	repos, err := ScanRepos(root)
	if err != nil {
		t.Fatalf("ScanRepos error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "my-service" {
		t.Errorf("expected name my-service, got %q", repos[0].Name)
	}
	if repos[0].Path != repoDir {
		t.Errorf("expected path %q, got %q", repoDir, repos[0].Path)
	}
}

func TestScanReposSkipsNoCIFile(t *testing.T) {
	root := t.TempDir()

	// Dir without .gitlab-ci.yml
	plainDir := filepath.Join(root, "plain-project")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	repos, err := ScanRepos(root)
	if err != nil {
		t.Fatalf("ScanRepos error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestScanReposSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()

	// Hidden dir with .gitlab-ci.yml — should be skipped
	hiddenDir := filepath.Join(root, ".hidden-service")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, ".gitlab-ci.yml"), []byte("# ci"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos, err := ScanRepos(root)
	if err != nil {
		t.Fatalf("ScanRepos error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos (hidden dir skipped), got %d", len(repos))
	}
}
