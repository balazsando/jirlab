package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/andob/jirlab/internal/integration"
)

func TestStageAllCommitPush_NoRemote(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := integration.NewGitCmdService()
	err := svc.StageAllCommitPush(dir, "initial commit")
	if err == nil {
		t.Fatal("expected error when no remote is configured")
	}
}

func TestStageAllCommitPush_NothingToCommit(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	svc := integration.NewGitCmdService()
	err := svc.StageAllCommitPush(dir, "empty commit")
	if err == nil {
		t.Fatal("expected error when nothing to commit")
	}
}
