package tui

import (
	"testing"

	"github.com/andob/jirlab/internal/service"
)

func TestIssueColorDefault(t *testing.T) {
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "other"}
	color := IssueColor(issue, "me", map[string]bool{}, map[string]mrInfo{})
	if color != colorWhite {
		t.Errorf("expected white, got %v", color)
	}
}

func TestIssueColorAssignedToMe(t *testing.T) {
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "me"}
	color := IssueColor(issue, "me", map[string]bool{}, map[string]mrInfo{})
	if color != colorDarkBlue {
		t.Errorf("expected dark blue, got %v", color)
	}
}

func TestIssueColorLocalBranch(t *testing.T) {
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: ""}
	color := IssueColor(issue, "me", map[string]bool{"PROJ-1": true}, map[string]mrInfo{})
	if color != colorLightBlue {
		t.Errorf("expected light blue, got %v", color)
	}
}

func TestIssueColorMRMine(t *testing.T) {
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "other"}
	mrs := map[string]mrInfo{"PROJ-1": {isMine: true, approved: false}}
	color := IssueColor(issue, "me", map[string]bool{}, mrs)
	if color != colorYellow {
		t.Errorf("expected yellow, got %v", color)
	}
}

func TestIssueColorMRApproved(t *testing.T) {
	// mine+approved → green (higher than plain "mine")
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "other"}
	mrs := map[string]mrInfo{"PROJ-1": {isMine: true, approved: true}}
	color := IssueColor(issue, "me", map[string]bool{}, mrs)
	if color != colorGreen {
		t.Errorf("expected green (my approved MR), got %v", color)
	}
}

func TestIssueColorMRFromOthers(t *testing.T) {
	// others+unapproved → red (highest issue priority)
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "other"}
	mrs := map[string]mrInfo{"PROJ-1": {isMine: false, approved: false}}
	color := IssueColor(issue, "me", map[string]bool{}, mrs)
	if color != colorRed {
		t.Errorf("expected red (others unapproved MR), got %v", color)
	}
}

func TestIssueColorMRFromOthersApproved(t *testing.T) {
	issue := service.Issue{Key: "PROJ-1", AssigneeKey: "other"}
	mrs := map[string]mrInfo{"PROJ-1": {isMine: false, approved: true}}
	color := IssueColor(issue, "me", map[string]bool{}, mrs)
	if color != colorOrange {
		t.Errorf("expected orange (others approved), got %v", color)
	}
}

func TestTimeLogColorEmpty(t *testing.T) {
	if c := TimeLogColor(0); c != colorRed {
		t.Errorf("0h: expected red, got %v", c)
	}
}

func TestTimeLogColorPartial(t *testing.T) {
	if c := TimeLogColor(4); c != colorYellow {
		t.Errorf("4h: expected yellow, got %v", c)
	}
}

func TestTimeLogColorFull(t *testing.T) {
	if c := TimeLogColor(8); c != colorGreen {
		t.Errorf("8h: expected green, got %v", c)
	}
	if c := TimeLogColor(10); c != colorGreen {
		t.Errorf("10h: expected green, got %v", c)
	}
}
