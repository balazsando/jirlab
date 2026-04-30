package tui

// HelpEntry pairs a keyboard shortcut with its human-readable description.
type HelpEntry struct {
	Key  string
	Desc string
}

// GlobalKeys are always available regardless of which tab is active.
var GlobalKeys = []HelpEntry{
	{"1", "Sprint Board"},
	{"2", "Repositories"},
	{"3", "Merge Requests"},
	{"4", "Kubernetes"},
	{"5", "Chats"},
	{"0", "refresh"},
	{"?", "toggle help"},
	{"esc", "close modal"},
	{"q", "quit"},
}

// BoardKeys are the hotkeys available in the Sprint Board section.
var BoardKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"enter/d", "description"},
	{"→", "command palette"},
	{"a", "assign to me"},
	{"u", "unassign"},
	{"i", "move to In Progress (confirm)"},
	{"t", "move to Testing (confirm)"},
	{"r", "open MR in browser"},
	{"w", "open Jira ticket in browser"},
	{"m", "merge MR (confirm)"},
	{"b", "create branch in repo"},
	{"n", "navigate to repo folder"},
	{"c", "add comment (template picker)"},
	{"l", "log full day 8-16 (confirm)"},
	{"h", "log half day 8-12 (confirm)"},
	{"f", "toggle filter"},
}

// ReposKeys are the hotkeys available in the Repositories section.
// Ordered by priority: navigation → discovery → trigger actions → utilities.
var ReposKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter/n", "checkout"},
	{"r", "branches subtab"},
	{"t", "tags subtab"},
	{"m", "merge requests subtab"},
	{"p", "pipelines subtab"},
	{"i", "trigger pipeline"},
	{"g", "git push all"},
	{"c", "create MR for branch"},
	{"b", "create branch"},
	{"d", "checkout default branch"},
	{"o", "open in editor"},
	{"w", "open URL in browser"},
	{"v", "copy version to clipboard"},
}

// MRsKeys are the hotkeys available in the Merge Requests section.
var MRsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"enter/w", "open MR in browser"},
	{"p", "create patch file"},
	{"c", "checkout MR branch"},
	{"m", "merge MR (confirm)"},
	{"d", "close MR (confirm)"},
}

// TrackerKeys are the hotkeys available in the Time Tracker section.
var TrackerKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "cycle panes"},
	{"o", "notes pane"},
	{"t", "templates pane"},
	{"l", "log hours"},
	{"←/→", "prev/next day"},
	{"w", "open timetracker in browser"},
}

// TrackerNotesKeys are the hotkeys for the Time Tracker — Notes pane.
var TrackerNotesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "cycle panes"},
	{"o", "notes pane (current)"},
	{"t", "templates pane"},
	{"n", "new note"},
	{"enter", "open note"},
	{"d", "delete note"},
	{"s", "cycle sort"},
	{"f", "cycle category filter"},
}

// TrackerTemplatesKeys are the hotkeys for the Time Tracker — Templates pane.
var TrackerTemplatesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "cycle panes"},
	{"o", "notes pane"},
	{"t", "templates pane (current)"},
	{"n", "new template"},
	{"enter", "edit template"},
	{"d", "delete template"},
}

// ChatsTopKeys are the hotkeys for the Chats section top pane (Teams conversations).
var ChatsTopKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch to notes/templates pane"},
	{"enter", "authenticate (if required)"},
	{"r", "refresh chats"},
}

// ChatsNotesKeys are the hotkeys for the Chats section bottom pane.
var ChatsNotesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch to chats pane"},
	{"o", "notes sub-pane"},
	{"t", "templates sub-pane"},
	{"n", "new note/template"},
	{"enter", "open note / edit template"},
	{"d", "delete"},
	{"s", "cycle sort (notes)"},
	{"f", "cycle category filter (notes)"},
}

// KubeKeys are the hotkeys available in the Kubernetes section.
var KubeKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"enter", "select config"},
	{"t", "switch to test env"},
	{"s", "switch to staging/preprod env"},
	{"p", "switch to production env"},
	{"tab", "switch pane"},
	{"o", "pods tab"},
	{"e", "services tab"},
	{"m", "deployments tab"},
	{"x", "exec shell (pods tab)"},
	{"l", "pod logs (only in pods tab)"},
	{"r", "scale replicas (deployments)"},
	{"d", "describe resource"},
	{"y", "get resource YAML"},
}

// --- Per-pane key sets ---

// ReposMainKeys is the key set for the Repositories main pane (repo list).
var ReposMainKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter/n", "navigate"},
	{"i", "trigger pipeline"},
	{"g", "git push all"},
	{"c", "create MR for current branch"},
	{"b", "create branch"},
	{"d", "checkout default branch"},
	{"r", "branches subtab"},
	{"t", "tags subtab"},
	{"m", "merge requests subtab"},
	{"p", "pipelines subtab"},
	{"o", "open in editor"},
	{"w", "open URL in browser"},
	{"v", "copy version to clipboard"},
}

// ReposBranchesKeys is the key set for the Repositories — Branches subtab.
var ReposBranchesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter", "checkout branch"},
	{"w", "open in browser"},
}

// ReposTagsKeys is the key set for the Repositories — Tags subtab.
var ReposTagsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter", "checkout tag"},
}

// ReposSubMRsKeys is the key set for the Repositories — Merge Requests subtab.
var ReposSubMRsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter", "checkout MR branch"},
	{"w", "open in browser"},
}

// ReposPipelinesKeys is the key set for the Repositories — Pipelines subtab.
var ReposPipelinesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter/w", "open pipeline in browser"},
	{"i", "trigger new pipeline"},
}

// MRsMainKeys is the key set for the Merge Requests main pane.
var MRsMainKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter/w", "open MR in browser"},
	{"p", "download patch file"},
	{"c", "checkout branch"},
	{"m", "merge MR (confirm)"},
	{"d", "close MR (confirm)"},
}

// MRsPipelinesKeys is the key set for the Merge Requests — Pipelines pane.
var MRsPipelinesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter/w", "open pipeline in browser"},
}

// KubeConfigsKeys is the key set for the Kubernetes — Configs pane.
var KubeConfigsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"enter", "select config"},
	{"t", "switch to test env"},
	{"s", "switch to staging env"},
	{"p", "switch to prod env"},
}

// KubePodsKeys is the key set for the Kubernetes — Pods pane.
var KubePodsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"x", "exec shell"},
	{"l", "view logs"},
	{"d", "describe pod"},
	{"y", "get YAML"},
}

// KubeServicesKeys is the key set for the Kubernetes — Services pane.
var KubeServicesKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"d", "describe service"},
	{"y", "get YAML"},
}

// KubeDeploymentsKeys is the key set for the Kubernetes — Deployments pane.
var KubeDeploymentsKeys = []HelpEntry{
	{"↑/k", "up"},
	{"↓/j", "down"},
	{"tab", "switch pane"},
	{"→", "command palette"},
	{"r", "scale replicas"},
	{"d", "describe deployment"},
	{"y", "get YAML"},
}
