package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/actions"
	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
	"github.com/andob/jirlab/internal/templates"
)

// --- Message types ---

type boardIssuesLoadedMsg struct {
	issues []service.Issue
	err    error
}

type issueDetailsLoadedMsg struct {
	issue *service.Issue
	err   error
}

type boardActionDoneMsg struct {
	message string
	reload  bool // trigger board reload after action
}

// --- BoardSection ---

type BoardSection struct {
	allIssues      []service.Issue // all sprint issues, unfiltered
	cursor         int
	loading        bool
	filtered       bool // f-key toggle
	statusMsg      string
	boardID        string
	jira           integration.JiraService
	gitlab         integration.GitLabService
	git            integration.GitCmdService
	fs             integration.FilesystemService
	shell          integration.ShellService
	myUserKey      string            // Jira account ID of current user
	jiraBaseURL    string            // e.g. https://company.atlassian.net
	mrStatuses     map[string]mrInfo // issue key -> MR info
	localBranches  map[string]bool   // issue key -> has local branch
	branchNames    map[string]string // issue key -> branch name for display
	repos          []service.Repo    // all known repos (for branch creation)
	repoPaths      map[string]string // issue key -> repo path (for navigate)
	termHeight     int               // terminal height, set on resize
	templatesStore templates.Store   // shared with TrackerSection
}

func newBoardSection(jira integration.JiraService, gitlab integration.GitLabService, git integration.GitCmdService, shell integration.ShellService, boardID, myUserKey, jiraBaseURL string) BoardSection {
	tmplStore, _ := templates.DefaultStore()
	if tmplStore == nil {
		tmplStore = templates.NewFilesystemStore(os.TempDir() + "/jirlab-templates")
	}
	return BoardSection{
		boardID:        boardID,
		jira:           jira,
		gitlab:         gitlab,
		git:            git,
		fs:             integration.NewFilesystemService(),
		shell:          shell,
		myUserKey:      myUserKey,
		jiraBaseURL:    jiraBaseURL,
		loading:        jira != nil && boardID != "",
		mrStatuses:     make(map[string]mrInfo),
		localBranches:  make(map[string]bool),
		branchNames:    make(map[string]string),
		repoPaths:      make(map[string]string),
		templatesStore: tmplStore,
	}
}

// fetchBoardIssuesCmd returns a command that loads ALL sprint issues async.
func fetchBoardIssuesCmd(jira integration.JiraService, boardID string) tea.Cmd {
	return func() tea.Msg {
		issues, err := jira.GetAllSprintIssues(boardID)
		if err != nil {
			return boardIssuesLoadedMsg{err: err}
		}
		return boardIssuesLoadedMsg{issues: issues}
	}
}

// getIssueDetailsCmd fetches full issue details async.
func getIssueDetailsCmd(jira integration.JiraService, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := jira.GetIssueDetails(key)
		if err != nil {
			return issueDetailsLoadedMsg{err: err}
		}
		return issueDetailsLoadedMsg{issue: issue}
	}
}

// --- Async action commands ---

// myAccountIDMsg carries the resolved Jira accountId of the current user.
type myAccountIDMsg struct{ id string }

// fetchMyAccountIDCmd resolves the current user's account ID via /3/myself.
func fetchMyAccountIDCmd(jira integration.JiraService) tea.Cmd {
	return func() tea.Msg {
		id, err := jira.GetMyAccountID()
		if err != nil {
			return errMsg{source: "board", err: fmt.Errorf("resolve my account: %w", err)}
		}
		return myAccountIDMsg{id: id}
	}
}

func assignToMeCmd(jira integration.JiraService, issueKey string) tea.Cmd {
	return func() tea.Msg {
		if err := jira.AssignToMe(issueKey); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Assigned %s to me", issueKey), reload: true}
	}
}

func unassignCmd(jira integration.JiraService, issueKey string) tea.Cmd {
	return func() tea.Msg {
		if err := jira.Unassign(issueKey); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Unassigned %s", issueKey), reload: true}
	}
}

func pickIssueCmd(jira integration.JiraService, issueKey string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.PickupIssue(jira, issueKey); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Picked %s → In Progress", issueKey), reload: true}
	}
}

func moveToTestingCmd(jira integration.JiraService, issueKey string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.MoveToTestEnv(jira, issueKey); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Moved %s → Testing", issueKey), reload: true}
	}
}

func addCommentCmd(jira integration.JiraService, issueKey, text string) tea.Cmd {
	return func() tea.Msg {
		if err := jira.AddComment(issueKey, text); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Comment added to %s", issueKey)}
	}
}

func logWorkBoardCmd(jira integration.JiraService, issueKey string, fromH, fromM, toH, toM int) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), fromH, fromM, 0, 0, now.Location())
		to := time.Date(now.Year(), now.Month(), now.Day(), toH, toM, 0, 0, now.Location())
		if err := jira.LogWork(issueKey, from, to); err != nil {
			return errMsg{source: "board", err: err}
		}
		hours := to.Sub(from).Hours()
		return boardActionDoneMsg{message: fmt.Sprintf("Logged %.0fh for %s", hours, issueKey)}
	}
}

// mergeMRCmd merges the MR via GitLab API.
func mergeMRCmd(gitlab integration.GitLabService, projectID, mrIID int, issueKey string) tea.Cmd {
	return func() tea.Msg {
		if err := gitlab.MergeMR(projectID, mrIID); err != nil {
			return errMsg{source: "board", err: err}
		}
		return boardActionDoneMsg{message: fmt.Sprintf("Merged MR for %s", issueKey), reload: false}
	}
}

// displayIssues returns the issues to render (all or filtered).
func (s BoardSection) displayIssues() []service.Issue {
	if !s.filtered {
		return s.allIssues
	}
	var result []service.Issue
	for _, iss := range s.allIssues {
		if isBoardFilterMatch(iss) {
			result = append(result, iss)
		}
	}
	return result
}

// isBoardFilterMatch mirrors jirasprint.zsh filter + always includes In Progress.
func isBoardFilterMatch(iss service.Issue) bool {
	allowedTypes := map[string]bool{
		"Development Sub-task": true,
		"Development task":     true,
		"Task":                 true,
		"Sub-task":             true,
		"Bug":                  true,
	}
	backlogTypes := map[string]bool{
		"Development Sub-task": true,
		"Sub-task":             true,
	}
	if !allowedTypes[iss.IssueType] {
		return false
	}
	switch iss.Status {
	case "Prio 1", "To Dev", "To Do", "In Progress", "Review", "Bug", "In Backlog":
		return true
	case "Backlog":
		return backlogTypes[iss.IssueType]
	}
	return false
}

func (s *BoardSection) selectedIssue() *service.Issue {
	issues := s.displayIssues()
	if len(issues) == 0 || s.cursor >= len(issues) {
		return nil
	}
	return &issues[s.cursor]
}

func (s BoardSection) update(msg tea.Msg) (BoardSection, tea.Cmd) {
	switch msg := msg.(type) {
	case boardIssuesLoadedMsg:
		s.loading = false
		if msg.err != nil {
			return s, func() tea.Msg { return errMsg{source: "board", err: msg.err} }
		}
		s.allIssues = msg.issues
		s.cursor = 0

	case boardActionDoneMsg:
		s.statusMsg = msg.message
		if msg.reload {
			s.loading = true
			return s, fetchBoardIssuesCmd(s.jira, s.boardID)
		}

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

func (s BoardSection) handleKey(msg tea.KeyMsg) (BoardSection, tea.Cmd) {
	issues := s.displayIssues()
	n := len(issues)

	switch msg.String() {
	case "j", "down":
		if s.cursor < n-1 {
			s.cursor++
		}

	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}

	case "d", "enter":
		if issue := s.selectedIssue(); issue != nil {
			key := issue.Key
			return s, tea.Batch(
				func() tea.Msg {
					return OpenModalMsg{M: LoadingModal{Message: fmt.Sprintf("Loading %s...", key)}}
				},
				getIssueDetailsCmd(s.jira, key),
			)
		}

	case "a": // assign to me
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			return s, assignToMeCmd(s.jira, issue.Key)
		}

	case "u": // unassign
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			return s, unassignCmd(s.jira, issue.Key)
		}

	case "i": // pick = move to In Progress + assign to me (confirm)
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			jr, key := s.jira, issue.Key
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   fmt.Sprintf("Move %s → In Progress?", key),
					onConfirm: pickIssueCmd(jr, key),
				}}
			}
		}

	case "t": // move to testing + unassign (confirm)
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			jr, key := s.jira, issue.Key
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   fmt.Sprintf("Move %s → Testing?", key),
					onConfirm: moveToTestingCmd(jr, key),
				}}
			}
		}

	case "r": // open MR in browser
		if issue := s.selectedIssue(); issue != nil {
			if info, ok := s.mrStatuses[issue.Key]; ok && info.webURL != "" {
				return s, openBrowserCmd(s.shell, info.webURL)
			}
			s.statusMsg = "No MR found for this issue"
		}

	case "m": // merge MR via GitLab API with confirmation
		if issue := s.selectedIssue(); issue != nil {
			if info, ok := s.mrStatuses[issue.Key]; ok && info.isMine && info.projectID != 0 {
				pID, mrIID, key := info.projectID, info.mrIID, issue.Key
				glab := s.gitlab
				return s, func() tea.Msg {
					return OpenModalMsg{M: ConfirmModal{
						message:   fmt.Sprintf("Merge MR for %s?", key),
						onConfirm: mergeMRCmd(glab, pID, mrIID, key),
					}}
				}
			} else {
				s.statusMsg = "No MR found for this issue"
			}
		}

	case "w": // open Jira ticket in browser
		if issue := s.selectedIssue(); issue != nil && s.jiraBaseURL != "" {
			url := strings.TrimRight(s.jiraBaseURL, "/") + "/browse/" + issue.Key
			return s, openBrowserCmd(s.shell, url)
		}

	case "n": // navigate to repo folder for this issue (only if branch exists)
		if issue := s.selectedIssue(); issue != nil {
			if repoPath, ok := s.repoPaths[issue.Key]; ok && repoPath != "" {
				return s, navigateCmd(s.git, repoPath)
			}
			s.statusMsg = "No local branch found for this issue"
		}

	case "c": // comment
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			key := issue.Key
			jr := s.jira
			store := s.templatesStore
			return s, func() tea.Msg {
				commentTitle := fmt.Sprintf("Comment on %s", key)
				onConfirm := func(text string) tea.Cmd { return addCommentCmd(jr, key, text) }
				if store != nil {
					tmpls, err := store.List()
					if err != nil {
						return errMsg{source: "templates", err: err}
					}
					if len(tmpls) > 0 {
						return OpenModalMsg{M: newCommentWithTemplateModal(tmpls, commentTitle, onConfirm)}
					}
				}
				return OpenModalMsg{M: newCommentInputModal(commentTitle, "", onConfirm)}
			}
		}

	case "l": // log work — ask issue key, then hours (auto-calculates start from existing logs)
		if issue := s.selectedIssue(); issue != nil && s.jira != nil {
			jr, key := s.jira, issue.Key
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message: fmt.Sprintf("Log work for %s?", key),
					onConfirm: func() tea.Msg {
						return OpenModalMsg{M: NewNumericHoursModal(
							fmt.Sprintf("Hours to log for %s", key),
							func(hours int) tea.Cmd {
								return logWorkBoardCmd(jr, key, 8, 0, 8+hours, 0)
							},
						)}
					},
				}}
			}
		}

	case "f": // toggle filter
		s.filtered = !s.filtered
		s.cursor = 0

	case "b": // pop up repo select modal to checkout new branch
		if issue := s.selectedIssue(); issue != nil && len(s.repos) > 0 {
			issueCopy := *issue // capture
			repos := s.repos
			names := make([]string, len(repos))
			for i, r := range repos {
				names[i] = r.Name
			}
			branchName := integration.BuildBranchName(issueCopy)
			git := s.git
			fs := s.fs
			shell := s.shell
			return s, func() tea.Msg {
				return OpenModalMsg{M: ListSelectModal{
					title: fmt.Sprintf("Create branch %s in:", branchName),
					items: names,
					onSelect: func(idx int) tea.Cmd {
						return checkoutNewBranchCmd(git, repos[idx], branchName, issueCopy, fs, shell)
					},
				}}
			}
		}

	case "right":
		if issue := s.selectedIssue(); issue != nil {
			cmds := s.buildCommandPalette(*issue)
			if len(cmds) > 0 {
				// 4 = tab bar + separator + header + headerSep
				anchorY := 4 + s.cursor
				return s, func() tea.Msg {
					return OpenModalMsg{M: CommandPaletteModal{commands: cmds, AnchorY: anchorY}}
				}
			}
		}
	}
	return s, nil
}

// buildCommandPalette returns context-sensitive actions for the selected board issue.
func (s BoardSection) buildCommandPalette(issue service.Issue) []CommandEntry {
	var entries []CommandEntry

	key := issue.Key
	jr := s.jira
	tmplStore := s.templatesStore

	// description
	entries = append(entries, CommandEntry{Key: "enter", Desc: "description", Cmd: tea.Batch(
		func() tea.Msg { return OpenModalMsg{M: LoadingModal{Message: fmt.Sprintf("Loading %s...", key)}} },
		getIssueDetailsCmd(jr, key),
	)})

	if jr != nil {
		entries = append(entries,
			CommandEntry{Key: "i", Desc: "in progress", Cmd: func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   fmt.Sprintf("Move %s → In Progress?", key),
					onConfirm: pickIssueCmd(jr, key),
				}}
			}},
			CommandEntry{Key: "t", Desc: "testing", Cmd: func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   fmt.Sprintf("Move %s → Testing?", key),
					onConfirm: moveToTestingCmd(jr, key),
				}}
			}},
			CommandEntry{Key: "a", Desc: "assign to me", Cmd: assignToMeCmd(jr, key)},
			CommandEntry{Key: "u", Desc: "unassign", Cmd: unassignCmd(jr, key)},
			CommandEntry{Key: "c", Desc: "comment", Cmd: func() tea.Msg {
				commentTitle := fmt.Sprintf("Comment on %s", key)
				onConfirm := func(text string) tea.Cmd { return addCommentCmd(jr, key, text) }
				if tmplStore != nil {
					tmpls, err := tmplStore.List()
					if err == nil && len(tmpls) > 0 {
						return OpenModalMsg{M: newCommentWithTemplateModal(tmpls, commentTitle, onConfirm)}
					}
				}
				return OpenModalMsg{M: newCommentInputModal(commentTitle, "", onConfirm)}
			}},
			CommandEntry{Key: "l", Desc: "log work", Cmd: func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message: fmt.Sprintf("Log work for %s?", key),
					onConfirm: func() tea.Msg {
						return OpenModalMsg{M: NewNumericHoursModal(
							fmt.Sprintf("Hours to log for %s", key),
							func(hours int) tea.Cmd {
								return logWorkBoardCmd(jr, key, 8, 0, 8+hours, 0)
							},
						)}
					},
				}}
			}},
		)
	}

	if info, ok := s.mrStatuses[issue.Key]; ok {
		if info.webURL != "" {
			url := info.webURL
			entries = append(entries, CommandEntry{Key: "r", Desc: "open MR", Cmd: openBrowserCmd(s.shell, url)})
		}
		if info.isMine && info.projectID != 0 {
			pID, mrIID := info.projectID, info.mrIID
			glab := s.gitlab
			entries = append(entries, CommandEntry{Key: "m", Desc: "merge MR", Cmd: func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   fmt.Sprintf("Merge MR for %s?", key),
					onConfirm: mergeMRCmd(glab, pID, mrIID, key),
				}}
			}})
		}
	}

	if s.jiraBaseURL != "" {
		url := strings.TrimRight(s.jiraBaseURL, "/") + "/browse/" + issue.Key
		entries = append(entries, CommandEntry{Key: "w", Desc: "open jira", Cmd: openBrowserCmd(s.shell, url)})
	}

	if repoPath, ok := s.repoPaths[issue.Key]; ok && repoPath != "" {
		entries = append(entries, CommandEntry{Key: "n", Desc: "navigate", Cmd: navigateCmd(s.git, repoPath)})
	}

	if len(s.repos) > 0 {
		repos := s.repos
		branchName := integration.BuildBranchName(issue)
		git := s.git
		fs := s.fs
		shell := s.shell
		issueCopy := issue
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.Name
		}
		entries = append(entries, CommandEntry{Key: "b", Desc: "create branch", Cmd: func() tea.Msg {
			return OpenModalMsg{M: ListSelectModal{
				title: fmt.Sprintf("Create branch %s in:", branchName),
				items: names,
				onSelect: func(idx int) tea.Cmd {
					return checkoutNewBranchCmd(git, repos[idx], branchName, issueCopy, fs, shell)
				},
			}}
		}})
	}

	return entries
}

// statusGroup maps a Jira status name to its display group.
var statusGroup = map[string]string{
	"In Backlog":  "Analysis",
	"Backlog":     "Analysis",
	"Open":        "Analysis",
	"On Hold":     "Analysis",
	"Prio 1":      "To Dev",
	"To Do":       "To Dev",
	"To Dev":      "To Dev",
	"In Progress": "In Progress",
	"Review":      "In Progress",
	"TEST ENV":    "Testing",
	"PREPROD ENV": "Testing",
	"Testing":     "Testing",
	"Done":        "Done",
	"Closed":      "Done",
}

// boardGroups is the ordered list of group names for the board display.
var boardGroups = []string{"Analysis", "To Dev", "In Progress", "Testing", "Done"}

func (s BoardSection) view(width, height int) string {
	issues := s.displayIssues()

	if s.loading && len(s.allIssues) == 0 {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading sprint issues...")
	}

	if len(issues) == 0 {
		msg := "No issues in the active sprint."
		if s.filtered {
			msg = "No issues match the filter. Press f to show all."
		}
		return lipgloss.NewStyle().Padding(1, 2).Foreground(colorMuted).Render(msg)
	}

	// Build grouped map: group name → list of (flat index, issue)
	type groupItem struct {
		flatIdx int
		issue   service.Issue
	}
	grouped := make(map[string][]groupItem, len(boardGroups))
	for i, iss := range issues {
		g, ok := statusGroup[iss.Status]
		if !ok {
			g = "Analysis"
		}
		grouped[g] = append(grouped[g], groupItem{flatIdx: i, issue: iss})
	}

	// Build a flat list of display lines and track which flat index each line maps to (-1 = header)
	type displayLine struct {
		text    string
		flatIdx int // -1 for group headers
	}
	var displayLines []displayLine

	groupHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(colorHeader)

	const (
		colKey    = 14
		colStatus = 16
		colInit   = 4
		colBranch = 22
	)
	summaryW := width - colKey - colStatus - colInit - colBranch - 8
	if summaryW < 10 {
		summaryW = 10
	}

	for _, grp := range boardGroups {
		items, ok := grouped[grp]
		if !ok || len(items) == 0 {
			continue
		}
		// Bold separator line with group name
		sep := strings.Repeat("─", 4)
		headerLine := groupHeaderStyle.Render(fmt.Sprintf("%s %s %s", sep, grp, sep))
		displayLines = append(displayLines, displayLine{text: headerLine, flatIdx: -1})

		for _, item := range items {
			iss := item.issue
			initials := service.AssigneeInitials(iss.Assignee)
			if initials == "" {
				initials = "--"
			}
			branch := ""
			if name, ok := s.branchNames[iss.Key]; ok {
				branch = name
			}
			row := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
				colKey, truncStr(iss.Key, colKey),
				colStatus, truncStr(iss.Status, colStatus),
				colInit, truncStr(initials, colInit),
				colBranch, truncStr(branch, colBranch),
				truncStr(iss.Summary, summaryW),
			)
			displayLines = append(displayLines, displayLine{text: row, flatIdx: item.flatIdx})
		}
	}

	// Column header row (shown once at the top)
	summaryHeader := "SUMMARY"
	if s.filtered {
		summaryHeader = "SUMMARY " + lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render("[FILTERED]")
	} else {
		summaryHeader = "SUMMARY " + lipgloss.NewStyle().Foreground(colorMuted).Render("[f: filter]")
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render(
		fmt.Sprintf("  %-*s %-*s %-*s %-*s ",
			colKey, "KEY",
			colStatus, "STATUS",
			colInit, "WHO",
			colBranch, "BRANCH",
		),
	) + summaryHeader
	displayLines = append([]displayLine{
		{text: header, flatIdx: -1},
		{text: tableHeaderSepLine(width), flatIdx: -1},
	}, displayLines...)

	// Find the display row that corresponds to s.cursor (selected issue)
	cursorDisplayRow := 0
	for i, dl := range displayLines {
		if dl.flatIdx == s.cursor {
			cursorDisplayRow = i
			break
		}
	}

	// Sliding window: centre the selected row in the visible area
	maxVisible := height - 4 // reserve 1 row for bottom sep, 1 for legend bar, 2 for implicit padding
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := cursorDisplayRow - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(displayLines) {
		end = len(displayLines)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	var lines []string
	for i := start; i < end; i++ {
		dl := displayLines[i]
		if dl.flatIdx == -1 {
			// Group header — not selectable
			lines = append(lines, dl.text)
			continue
		}
		if dl.flatIdx == s.cursor {
			iss := issues[dl.flatIdx]
			color := IssueColor(iss, s.myUserKey, s.localBranches, s.mrStatuses)
			lines = append(lines, selectedRowStyle.Foreground(color).Width(width).Render(dl.text))
		} else {
			iss := issues[dl.flatIdx]
			color := IssueColor(iss, s.myUserKey, s.localBranches, s.mrStatuses)
			lines = append(lines, normalRowStyle.Foreground(color).Width(width).Render(dl.text))
		}
	}

	view := strings.Join(lines, "\n")
	view += "\n" + tableHeaderSepLine(width)
	view += "\n" + IssueColorLegendBar(width)
	view += "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render("  enter: description  i: in progress  m: merge MR  b: create branch  →: commands")
	if s.statusMsg != "" {
		view += "\n" + lipgloss.NewStyle().Foreground(colorGreen).Render("  "+s.statusMsg)
	}
	return view
}

func (s BoardSection) helpKeys() []HelpEntry { return BoardKeys }
