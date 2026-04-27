package service

import (
	"strings"
	"testing"
)

func TestFormatUnifiedDiff_ModifiedFile(t *testing.T) {
	diffs := []mrDiffEntry{
		{
			OldPath: "main.go",
			NewPath: "main.go",
			Diff:    "@@ -1,3 +1,4 @@\n line1\n+new line\n line2\n line3\n",
		},
	}
	got := string(formatUnifiedDiff(diffs))
	want := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n line1\n+new line\n line2\n line3\n"
	if got != want {
		t.Errorf("modified file:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatUnifiedDiff_NewFile(t *testing.T) {
	diffs := []mrDiffEntry{
		{
			OldPath: "new.go",
			NewPath: "new.go",
			NewFile: true,
			Diff:    "@@ -0,0 +1,2 @@\n+line1\n+line2\n",
		},
	}
	got := string(formatUnifiedDiff(diffs))
	if !strings.Contains(got, "new file mode") {
		t.Error("expected 'new file mode' in output")
	}
	if !strings.Contains(got, "--- /dev/null") {
		t.Error("expected '--- /dev/null' for new file")
	}
	if !strings.Contains(got, "+++ b/new.go") {
		t.Error("expected '+++ b/new.go'")
	}
}

func TestFormatUnifiedDiff_DeletedFile(t *testing.T) {
	diffs := []mrDiffEntry{
		{
			OldPath:     "old.go",
			NewPath:     "old.go",
			DeletedFile: true,
			Diff:        "@@ -1,2 +0,0 @@\n-line1\n-line2\n",
		},
	}
	got := string(formatUnifiedDiff(diffs))
	if !strings.Contains(got, "deleted file mode") {
		t.Error("expected 'deleted file mode' in output")
	}
	if !strings.Contains(got, "+++ /dev/null") {
		t.Error("expected '+++ /dev/null' for deleted file")
	}
}

func TestFormatUnifiedDiff_RenamedFile(t *testing.T) {
	diffs := []mrDiffEntry{
		{
			OldPath:     "old/path.go",
			NewPath:     "new/path.go",
			RenamedFile: true,
			Diff:        "@@ -1,2 +1,2 @@\n line1\n-old\n+new\n",
		},
	}
	got := string(formatUnifiedDiff(diffs))
	if !strings.Contains(got, "rename from old/path.go") {
		t.Error("expected 'rename from'")
	}
	if !strings.Contains(got, "rename to new/path.go") {
		t.Error("expected 'rename to'")
	}
}

func TestFormatUnifiedDiff_EnsuresTrailingNewline(t *testing.T) {
	diffs := []mrDiffEntry{
		{
			OldPath: "a.go",
			NewPath: "a.go",
			Diff:    "@@ -1 +1 @@\n-old\n+new", // no trailing newline
		},
	}
	got := string(formatUnifiedDiff(diffs))
	if !strings.HasSuffix(got, "\n") {
		t.Error("output should end with a newline")
	}
}

func TestFormatUnifiedDiff_Empty(t *testing.T) {
	got := formatUnifiedDiff(nil)
	if len(got) != 0 {
		t.Errorf("empty input should produce empty output, got %q", got)
	}
}
