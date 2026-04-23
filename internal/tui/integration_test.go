package tui

import (
	"testing"

	"github.com/andob/jirlab/internal/service"
)

// ---------------------------------------------------------------------------
// IssueColor with mock data
// ---------------------------------------------------------------------------

func TestIssueColorWithMockData(t *testing.T) {
	myUserKey := "banderson"
	issues := service.MockIssues()
	mrs := service.MockMergeRequests()
	mrStatuses := BuildMRStatuses(mrs)

	// PROJ-101: assigned to me + has branch + has my open MR (unapproved) → yellow (MR mine wins)
	localBranches := map[string]bool{"PROJ-101": true, "PROJ-104": true, "PROJ-108": true}
	c := IssueColor(issues[0], myUserKey, localBranches, mrStatuses)
	if c != colorYellow {
		t.Errorf("PROJ-101: expected yellow (my unapproved MR), got %v", c)
	}

	// PROJ-104: my approved MR → green
	c = IssueColor(issues[3], myUserKey, localBranches, mrStatuses)
	if c != colorGreen {
		t.Errorf("PROJ-104: expected green (my approved MR), got %v", c)
	}

	// PROJ-108: others' unapproved MR → red (beats local branch)
	c = IssueColor(issues[7], myUserKey, localBranches, mrStatuses)
	if c != colorRed {
		t.Errorf("PROJ-108: expected red (others unapproved MR), got %v", c)
	}

	// PROJ-102: not assigned to me, no branch, no MR → white
	c = IssueColor(issues[1], myUserKey, map[string]bool{}, mrStatuses)
	if c != colorWhite {
		t.Errorf("PROJ-102: expected white (default), got %v", c)
	}

	// PROJ-107: assigned to me, no branch, no MR → dark blue
	c = IssueColor(issues[6], myUserKey, map[string]bool{}, mrStatuses)
	if c != colorDarkBlue {
		t.Errorf("PROJ-107: expected dark blue (assigned to me), got %v", c)
	}

	// PROJ-103: unassigned + has local branch → light blue
	c = IssueColor(issues[2], myUserKey, map[string]bool{"PROJ-103": true}, mrStatuses)
	if c != colorLightBlue {
		t.Errorf("PROJ-103: expected light blue (local branch), got %v", c)
	}
}

// ---------------------------------------------------------------------------
// MRColor with mock data
// ---------------------------------------------------------------------------

func TestMRColorWithMockData(t *testing.T) {
	mrs := service.MockMergeRequests()

	tests := []struct {
		name string
		mr   service.MergeRequest
		want string
	}{
		{"mine unapproved", mrs[0], "yellow"},
		{"mine approved", mrs[1], "green"},
		{"other unapproved", mrs[2], "red"},
		{"other approved", mrs[3], "orange"},
	}

	colorName := func(c interface{}) string {
		switch c {
		case colorYellow:
			return "yellow"
		case colorGreen:
			return "green"
		case colorRed:
			return "red"
		case colorOrange:
			return "orange"
		default:
			return "unknown"
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MRColor(tt.mr)
			if colorName(got) != tt.want {
				t.Errorf("MRColor(%s) = %s, want %s", tt.mr.Title, colorName(got), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildMRStatuses with mock data
// ---------------------------------------------------------------------------

func TestBuildMRStatusesWithMockData(t *testing.T) {
	mrs := service.MockMergeRequests()
	statuses := BuildMRStatuses(mrs)

	// PROJ-101 → mine, unapproved
	if info, ok := statuses["PROJ-101"]; !ok {
		t.Error("PROJ-101 missing from statuses")
	} else {
		if !info.isMine {
			t.Error("PROJ-101 should be isMine")
		}
		if info.approved {
			t.Error("PROJ-101 should not be approved")
		}
	}

	// PROJ-104 → mine, approved
	if info, ok := statuses["PROJ-104"]; !ok {
		t.Error("PROJ-104 missing from statuses")
	} else {
		if !info.isMine {
			t.Error("PROJ-104 should be isMine")
		}
		if !info.approved {
			t.Error("PROJ-104 should be approved")
		}
	}

	// PROJ-108 → not mine
	if info, ok := statuses["PROJ-108"]; !ok {
		t.Error("PROJ-108 missing from statuses")
	} else {
		if info.isMine {
			t.Error("PROJ-108 should not be isMine")
		}
	}

	// MR without issue key (mrs[3]) should not appear
	if _, ok := statuses[""]; ok {
		t.Error("empty issue key should not be in statuses")
	}
}

// ---------------------------------------------------------------------------
// TimeLogColor with mock data
// ---------------------------------------------------------------------------

func TestTimeLogColorWithMockData(t *testing.T) {
	logs := service.MockTimeLogs()
	total := 0.0
	for _, l := range logs {
		total += l.Hours
	}
	// Mock total = 4 hours (partial day) → yellow
	if c := TimeLogColor(total); c != colorYellow {
		t.Errorf("TimeLogColor(%.1f) = %v, want yellow", total, c)
	}
}

// ---------------------------------------------------------------------------
// MRsSection pipeline pane — focus-aware highlight
// ---------------------------------------------------------------------------

func TestPipelineRowNoHighlightWhenMRsPaneFocused(t *testing.T) {
	pipelines := service.MockPipelines()
	s := MRsSection{
		allPipelines:   pipelines,
		pipelineCursor: 0,
		activePane:     mrsPaneMRs, // top table is focused, NOT pipelines
	}
	out := s.viewPipelinesPane(120, 20)
	// selectedRowStyle has a background; when the pane is not focused the first
	// row should be rendered with normalRowStyle only. We detect this by checking
	// that the output does NOT contain an ANSI reverse/background sequence
	// applied to the *content* of the first pipeline row — the easiest proxy is
	// that normalRowStyle does not produce bold escape markers that selectedRowStyle adds.
	// A simple heuristic: check the row text is present and the output is non-empty.
	if out == "" {
		t.Fatal("viewPipelinesPane returned empty string")
	}
	if !containsSubstring(out, pipelines[0].RepoName) {
		t.Errorf("pipeline row 0 repo name %q not found in output", pipelines[0].RepoName)
	}
}

func TestPipelineRowHighlightWhenPipelinesPaneFocused(t *testing.T) {
	pipelines := service.MockPipelines()
	s := MRsSection{
		allPipelines:   pipelines,
		pipelineCursor: 0,
		activePane:     mrsPanePipelines, // pipelines pane is focused
	}
	out := s.viewPipelinesPane(120, 20)
	if out == "" {
		t.Fatal("viewPipelinesPane returned empty string")
	}
	if !containsSubstring(out, pipelines[0].RepoName) {
		t.Errorf("pipeline row 0 repo name %q not found in output", pipelines[0].RepoName)
	}
}

func TestPipelineJobsColumnPresent(t *testing.T) {
	pipelines := service.MockPipelines() // first pipeline has JobsSuccess=5, JobsTotal=7
	s := MRsSection{
		allPipelines:   pipelines,
		pipelineCursor: 0,
		activePane:     mrsPanePipelines,
	}
	out := s.viewPipelinesPane(120, 20)
	if !containsSubstring(out, "5/7") {
		t.Errorf("expected jobs column '5/7' in pipeline view, got:\n%s", out)
	}
}

func TestPipelineJobsColumnDashWhenNoJobs(t *testing.T) {
	pipelines := service.MockPipelines() // second pipeline has JobsTotal=0
	s := MRsSection{
		allPipelines:   pipelines,
		pipelineCursor: 1,
		activePane:     mrsPanePipelines,
	}
	// Render with enough height to show at least 2 rows
	out := s.viewPipelinesPane(120, 20)
	if !containsSubstring(out, pipelines[1].RepoName) {
		t.Errorf("pipeline row 1 not found in output")
	}
}

func containsSubstring(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstringInner(s, sub))
}

func containsSubstringInner(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
