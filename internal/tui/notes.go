package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Note data model
// ---------------------------------------------------------------------------

type notePriority int

const (
	notePriorityLow notePriority = iota
	notePriorityMedium
	notePriorityHigh
)

func (p notePriority) String() string {
	switch p {
	case notePriorityHigh:
		return "High"
	case notePriorityMedium:
		return "Med"
	default:
		return "Low"
	}
}

func (p notePriority) Color() lipgloss.Color {
	switch p {
	case notePriorityHigh:
		return colorRed
	case notePriorityMedium:
		return colorYellow
	default:
		return colorGreen
	}
}

type noteTask struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type note struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Category    string       `json:"category"`
	Priority    notePriority `json:"priority"`
	Description string       `json:"description"`
	Tasks       []noteTask   `json:"tasks"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

func (n note) remainingTasks() int {
	count := 0
	for _, t := range n.Tasks {
		if !t.Done {
			count++
		}
	}
	return count
}

func (n note) taskSummary() string {
	if len(n.Tasks) == 0 {
		return "-"
	}
	done := len(n.Tasks) - n.remainingTasks()
	return fmt.Sprintf("%d/%d", done, len(n.Tasks))
}

// ---------------------------------------------------------------------------
// Notes persistence
// ---------------------------------------------------------------------------

func notesFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".jirlab-notes.json"
	}
	return filepath.Join(home, ".jirlab", "notes.json")
}

func loadNotes(path string) []note {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is fine
	}
	var notes []note
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil // corrupt file: start fresh
	}
	return notes
}

func saveNotesCmd(path string, notes []note) tea.Cmd {
	return func() tea.Msg {
		data, err := json.MarshalIndent(notes, "", "  ")
		if err != nil {
			return errMsg{source: "notes", err: err}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errMsg{source: "notes", err: err}
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return errMsg{source: "notes", err: err}
		}
		return notesSavedMsg{}
	}
}

// notesSavedMsg is returned after a successful notes save.
type notesSavedMsg struct{}

// notesUpdatedMsg is sent when notes data changes in the tracker.
type notesUpdatedMsg struct{ notes []note }

// ---------------------------------------------------------------------------
// Sort helpers
// ---------------------------------------------------------------------------

type notesSortMode int

const (
	notesSortDate notesSortMode = iota
	notesSortPriority
	notesSortRemaining
)

func (m notesSortMode) Label() string {
	switch m {
	case notesSortPriority:
		return "priority"
	case notesSortRemaining:
		return "remaining"
	default:
		return "date"
	}
}

func sortNotes(notes []note, mode notesSortMode) []note {
	out := make([]note, len(notes))
	copy(out, notes)
	switch mode {
	case notesSortPriority:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Priority > out[j].Priority
		})
	case notesSortRemaining:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].remainingTasks() > out[j].remainingTasks()
		})
	default: // date, newest first
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
	}
	return out
}

func filterNotes(notes []note, category string) []note {
	if category == "" {
		return notes
	}
	var out []note
	for _, n := range notes {
		if n.Category == category {
			out = append(out, n)
		}
	}
	return out
}

func uniqueCategories(notes []note) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, n := range notes {
		if n.Category != "" && !seen[n.Category] {
			seen[n.Category] = true
			cats = append(cats, n.Category)
		}
	}
	sort.Strings(cats)
	return cats
}

// ---------------------------------------------------------------------------
// NoteCreateModal
// ---------------------------------------------------------------------------

type noteCreateField int

const (
	noteFieldTitle noteCreateField = iota
	noteFieldCategory
	noteFieldPriority
	noteFieldDescription
	noteFieldTask
	noteFieldCount
)

// NoteCreateModal is a multi-field modal for creating a note with tab-switching.
type NoteCreateModal struct {
	fields     [noteFieldCount]textinput.Model
	focusedIdx noteCreateField
	priority   notePriority
	tasks      []string
	categories []string // existing categories for hint display
	onConfirm  func(n note) tea.Cmd
}

func NewNoteCreateModal(existingCategories []string, onConfirm func(n note) tea.Cmd) NoteCreateModal {
	m := NoteCreateModal{
		categories: existingCategories,
		onConfirm:  onConfirm,
	}
	placeholders := [noteFieldCount]string{
		"Note title",
		"Category (e.g. work, personal)",
		"", // priority — not a textinput
		"Short description or reminder",
		"Add a task and press Enter",
	}
	for i := range m.fields {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.CharLimit = 200
		m.fields[i] = ti
	}
	m.fields[noteFieldTitle].Focus()
	return m
}

func (m NoteCreateModal) update(msg tea.Msg) (modal, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		if m.focusedIdx != noteFieldPriority {
			m.fields[m.focusedIdx], cmd = m.fields[m.focusedIdx].Update(msg)
		}
		return m, cmd
	}

	switch k.String() {
	case "esc":
		return nil, nil

	case "tab":
		// cycle forward through fields
		m.fields[m.focusedIdx].Blur()
		m.focusedIdx = (m.focusedIdx + 1) % noteFieldCount
		if m.focusedIdx != noteFieldPriority {
			m.fields[m.focusedIdx].Focus()
		}
		return m, nil

	case "shift+tab":
		m.fields[m.focusedIdx].Blur()
		m.focusedIdx = (m.focusedIdx + noteFieldCount - 1) % noteFieldCount
		if m.focusedIdx != noteFieldPriority {
			m.fields[m.focusedIdx].Focus()
		}
		return m, nil

	case "left", "right":
		if m.focusedIdx == noteFieldPriority {
			if k.String() == "right" {
				m.priority = (m.priority + 1) % 3
			} else {
				m.priority = (m.priority + 2) % 3
			}
			return m, nil
		}

	case "enter":
		if m.focusedIdx == noteFieldTask {
			text := strings.TrimSpace(m.fields[noteFieldTask].Value())
			if text != "" {
				m.tasks = append(m.tasks, text)
				m.fields[noteFieldTask].SetValue("")
			}
			return m, nil
		}
		// enter on other fields = confirm
		return m.confirm()

	case "ctrl+d":
		// remove last task
		if m.focusedIdx == noteFieldTask && len(m.tasks) > 0 {
			m.tasks = m.tasks[:len(m.tasks)-1]
			return m, nil
		}
	}

	// pass keystrokes to the focused text input
	if m.focusedIdx != noteFieldPriority {
		var cmd tea.Cmd
		m.fields[m.focusedIdx], cmd = m.fields[m.focusedIdx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m NoteCreateModal) confirm() (modal, tea.Cmd) {
	title := strings.TrimSpace(m.fields[noteFieldTitle].Value())
	if title == "" {
		return m, nil // require at least a title
	}
	now := time.Now()
	id := fmt.Sprintf("%d", now.UnixNano())
	tasks := make([]noteTask, len(m.tasks))
	for i, t := range m.tasks {
		tasks[i] = noteTask{ID: fmt.Sprintf("%d-%d", now.UnixNano(), i), Text: t}
	}
	n := note{
		ID:          id,
		Title:       title,
		Category:    strings.TrimSpace(m.fields[noteFieldCategory].Value()),
		Priority:    m.priority,
		Description: strings.TrimSpace(m.fields[noteFieldDescription].Value()),
		Tasks:       tasks,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return nil, m.onConfirm(n)
}

func (m NoteCreateModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 52 {
		maxW = 52
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Width(14)
	hintStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Bold(true).Foreground(colorActive).Width(14)

	lbl := func(i noteCreateField, text string) string {
		if i == m.focusedIdx {
			return activeLabel.Render("▶ " + text)
		}
		return labelStyle.Render("  " + text)
	}

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render("New Note") + "\n\n")

	// Title
	sb.WriteString(lbl(noteFieldTitle, "Title") + "\n")
	m.fields[noteFieldTitle].Width = maxW - 18
	sb.WriteString("              " + m.fields[noteFieldTitle].View() + "\n\n")

	// Category
	sb.WriteString(lbl(noteFieldCategory, "Category") + "\n")
	m.fields[noteFieldCategory].Width = maxW - 18
	sb.WriteString("              " + m.fields[noteFieldCategory].View())
	if len(m.categories) > 0 && m.focusedIdx == noteFieldCategory {
		sb.WriteString("  " + hintStyle.Render("existing: "+strings.Join(m.categories, ", ")))
	}
	sb.WriteString("\n\n")

	// Priority
	prioLabel := lbl(noteFieldPriority, "Priority")
	prioVal := lipgloss.NewStyle().Foreground(m.priority.Color()).Bold(true).Render(m.priority.String())
	if m.focusedIdx == noteFieldPriority {
		prioVal += hintStyle.Render("  ← →")
	}
	sb.WriteString(prioLabel + " " + prioVal + "\n\n")

	// Description
	sb.WriteString(lbl(noteFieldDescription, "Description") + "\n")
	m.fields[noteFieldDescription].Width = maxW - 18
	sb.WriteString("              " + m.fields[noteFieldDescription].View() + "\n\n")

	// Tasks
	sb.WriteString(lbl(noteFieldTask, "Tasks") + "\n")
	for i, t := range m.tasks {
		sb.WriteString(fmt.Sprintf("              %d. %s\n", i+1, t))
	}
	m.fields[noteFieldTask].Width = maxW - 18
	sb.WriteString("              " + m.fields[noteFieldTask].View())
	if m.focusedIdx == noteFieldTask {
		sb.WriteString(hintStyle.Render("  enter: add  ctrl+d: remove last"))
	}
	sb.WriteString("\n")

	sb.WriteString(modalHintStyle.Render("\ntab/shift+tab: next/prev field  enter: confirm  esc: cancel"))
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---------------------------------------------------------------------------
// NoteDetailModal
// ---------------------------------------------------------------------------

// NoteDetailModal shows a note's details and lets the user toggle tasks.
type NoteDetailModal struct {
	noteIdx    int // index in the source notes slice
	n          note
	taskCursor int
	onSave     func(updated note) tea.Cmd
}

func NewNoteDetailModal(idx int, n note, onSave func(updated note) tea.Cmd) NoteDetailModal {
	return NoteDetailModal{noteIdx: idx, n: n, onSave: onSave}
}

func (m NoteDetailModal) update(msg tea.Msg) (modal, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc", "q":
		return nil, nil
	case "j", "down":
		if m.taskCursor < len(m.n.Tasks)-1 {
			m.taskCursor++
		}
	case "k", "up":
		if m.taskCursor > 0 {
			m.taskCursor--
		}
	case " ", "enter":
		if len(m.n.Tasks) == 0 || m.taskCursor >= len(m.n.Tasks) {
			break
		}
		m.n.Tasks[m.taskCursor].Done = !m.n.Tasks[m.taskCursor].Done
		m.n.UpdatedAt = time.Now()

		// If we just completed the last remaining task, prompt delete/keep.
		if m.n.Tasks[m.taskCursor].Done && m.n.remainingTasks() == 0 && len(m.n.Tasks) > 0 {
			updatedNote := m.n
			onSave := m.onSave
			idx := m.noteIdx
			return nil, func() tea.Msg {
				return OpenModalMsg{M: noteAllDoneModal{
					noteIdx: idx,
					n:       updatedNote,
					onSave:  onSave,
				}}
			}
		}
		return m, m.onSave(m.n)
	}
	return m, nil
}

func (m NoteDetailModal) view(width, height int) string {
	maxW := width - 10
	if maxW < 52 {
		maxW = 52
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Width(14)
	row := func(k, v string) string {
		return labelStyle.Render(k+":") + " " + v + "\n"
	}

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(m.n.Title) + "\n\n")
	sb.WriteString(row("Category", m.n.Category))
	prioStr := lipgloss.NewStyle().Foreground(m.n.Priority.Color()).Bold(true).Render(m.n.Priority.String())
	sb.WriteString(row("Priority", prioStr))
	sb.WriteString(row("Created", m.n.CreatedAt.Format("2006-01-02 15:04")))
	if m.n.Description != "" {
		sb.WriteString("\n" + labelStyle.Render("Description:") + "\n")
		sb.WriteString("  " + wordWrap(m.n.Description, maxW-6) + "\n")
	}
	if len(m.n.Tasks) > 0 {
		sb.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Render("Tasks:") + "\n")
		for i, t := range m.n.Tasks {
			check := "[ ]"
			col := colorFg
			if t.Done {
				check = "[✓]"
				col = colorGreen
			}
			line := fmt.Sprintf("  %s %s", check, t.Text)
			if i == m.taskCursor {
				sb.WriteString(selectedRowStyle.Foreground(col).Width(maxW - 8).Render(line) + "\n")
			} else {
				sb.WriteString(normalRowStyle.Foreground(col).Render(line) + "\n")
			}
		}
	}

	sb.WriteString(modalHintStyle.Render("\nspace/enter: toggle task  j/k: navigate  esc: close"))

	lines := strings.Split(sb.String(), "\n")
	maxVis := height - 6
	if maxVis < 5 {
		maxVis = 5
	}
	if len(lines) > maxVis {
		lines = lines[:maxVis]
	}
	return modalBoxStyle.Width(maxW).Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// noteAllDoneModal — prompt after last task is completed
// ---------------------------------------------------------------------------

type noteAllDoneModal struct {
	noteIdx int
	n       note
	onSave  func(updated note) tea.Cmd
}

func (m noteAllDoneModal) update(msg tea.Msg) (modal, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "k", "K":
		// keep: save the note with all tasks done
		return nil, m.onSave(m.n)
	case "d", "D":
		// delete
		return nil, func() tea.Msg {
			return noteDeleteMsg{noteIdx: m.noteIdx}
		}
	case "esc":
		return nil, m.onSave(m.n)
	}
	return m, nil
}

func (m noteAllDoneModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 48 {
		maxW = 48
	}
	body := modalTitleStyle.Render("All tasks complete! 🎉") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorFg).Render(`"` + truncStr(m.n.Title, maxW-10) + `"`) + "\n\n" +
		"All tasks are done. What would you like to do?\n" +
		modalHintStyle.Render("\nk: keep  d: delete  esc: keep")
	return modalBoxStyle.Width(maxW).Render(body)
}

// noteDeleteMsg is sent when the user chooses to delete a fully-completed note.
type noteDeleteMsg struct{ noteIdx int }
