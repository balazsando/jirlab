package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/notes"
	"github.com/andob/jirlab/internal/service"
	"github.com/andob/jirlab/internal/templates"
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

// hasLogsInRange checks if any worklog in the slice overlaps with the given time range on the given date.
func hasLogsInRange(logs []service.TimeLog, date time.Time, startH, endH int) bool {
	start := time.Date(date.Year(), date.Month(), date.Day(), startH, 0, 0, 0, date.Location())
	end := time.Date(date.Year(), date.Month(), date.Day(), endH, 0, 0, 0, date.Location())
	for _, log := range logs {
		// Check if log overlaps with [start, end)
		if log.From.Before(end) && log.To.After(start) {
			return true
		}
	}
	return false
}

// LatestLogEndHour returns the end hour of the latest worklog on date.
// Returns 8 (default start-of-day) when no logs exist for the given date.
// Exported so acceptance tests can verify the calculation directly.
func LatestLogEndHour(logs []service.TimeLog, date time.Time) int {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	latestEnd := 0
	for _, log := range logs {
		if log.To.Before(dayStart) || log.To.After(dayEnd) {
			continue
		}
		if log.To.Hour() > latestEnd {
			latestEnd = log.To.Hour()
		}
	}
	if latestEnd == 0 {
		return 8 // default: work starts at 8
	}
	return latestEnd
}

// NumericInputHandleKey processes a single keypress for the hours input widget.
// Only digit characters are accepted; each digit overwrites the current value.
// Non-digit input is ignored and the current value is returned unchanged.
// Exported so acceptance tests can verify the input behaviour directly.
func NumericInputHandleKey(current, key string) string {
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		return key
	}
	return current
}

// ---------------------------------------------------------------------------
// Tracker sub-pane enum
// ---------------------------------------------------------------------------

type trackerPane int

const (
	trackerPaneWorklogs  trackerPane = iota
	trackerPaneNotes
	trackerPaneTemplates
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
	notes         []notes.Note
	notesFiltered []notes.Note // sorted+filtered view
	notesCursor   int
	notesSort     notesSortMode
	notesFilter   string // "" = all categories
	notesStore    notes.Store

	// Templates sub-pane
	templates       []templates.Template
	templatesCursor int
	templatesStore  templates.Store
}

func newTrackerSection(jira integration.JiraService, accountID string, shell integration.ShellService) TrackerSection {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	store, _ := notes.DefaultStore()
	if store == nil {
		store = notes.NewFilesystemStore("/tmp/jirlab-notes")
	}
	// One-shot migration from legacy ~/.jirlab/notes.json
	_ = notes.MigrateFromJSON(store, notes.LegacyJSONPath())
	notesList, _ := store.List()

	tmplStore, _ := templates.DefaultStore()
	if tmplStore == nil {
		tmplStore = templates.NewFilesystemStore(os.TempDir() + "/jirlab-templates")
	}
	tmplList, _ := tmplStore.List()

	s := TrackerSection{
		currentDate:    today,
		loading:        jira != nil,
		jira:           jira,
		accountID:      accountID,
		shell:          shell,
		notes:          notesList,
		notesStore:     store,
		templates:      tmplList,
		templatesStore: tmplStore,
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

	case noteSavedMsg:
		if msg.isNew {
			s.notes = append(s.notes, msg.note)
		} else {
			for i, n := range s.notes {
				if n.ID == msg.note.ID {
					s.notes[i] = msg.note
					break
				}
			}
		}
		s.rebuildNotesView()
		return s, notePersistCmd(s.notesStore, msg.note)

	case noteRemovedMsg:
		s.notes = removeNoteByID(s.notes, msg.id)
		s.rebuildNotesView()
		return s, noteDeleteFileCmd(s.notesStore, msg.id)

	case templateSavedMsg:
		if msg.isNew {
			s.templates = append(s.templates, msg.tmpl)
		} else {
			for i, t := range s.templates {
				if t.ID == msg.tmpl.ID {
					s.templates[i] = msg.tmpl
					break
				}
			}
		}
		return s, templatePersistCmd(s.templatesStore, msg.tmpl)

	case templateRemovedMsg:
		s.templates = removeTemplateByID(s.templates, msg.id)
		if s.templatesCursor >= len(s.templates) {
			s.templatesCursor = max(0, len(s.templates)-1)
		}
		return s, templateDeleteFileCmd(s.templatesStore, msg.id)

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

// removeNoteByID removes the note with the given ID from the slice.
func removeNoteByID(ns []notes.Note, id string) []notes.Note {
	out := make([]notes.Note, 0, len(ns))
	for _, n := range ns {
		if n.ID != id {
			out = append(out, n)
		}
	}
	return out
}

// removeTemplateByID removes the template with the given ID from the slice.
func removeTemplateByID(ts []templates.Template, id string) []templates.Template {
	out := make([]templates.Template, 0, len(ts))
	for _, t := range ts {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

func (s TrackerSection) handleKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
	// tab cycles through all three panes
	if msg.String() == "tab" {
		s.activePane = (s.activePane + 1) % 3
		return s, nil
	}

	// direct pane-jump hotkeys available from any pane
	switch msg.String() {
	case "o":
		s.activePane = trackerPaneNotes
		return s, nil
	case "t":
		s.activePane = trackerPaneTemplates
		return s, nil
	}

	switch s.activePane {
	case trackerPaneNotes:
		return s.handleNotesKey(msg)
	case trackerPaneTemplates:
		return s.handleTemplatesKey(msg)
	default:
		return s.handleWorklogsKey(msg)
	}
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
	case "l": // log work — ask issue key first, then hours
		if s.jira != nil {
			jira := s.jira
			logs := s.logs
			currentDate := s.currentDate
			return s, func() tea.Msg {
				startH := LatestLogEndHour(logs, currentDate)
				return OpenModalMsg{M: NewInputModal(
					fmt.Sprintf("Log Work (start %d:00) — Issue Key", startH),
					"e.g. PROJ-123",
					func(issueKey string) tea.Cmd {
						return func() tea.Msg {
							title := fmt.Sprintf("Log Work for %s (start %d:00) — Hours", issueKey, startH)
							return OpenModalMsg{M: NewNumericHoursModal(title, func(hours int) tea.Cmd {
								endH := startH + hours
								if endH > 16 {
									endH = 16
								}
								return trackerLogWorkCmd(jira, issueKey, startH, 0, endH, 0)
							})}
						}
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
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewNoteCreateModal(cats, func(newNote notes.Note) tea.Cmd {
				return func() tea.Msg {
					return noteSavedMsg{note: newNote, isNew: true}
				}
			})}
		}

	case "enter": // open note detail
		if len(s.notesFiltered) == 0 || s.notesCursor >= len(s.notesFiltered) {
			break
		}
		sel := s.notesFiltered[s.notesCursor]
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewNoteDetailModal(sel, func(updated notes.Note) tea.Cmd {
				return func() tea.Msg {
					return noteSavedMsg{note: updated, isNew: false}
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
		noteTitle := sel.Title
		noteID := sel.ID
		return s, func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Delete note: %s?", truncStr(noteTitle, 40)),
				onConfirm: func() tea.Msg {
					return noteRemovedMsg{id: noteID}
				},
			}}
		}
	}
	return s, nil
}

func (s TrackerSection) handleTemplatesKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if s.templatesCursor < len(s.templates)-1 {
			s.templatesCursor++
		}
	case "k", "up":
		if s.templatesCursor > 0 {
			s.templatesCursor--
		}

	case "n": // new template
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewTemplateCreateModal(func(t templates.Template) tea.Cmd {
				return func() tea.Msg {
					return templateSavedMsg{tmpl: t, isNew: true}
				}
			})}
		}

	case "enter": // edit selected template
		if len(s.templates) == 0 || s.templatesCursor >= len(s.templates) {
			break
		}
		sel := s.templates[s.templatesCursor]
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewTemplateEditModal(sel, func(t templates.Template) tea.Cmd {
				return func() tea.Msg {
					return templateSavedMsg{tmpl: t, isNew: false}
				}
			})}
		}

	case "d": // delete selected template
		if len(s.templates) == 0 || s.templatesCursor >= len(s.templates) {
			break
		}
		sel := s.templates[s.templatesCursor]
		tmplName := sel.Name
		tmplID := sel.ID
		return s, func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Delete template: %s?", truncStr(tmplName, 40)),
				onConfirm: func() tea.Msg {
					return templateRemovedMsg{id: tmplID}
				},
			}}
		}
	}
	return s, nil
}

func (s TrackerSection) view(width, height int) string {
	// Vertical split: worklogs top, bottom pane below.
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
	templatesFocused := s.activePane == trackerPaneTemplates

	top := s.viewWorklogs(width, topH, worklogsFocused)
	sep := sectionSepLine(width)
	paneBar := s.viewBottomPaneBar(width)
	paneSep := sectionSepLine(width)

	var bot string
	switch s.activePane {
	case trackerPaneTemplates:
		bot = s.viewTemplates(width, botH, templatesFocused)
	default:
		bot = s.viewNotes(width, botH, notesFocused)
	}

	return strings.Join([]string{top, sep, paneBar, paneSep, bot}, "\n")
}

// viewBottomPaneBar renders a tab bar for the two bottom sub-panes (Notes and Templates).
func (s TrackerSection) viewBottomPaneBar(width int) string {
	noteLabel := s.notesHeader()
	tmplLabel := fmt.Sprintf("Templates (%d)", len(s.templates))

	notesTab := tabDimStyle.Render("[o] " + noteLabel)
	tmplTab := tabDimStyle.Render("[t] " + tmplLabel)

	switch s.activePane {
	case trackerPaneNotes:
		notesTab = tabActiveStyle.Render("[o] " + noteLabel)
	case trackerPaneTemplates:
		tmplTab = tabActiveStyle.Render("[t] " + tmplLabel)
	}

	return notesTab + "  " + tmplTab
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
	result += "\n" + lipgloss.NewStyle().Foreground(colorSubtle).Render("  l: log full day  h: log half day  ←/→: prev/next day  w: timetracker  o: notes  t: templates")
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
			tasksW, truncStr(noteTaskSummary(nn), tasksW),
			dateW, truncStr(nn.CreatedAt.Format("2006-01-02"), dateW),
		)
		col := notePriorityColor(nn.Priority)
		if i == s.notesCursor {
			rows = append(rows, selRow.Foreground(col).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(col).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, NotePriorityLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorSubtle).Render(
		"  n: new  enter: open  d: delete  s: sort  f: filter  tab: cycle panes",
	))
	return strings.Join(rows, "\n")
}

func (s TrackerSection) viewTemplates(width, height int, focused bool) string {
	selRow := selectedRowDimStyle
	if focused {
		selRow = selectedRowStyle
	}

	if len(s.templates) == 0 {
		empty := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).Render("No templates. Press n to create one.")
		return empty
	}

	nameW := width / 3
	previewW := width - nameW - 14 - 4
	dateW := 12
	if previewW < 10 {
		previewW = 10
	}
	if nameW < 12 {
		nameW = 12
	}

	colH := func(s string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(s, w)) }
	tableHeader := colH("NAME", nameW) + " " + colH("BODY PREVIEW", previewW) + " " + colH("CREATED", dateW)

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.templatesCursor, len(s.templates), maxRows)

	var rows []string
	rows = append(rows, tableHeader, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		tmpl := s.templates[i]
		preview := strings.ReplaceAll(tmpl.Body, "\n", " ")
		line := fmt.Sprintf("%-*s %-*s %-*s",
			nameW, truncStr(tmpl.Name, nameW),
			previewW, truncStr(preview, previewW),
			dateW, truncStr(tmpl.CreatedAt.Format("2006-01-02"), dateW),
		)
		if i == s.templatesCursor {
			rows = append(rows, selRow.Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorSubtle).Render(
		"  n: new  enter: edit  d: delete  tab: cycle panes",
	))
	return strings.Join(rows, "\n")
}

func (s TrackerSection) helpKeys() []HelpEntry {
	switch s.activePane {
	case trackerPaneNotes:
		return TrackerNotesKeys
	case trackerPaneTemplates:
		return TrackerTemplatesKeys
	default:
		return TrackerKeys
	}
}
