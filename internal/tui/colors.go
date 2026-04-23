package tui

import (
	"strings"
	"time"
	"github.com/charmbracelet/lipgloss"
	"github.com/andob/jirlab/internal/service"
)

// hasRecentIssueComment reports whether any Jira comment was posted in the last 24 hours.
func hasRecentIssueComment(comments []service.IssueComment) bool {
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, c := range comments {
		if c.Created.After(cutoff) {
			return true
		}
	}
	return false
}

// IssueColor returns the foreground colour to use when rendering an issue row.
// Priority (highest → lowest):
//  1. Others' MR, unapproved             → red
//  2. My MR, approved                    → green
//  3. My MR, unapproved                  → yellow
//  4. New comment in last 24 h           → pink
//  5. Others' MR, approved               → orange
//  6. Local branch exists, no MR         → light blue
//  7. Assigned to me                     → dark blue
//  8. Default                            → white
func IssueColor(
	issue service.Issue,
	myUserKey string,
	localBranches map[string]bool,
	mrStatuses map[string]mrInfo,
) lipgloss.Color {
	if info, ok := mrStatuses[issue.Key]; ok {
		if !info.isMine && !info.approved {
			return colorRed
		}
		if info.isMine && info.approved {
			return colorGreen
		}
		if info.isMine {
			return colorYellow
		}
		if hasRecentIssueComment(issue.Comments) {
			return colorPink
		}
		return colorOrange
	}
	if hasRecentIssueComment(issue.Comments) {
		return colorPink
	}
	if localBranches[issue.Key] {
		return colorLightBlue
	}
	if myUserKey != "" && issue.AssigneeKey == myUserKey {
		return colorDarkBlue
	}
	return colorWhite
}

// mrInfo is a lightweight MR state lookup value used by IssueColor.
type mrInfo struct {
	isMine    bool
	approved  bool
	webURL    string // URL to open in browser
	mrIID     int    // project-scoped MR number (for merge/close API)
	projectID int    // GitLab project ID (for merge/close API)
}

// BuildMRStatuses converts a slice of MergeRequests into the map consumed by IssueColor.
// When multiple MRs map to the same issue key, priority:
// others-unapproved > mine-unapproved > mine-approved > others-approved.
func BuildMRStatuses(mrs []service.MergeRequest) map[string]mrInfo {
	m := make(map[string]mrInfo, len(mrs))
	for _, mr := range mrs {
		if mr.IssueKey == "" {
			continue
		}
		candidate := mrInfo{
			isMine:    mr.IsMine,
			approved:  mr.Approved,
			webURL:    mr.WebURL,
			mrIID:     mr.IID,
			projectID: mr.ProjectID,
		}
		existing, exists := m[mr.IssueKey]
		if !exists {
			m[mr.IssueKey] = candidate
			continue
		}
		// Replace existing if candidate has higher priority
		if mrPriority(candidate) > mrPriority(existing) {
			m[mr.IssueKey] = candidate
		}
	}
	return m
}

// mrPriority returns a numeric priority for BuildMRStatuses conflict resolution.
// Higher number = higher priority (shown first).
func mrPriority(info mrInfo) int {
	switch {
	case !info.isMine && !info.approved:
		return 4 // others, unapproved — highest
	case info.isMine && !info.approved:
		return 3
	case info.isMine && info.approved:
		return 2
	default:
		return 1 // others, approved — lowest
	}
}

// BuildProjectMRStatuses converts a slice of MergeRequests into a map keyed by
// GitLab ProjectID, used to colour repos that have no issue-key branch match.
// Same priority rules as BuildMRStatuses (others-unapproved > mine-unapproved > ...).
func BuildProjectMRStatuses(mrs []service.MergeRequest) map[int]mrInfo {
	m := make(map[int]mrInfo, len(mrs))
	for _, mr := range mrs {
		if mr.ProjectID == 0 {
			continue
		}
		candidate := mrInfo{
			isMine:    mr.IsMine,
			approved:  mr.Approved,
			webURL:    mr.WebURL,
			mrIID:     mr.IID,
			projectID: mr.ProjectID,
		}
		existing, exists := m[mr.ProjectID]
		if !exists {
			m[mr.ProjectID] = candidate
			continue
		}
		if mrPriority(candidate) > mrPriority(existing) {
			m[mr.ProjectID] = candidate
		}
	}
	return m
}

// MRColor returns the foreground colour for a merge request row.
// Priority (highest → lowest):
//  1. Others, unapproved           → red
//  2. Mine, approved               → green
//  3. Recent comment (last 24 h)   → pink
//  4. Mine, unapproved             → yellow
//  5. Others, approved             → orange
func MRColor(mr service.MergeRequest) lipgloss.Color {
	if !mr.IsMine && !mr.Approved {
		return colorRed
	}
	if mr.IsMine && mr.Approved {
		return colorGreen
	}
	if mr.HasRecentComment {
		return colorPink
	}
	if mr.IsMine {
		return colorYellow
	}
	return colorOrange
}

// TimeLogColor returns a colour representing how complete the day's logged hours are.
//   - 0h      → red
//   - >0 <8h  → yellow
//   - ≥8h     → green
func TimeLogColor(totalHours float64) lipgloss.Color {
	switch {
	case totalHours <= 0:
		return colorRed
	case totalHours < 8:
		return colorYellow
	default:
		return colorGreen
	}
}

// legendEntry pairs a colour with a human-readable label for legend bars.
type legendEntry struct {
	color lipgloss.Color
	label string
}

// buildLegendBar renders a compact single-line colour legend, dropping entries
// from the right when the result would exceed width (width=0 means no truncation).
func buildLegendBar(entries []legendEntry, width int) string {
	s := lipgloss.NewStyle()
	result := ""
	for i, e := range entries {
		sep := "  "
		if i == 0 {
			sep = ""
		}
		candidate := result + sep + s.Foreground(e.color).Render("● "+e.label)
		if width > 0 && lipgloss.Width(candidate) > width {
			break
		}
		result = candidate
	}
	return result
}

// legendLines returns one formatted line per entry for use inside a modal.
// The label is indented to column 13, matching the hotkey description alignment
// produced by fmt.Sprintf("  %-10s %s", key, desc).
func legendLines(entries []legendEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		bullet := lipgloss.NewStyle().Foreground(e.color).Render("●")
		label := strings.Repeat(" ", 10) + e.label
		out[i] = "  " + bullet + label
	}
	return out
}

// buildLegendGrid renders colour legend entries in a 2-column grid,
// matching the 2-column hotkey layout used in the help modal.
// colW is the width of each column (same value used for the hotkey columns).
func buildLegendGrid(entries []legendEntry, colW int) []string {
	plain := lipgloss.NewStyle()
	leftCol := plain.Width(colW)
	rightCol := plain.Width(colW)

	entryLine := func(e legendEntry) string {
		bullet := lipgloss.NewStyle().Foreground(e.color).Render("●")
		label := "  " + e.label
		return "  " + bullet + label
	}

	var rows []string
	for i := 0; i < len(entries); i += 2 {
		left := entryLine(entries[i])
		right := ""
		if i+1 < len(entries) {
			right = entryLine(entries[i+1])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top,
			leftCol.Render(left),
			rightCol.Render(right),
		))
	}
	return rows
}

// --- Sprint Board ---

var issueColorEntries = []legendEntry{
	{colorRed, "others: unapproved MR"},
	{colorGreen, "my MR: approved"},
	{colorYellow, "my MR"},
	{colorPink, "new comment"},
	{colorOrange, "others: approved MR"},
	{colorLightBlue, "local branch"},
	{colorDarkBlue, "assigned"},
}

// IssueColorLegendBar returns a truncated single-line colour legend for the board footer.
func IssueColorLegendBar(width int) string { return buildLegendBar(issueColorEntries, width) }

// IssueColorLegendLines returns per-entry lines for modal display.
func IssueColorLegendLines() []string { return legendLines(issueColorEntries) }

// --- Branches subtab ---

var branchColorEntries = []legendEntry{
	{colorGreen, "active branch"},
	{colorMuted, "protected"},
}

// BranchColorLegendBar returns a single-line colour legend for the branches subtab footer.
func BranchColorLegendBar(width int) string { return buildLegendBar(branchColorEntries, width) }

// BranchColorLegendLines returns per-entry lines for modal display.
func BranchColorLegendLines() []string { return legendLines(branchColorEntries) }

// --- Pipeline status ---

var pipelineStatusColorEntries = []legendEntry{
	{colorGreen, "running"},
	{colorYellow, "pending"},
	{colorRed, "failed"},
	{colorMuted, "canceled/skipped"},
}

// PipelineStatusLegendBar returns a single-line colour legend for pipeline status footer.
func PipelineStatusLegendBar(width int) string {
	return buildLegendBar(pipelineStatusColorEntries, width)
}

// PipelineStatusLegendLines returns per-entry lines for modal display.
func PipelineStatusLegendLines() []string { return legendLines(pipelineStatusColorEntries) }

// --- Repositories ---

var repoColorEntries = []legendEntry{
	{colorRed, "others unapproved"},
	{colorGreen, "mine approved"},
	{colorYellow, "mine"},
	{colorOrange, "others approved"},
	{colorLightBlue, "feature branch"},
}

// RepoColorLegendBar returns a truncated single-line colour legend for the repos footer.
func RepoColorLegendBar(width int) string { return buildLegendBar(repoColorEntries, width) }

// RepoColorLegendLines returns per-entry lines for modal display.
func RepoColorLegendLines() []string { return legendLines(repoColorEntries) }

// --- Merge Requests ---

var mrColorEntries = []legendEntry{
	{colorRed, "others unapproved"},
	{colorGreen, "mine approved"},
	{colorPink, "new comment"},
	{colorYellow, "mine"},
	{colorOrange, "others approved"},
}

// MRColorLegendBar returns a truncated single-line colour legend for the MRs footer.
func MRColorLegendBar(width int) string { return buildLegendBar(mrColorEntries, width) }

// MRColorLegendLines returns per-entry lines for modal display.
func MRColorLegendLines() []string { return legendLines(mrColorEntries) }

// --- Kubernetes configs ---

var kubeConfigColorEntries = []legendEntry{
	{colorGreen, "active"},
	{colorMuted, "monitoring"},
}

// KubeConfigLegendBar returns a truncated single-line legend for the kube config table footer.
func KubeConfigLegendBar(width int) string { return buildLegendBar(kubeConfigColorEntries, width) }

// KubeConfigLegendLines returns per-entry lines for modal display.
func KubeConfigLegendLines() []string { return legendLines(kubeConfigColorEntries) }

// --- Kubernetes pod status ---

var podStatusColorEntries = []legendEntry{
	{colorGreen, "running"},
	{colorYellow, "pending"},
	{colorRed, "failed"},
	{colorMuted, "completed/succeeded"},
}

// PodStatusLegendBar returns a truncated single-line legend for the pods footer.
func PodStatusLegendBar(width int) string { return buildLegendBar(podStatusColorEntries, width) }

// PodStatusLegendLines returns per-entry lines for modal display.
func PodStatusLegendLines() []string { return legendLines(podStatusColorEntries) }

// --- Time Tracker ---

var timeLogColorEntries = []legendEntry{
	{colorGreen, "≥8h logged"},
	{colorYellow, ">0h logged"},
	{colorRed, "0h logged"},
}

// TimeLogColorLegendBar returns a truncated single-line colour legend for the tracker footer.
func TimeLogColorLegendBar(width int) string { return buildLegendBar(timeLogColorEntries, width) }

// TimeLogColorLegendLines returns per-entry lines for modal display.
func TimeLogColorLegendLines() []string { return legendLines(timeLogColorEntries) }
