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
}

func newTrackerSection(jira integration.JiraService, accountID string, shell integration.ShellService) TrackerSection {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return TrackerSection{
		currentDate: today,
		loading:     jira != nil,
		jira:        jira,
		accountID:   accountID,
		shell:       shell,
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

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

func (s TrackerSection) handleKey(msg tea.KeyMsg) (TrackerSection, tea.Cmd) {
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

func (s TrackerSection) view(width, height int) string {
	hoursStyle := lipgloss.NewStyle().Bold(true)
	if s.totalHours >= 8 {
		hoursStyle = hoursStyle.Foreground(colorGreen)
	} else if s.totalHours > 0 {
		hoursStyle = hoursStyle.Foreground(colorYellow)
	} else {
		hoursStyle = hoursStyle.Foreground(colorRed)
	}

	nav := lipgloss.NewStyle().Foreground(colorSubtle).Render("  ← prev  → next  r: refresh  w: timetracker")

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
		note := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).Render("No time logged for this day.")
		return header + "\n\n" + note
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

	maxRows := height - 8
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
		log := s.logs[i]
		fromTo := fmt.Sprintf("%s-%s",
			log.From.Format("15:04"),
			log.To.Format("15:04"),
		)
		row := fmt.Sprintf("%-*s %-*s %-*s %-*s",
			timeW, truncStr(fromTo, timeW),
			issueW, truncStr(log.IssueKey, issueW),
			summaryW, truncStr(log.Description, summaryW),
			commentW, truncStr(log.Comment, commentW),
		)
		if i == s.cursor {
			lines = append(lines, selectedRowStyle.Width(width).Render(row))
		} else {
			lines = append(lines, normalRowStyle.Width(width).Render(row))
		}
	}

	result := strings.Join(lines, "\n")
	result += "\n" + tableHeaderSepLine(width)
	result += "\n" + TimeLogColorLegendBar(width)
	result += "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render("  l: log full day  h: log half day  ←/→: prev/next day  w: timetracker")
	if s.statusMsg != "" {
		result += "\n" + lipgloss.NewStyle().Foreground(colorGreen).Render("  "+s.statusMsg)
	}
	return result
}

func (s TrackerSection) helpKeys() []HelpEntry { return TrackerKeys }
