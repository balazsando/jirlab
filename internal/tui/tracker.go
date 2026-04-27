package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// worklogsLoadedMsg is delivered when worklog fetching completes.
type worklogsLoadedMsg struct {
	logs []service.TimeLog
	date time.Time // which date was fetched, for staleness check
	err  error
}

// worklogAddedMsg is delivered after a worklog is posted.
type worklogAddedMsg struct {
	message string
	date    time.Time
}

// fetchWorklogsCmd loads worklogs for the given date.
func fetchWorklogsCmd(jira integration.JiraService, accountID string, date time.Time) tea.Cmd {
	return func() tea.Msg {
		logs, err := jira.GetWorklogsForDate(accountID, date)
		if err != nil {
			return worklogsLoadedMsg{date: date, err: err}
		}
		return worklogsLoadedMsg{logs: logs, date: date}
	}
}

// trackerLogWorkCmd logs time for the given issue on today's date.
func trackerLogWorkCmd(jira integration.JiraService, issueKey string, fromH, fromM, toH, toM int) tea.Cmd {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return func() tea.Msg {
		if issueKey == "" {
			return errMsg{source: "tracker", err: fmt.Errorf("issue key is required")}
		}
		from := time.Date(now.Year(), now.Month(), now.Day(), fromH, fromM, 0, 0, now.Location())
		to := time.Date(now.Year(), now.Month(), now.Day(), toH, toM, 0, 0, now.Location())
		if err := jira.LogWork(issueKey, from, to); err != nil {
			return errMsg{source: "tracker", err: err}
		}
		hours := to.Sub(from).Hours()
		return worklogAddedMsg{
			message: fmt.Sprintf("Logged %.0fh for %s", hours, issueKey),
			date:    today,
		}
	}
}

// ---------------------------------------------------------------------------
// Tracker sub-pane enum
// ---------------------------------------------------------------------------

type trackerPane int

const (
	trackerPaneWorklogs trackerPane = iota
	trackerPaneNotes
)

// TrackerSection is the Time Tracker tab.
type TrackerSection struct {
	logs        []service.TimeLog
	cursor      int
	totalHours  float64
	statusMsg   string
	currentDate time.Time
	loading     bool
	jira        integration.JiraService
	accountID   string
	shell       integration.ShellService

	// Notes sub-pane
	activePane    trackerPane
	notes         []note
	notesFiltered []note // sorted+filtered view
	notesCursor   int
	notesSort     notesSortMode
	notesFilter   string // "" = all categories
	notesPath     string
}

func newTrackerSection(jira integration.JiraService, accountID string, shell integration.ShellService) TrackerSection {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	path := notesFilePath()
	notes := loadNotes(path)
	s := TrackerSection{
		currentDate: today,
		loading:     jira != nil,
		jira:        jira,
		accountID:   accountID,
		shell:       shell,
		notes:       notes,
		notesPath:   path,
	}
	s.rebuildNotesView()
	return s
}

// rebuildNotesView applies sort + filter to produce s.notesFiltered.
func (s *TrackerSection) rebuildNotesView() {
	filtered := filterNotes(s.notes, s.notesFilter)
	s.notesFiltered = sortNotes(filtered, s.notesSort)
	if s.notesCursor >= len(s.notesFiltered) {
		s.notesCursor = max(0, len(s.notesFiltered)-1)
	}
}

func (s TrackerSection) dayLabel() string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	switch {
	case s.currentDate.Equal(today):
		return "Today"
	case s.currentDate.Equal(yesterday):
		return "Yesterday"
	default:
		return s.currentDate.Format("Mon, Jan 2 2006")
	}
}

func (s TrackerSection) update(msg tea.Msg) (TrackerSection, tea.Cmd) {
	switch msg := msg.(type) {
	case worklogsLoadedMsg:
		// Only accept results for the currently displayed date.
		if msg.date.Equal(s.currentDate) {
			s.loading = false
			if msg.err != nil {
				return s, func() tea.Msg { return errMsg{source: "tracker", err: msg.err} }
			}
			s.logs = msg.logs
			s.cursor = 0
			s.totalHours = 0
			for _, l := range s.logs {
				s.totalHours += l.Hours
			}
		}

	case worklogAddedMsg:
		s.statusMsg = msg.message
		// Reload worklogs for the relevant date if it's the current one
		if msg.date.Equal(s.currentDate) && s.jira != nil {
			s.loading = true
			return s, fetchWorklogsCmd(s.jira, s.accountID, s.currentDate)
		}

	case notesSavedMsg:
		// no-op: save is fire-and-forget

	case notesUpdatedMsg:
		s.notes = msg.notes
		s.rebuildNotesView()

	case noteDeleteMsg:
		s.notes = deleteNote(s.notes, msg.noteIdx)
		s.rebuildNotesView()
		return s, saveNotesCmd(s.notesPath, s.notes)

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

// deleteNote removes the note at originalIdx from the slice.
func deleteNote(notes []note, originalIdx int) []note {
	if originalIdx < 0 || originalIdx >= len(notes) {
		return notes
	}
	out := make([]note, 0, len(notes)-1)
	out = append(out, notes[:originalIdx]...)
	out = append(out, notes[originalIdx+1:]...)
	return out
}

func (s TrackerSection) handleKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
	// tab switches between worklogs and notes
	if msg.String() == "tab" {
		if s.activePane == trackerPaneWorklogs {
			s.activePane = trackerPaneNotes
		} else {
			s.activePane = trackerPaneWorklogs
		}
		return s, nil
	}

	if s.activePane == trackerPaneNotes {
		return s.handleNotesKey(msg)
	}
	return s.handleWorklogsKey(msg)
}

func (s TrackerSection) handleWorklogsKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
	n := len(s.logs)
	switch msg.String() {
	case "j", "down":
		if s.cursor < n-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "left":
		s.currentDate = s.currentDate.AddDate(0, 0, -1)
		s.logs = nil
		s.totalHours = 0
		s.cursor = 0
		if s.jira != nil {
			s.loading = true
			return s, fetchWorklogsCmd(s.jira, s.accountID, s.currentDate)
		}
	case "right":
		s.currentDate = s.currentDate.AddDate(0, 0, 1)
		s.logs = nil
		s.totalHours = 0
		s.cursor = 0
		if s.jira != nil {
			s.loading = true
			return s, fetchWorklogsCmd(s.jira, s.accountID, s.currentDate)
		}
	case "l": // log full day 8:00-16:00
		if s.jira != nil {
			jira := s.jira
			return s, func() tea.Msg {
				return OpenModalMsg{M: NewInputModal(
					"Log Full Day (8h)",
					"Issue key (e.g. PROJ-123)",
					func(issueKey string) tea.Cmd {
						return trackerLogWorkCmd(jira, issueKey, 8, 0, 16, 0)
					},
				)}
			}
		}
	case "h": // log half day 8:00-12:00
		if s.jira != nil {
			jira := s.jira
			return s, func() tea.Msg {
				return OpenModalMsg{M: NewInputModal(
					"Log Half Day (4h)",
					"Issue key (e.g. PROJ-123)",
					func(issueKey string) tea.Cmd {
						return trackerLogWorkCmd(jira, issueKey, 8, 0, 12, 0)
					},
				)}
			}
		}
	case "w": // open Jira timetracker in browser
		if s.jira != nil {
			baseURL := s.jira.GetBaseURL()
			ttURL := baseURL + "/plugins/servlet/ac/org.everit.jira.timetracker.plugin/timetracker-page#!/calendar"
			return s, openBrowserCmd(s.shell, ttURL)
		}
	}
	return s, nil
}

func (s TrackerSection) handleNotesKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if s.notesCursor < len(s.notesFiltered)-1 {
			s.notesCursor++
		}
	case "k", "up":
		if s.notesCursor > 0 {
			s.notesCursor--
		}

	case "n": // new note
		cats := uniqueCategories(s.notes)
		currentNotes := s.notes
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewNoteCreateModal(cats, func(newNote note) tea.Cmd {
				return func() tea.Msg {
					return notesUpdatedMsg{notes: append(currentNotes, newNote)}
				}
			})}
		}

	case "enter": // open note detail
		if len(s.notesFiltered) == 0 || s.notesCursor >= len(s.notesFiltered) {
			break
		}
		sel := s.notesFiltered[s.notesCursor]
		// find original index in s.notes
		origIdx := -1
		for i, n := range s.notes {
			if n.ID == sel.ID {
				origIdx = i
				break
			}
		}
		if origIdx < 0 {
			break
		}
		notesSlice := s.notes
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewNoteDetailModal(origIdx, sel, func(updated note) tea.Cmd {
				return func() tea.Msg {
					newNotes := make([]note, len(notesSlice))
					copy(newNotes, notesSlice)
					newNotes[origIdx] = updated
					return notesUpdatedMsg{notes: newNotes}
				}
			})}
		}

	case "s": // cycle sort
		s.notesSort = (s.notesSort + 1) % 3
		s.rebuildNotesView()

	case "f": // cycle category filter
		cats := uniqueCategories(s.notes)
		if len(cats) == 0 {
			break
		}
		if s.notesFilter == "" {
			s.notesFilter = cats[0]
		} else {
			found := false
			for i, c := range cats {
				if c == s.notesFilter {
					if i+1 < len(cats) {
						s.notesFilter = cats[i+1]
					} else {
						s.notesFilter = "" // back to all
					}
					found = true
					break
				}
			}
			if !found {
				s.notesFilter = ""
			}
		}
		s.notesCursor = 0
		s.rebuildNotesView()

	case "d": // delete selected note
		if len(s.notesFiltered) == 0 || s.notesCursor >= len(s.notesFiltered) {
			break
		}
		sel := s.notesFiltered[s.notesCursor]
		origIdx := -1
		for i, n := range s.notes {
			if n.ID == sel.ID {
				origIdx = i
				break
			}
		}
		if origIdx < 0 {
			break
		}
		noteTitle := sel.Title
		notesSlice := s.notes
		return s, func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Delete note: %s?", truncStr(noteTitle, 40)),
				onConfirm: func() tea.Msg {
					newNotes := deleteNote(notesSlice, origIdx)
					return notesUpdatedMsg{notes: newNotes}
				},
			}}
		}
	}
	return s, nil
}

func (s TrackerSection) view(width, height int) string {
	// Vertical split: worklogs top, notes bottom.
	// Overhead: sep(1) + paneBar(1) + paneSep(1) = 3 rows between sections.
	topH := height * 55 / 100
	if topH < 4 {
		topH = 4
	}
	botH := height - topH - 3
	if botH < 5 {
		botH = 5
	}

	worklogsFocused := s.activePane == trackerPaneWorklogs
	notesFocused := s.activePane == trackerPaneNotes

	top := s.viewWorklogs(width, topH, worklogsFocused)
	sep := sectionSepLine(width)
	paneBar := s.viewNotesPaneBar(width, notesFocused)
	paneSep := sectionSepLine(width)
	bot := s.viewNotes(width, botH, notesFocused)

	return strings.Join([]string{top, sep, paneBar, paneSep, bot}, "\n")
}

// viewNotesPaneBar renders the Notes sub-section tab bar, matching the design of
// the Pods/Services/Deployments and Branches/Tags pane bars in other sections.
func (s TrackerSection) viewNotesPaneBar(width int, focused bool) string {
	label := s.notesHeader()
	if focused {
		return tabActiveStyle.Render(label)
	}
	return tabDimStyle.Render(label)
}

// notesHeader builds the Notes section label including count, filter, and sort info.
func (s TrackerSection) notesHeader() string {
	label := fmt.Sprintf("Notes (%d)", len(s.notes))
	if s.notesFilter != "" {
		label += " [" + s.notesFilter + "]"
	}
	if s.notesSort != notesSortDate {
		label += " [" + s.notesSort.Label() + "]"
	}
	return label
}

func (s TrackerSection) viewWorklogs(width, height int, focused bool) string {
	hoursStyle := lipgloss.NewStyle().Bold(true)
	if s.totalHours >= 8 {
		hoursStyle = hoursStyle.Foreground(colorGreen)
	} else if s.totalHours > 0 {
		hoursStyle = hoursStyle.Foreground(colorYellow)
	} else {
		hoursStyle = hoursStyle.Foreground(colorRed)
	}

	selRow := selectedRowDimStyle
	if focused {
		selRow = selectedRowStyle
	}

	nav := lipgloss.NewStyle().Foreground(colorSubtle).Render("  ← prev  → next  w: timetracker")

	if s.loading && len(s.logs) == 0 {
		header := lipgloss.NewStyle().Bold(true).Padding(0, 2).Render(s.dayLabel())
		return header + "   " + nav + "\n\n" +
			lipgloss.NewStyle().Padding(0, 2).Render("Loading worklogs...")
	}

	dayStr := s.dayLabel()
	summary := fmt.Sprintf("  %s: %s logged", dayStr,
		hoursStyle.Render(fmt.Sprintf("%.1fh", s.totalHours)))
	header := summary + "   " + nav

	if len(s.logs) == 0 {
		noData := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).Render("No time logged for this day.")
		return header + "\n\n" + noData
	}

	timeW := 13 // "09:00-11:30 "
	issueW := 14
	commentW := 30
	summaryW := width - timeW - issueW - commentW - 6
	if summaryW < 10 {
		summaryW = 10
	}

	colHeader := func(str string, w int) string {
		return lipgloss.NewStyle().Bold(true).Foreground(colorBlue).
			Width(w).Render(truncStr(str, w))
	}
	tableHeader := colHeader("TIME", timeW) + " " + colHeader("ISSUE", issueW) + " " +
		colHeader("SUMMARY", summaryW) + " " + colHeader("COMMENT", commentW)
	sepLine := lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("-", width))

	maxRows := height - 7
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if s.cursor >= maxRows {
		start = s.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(s.logs) {
		end = len(s.logs)
	}

	var lines []string
	lines = append(lines, header, "", tableHeader, sepLine)

	for i := start; i < end; i++ {
		entry := s.logs[i]
		fromTo := fmt.Sprintf("%s-%s",
			entry.From.Format("15:04"),
			entry.To.Format("15:04"),
		)
		row := fmt.Sprintf("%-*s %-*s %-*s %-*s",
			timeW, truncStr(fromTo, timeW),
			issueW, truncStr(entry.IssueKey, issueW),
			summaryW, truncStr(entry.Description, summaryW),
			commentW, truncStr(entry.Comment, commentW),
		)
		if i == s.cursor {
			lines = append(lines, selRow.Width(width).Render(row))
		} else {
			lines = append(lines, normalRowStyle.Width(width).Render(row))
		}
	}

	result := strings.Join(lines, "\n")
	result += "\n" + tableHeaderSepLine(width)
	result += "\n" + TimeLogColorLegendBar(width)
	result += "\n" + lipgloss.NewStyle().Foreground(colorSubtle).Render("  l: log full day  h: log half day  ←/→: prev/next day  w: timetracker  tab: notes")
	if s.statusMsg != "" {
		result += "\n" + lipgloss.NewStyle().Foreground(colorGreen).Render("  "+s.statusMsg)
	}
	return result
}

func (s TrackerSection) viewNotes(width, height int, focused bool) string {
	selRow := selectedRowDimStyle
	if focused {
		selRow = selectedRowStyle
	}

	if len(s.notesFiltered) == 0 {
		empty := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).Render("No notes. Press n to create one.")
		return empty
	}

	titleW := width / 3
	catW := 14
	prioW := 6
	tasksW := 8
	dateW := 12
	if titleW+catW+prioW+tasksW+dateW+8 > width {
		titleW = width - catW - prioW - tasksW - dateW - 8
	}
	if titleW < 12 {
		titleW = 12
	}

	colH := func(s string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(s, w)) }
	tableHeader := colH("TITLE", titleW) + " " + colH("CATEGORY", catW) + " " +
		colH("PRIO", prioW) + " " + colH("TASKS", tasksW) + " " + colH("CREATED", dateW)

	// rows = total height minus: col header + table sep + legend + hints
	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.notesCursor, len(s.notesFiltered), maxRows)

	var rows []string
	rows = append(rows, tableHeader, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		nn := s.notesFiltered[i]
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
			titleW, truncStr(nn.Title, titleW),
			catW, truncStr(nn.Category, catW),
			prioW, truncStr(nn.Priority.String(), prioW),
			tasksW, truncStr(nn.taskSummary(), tasksW),
			dateW, truncStr(nn.CreatedAt.Format("2006-01-02"), dateW),
		)
		col := nn.Priority.Color()
		if i == s.notesCursor {
			rows = append(rows, selRow.Foreground(col).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(col).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, NotePriorityLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorSubtle).Render(
		"  n: new  enter: open  d: delete  s: sort  f: filter  tab: worklogs",
	))
	return strings.Join(rows, "\n")
}

func (s TrackerSection) helpKeys() []HelpEntry {
	if s.activePane == trackerPaneNotes {
		return TrackerNotesKeys
	}
	return TrackerKeys
}
