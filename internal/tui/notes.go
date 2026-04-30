package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/notes"
)

// ---------------------------------------------------------------------------
// Note helpers (TUI-layer only — colour and display logic)
// ---------------------------------------------------------------------------

func notePriorityColor(p notes.Priority) lipgloss.Color {
	switch p {
	case notes.PriorityHigh:
		return colorRed
	case notes.PriorityMedium:
		return colorYellow
	default:
		return colorGreen
	}
}

func noteRemainingTasks(n notes.Note) int {
	count := 0
	for _, t := range n.Tasks {
		if !t.Done {
			count++
		}
	}
	return count
}

func noteTaskSummary(n notes.Note) string {
	if len(n.Tasks) == 0 {
		return "-"
	}
	done := len(n.Tasks) - noteRemainingTasks(n)
	return fmt.Sprintf("%d/%d", done, len(n.Tasks))
}

// ---------------------------------------------------------------------------
// Persistence tea.Cmds
// ---------------------------------------------------------------------------

// notesSavedMsg is returned after a successful note file operation.
type notesSavedMsg struct{}

// noteSavedMsg is sent when a note is created or updated.
type noteSavedMsg struct {
	note  notes.Note
	isNew bool
}

// noteRemovedMsg is sent when a note should be deleted from the store.
type noteRemovedMsg struct{ id string }

func notePersistCmd(store notes.Store, n notes.Note) tea.Cmd {
	return func() tea.Msg {
		if err := store.Save(n); err != nil {
			return errMsg{source: "notes", err: err}
		}
		return notesSavedMsg{}
	}
}

func noteDeleteFileCmd(store notes.Store, id string) tea.Cmd {
	return func() tea.Msg {
		if err := store.Delete(id); err != nil {
			return errMsg{source: "notes", err: err}
		}
		return notesSavedMsg{}
	}
}

// ---------------------------------------------------------------------------
// Sort helpers
// ---------------------------------------------------------------------------

type notesSortMode int

const (
	notesSortDate      notesSortMode = iota
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

func sortNotes(ns []notes.Note, mode notesSortMode) []notes.Note {
	out := make([]notes.Note, len(ns))
	copy(out, ns)
	switch mode {
	case notesSortPriority:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Priority > out[j].Priority
		})
	case notesSortRemaining:
		sort.SliceStable(out, func(i, j int) bool {
			return noteRemainingTasks(out[i]) > noteRemainingTasks(out[j])
		})
	default: // date, newest first
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
	}
	return out
}

func filterNotes(ns []notes.Note, category string) []notes.Note {
	if category == "" {
		return ns
	}
	var out []notes.Note
	for _, n := range ns {
		if n.Category == category {
			out = append(out, n)
		}
	}
	return out
}

func uniqueCategories(ns []notes.Note) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, n := range ns {
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
	priority   notes.Priority
	tasks      []string
	categories []string // existing categories for hint display
	onConfirm  func(n notes.Note) tea.Cmd
}

func NewNoteCreateModal(existingCategories []string, onConfirm func(n notes.Note) tea.Cmd) NoteCreateModal {
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
				m.priority = notes.Priority((int(m.priority) + 1) % 3)
			} else {
				m.priority = notes.Priority((int(m.priority) + 2) % 3)
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
	id, err := notes.NewID()
	if err != nil {
		id = fmt.Sprintf("%d", now.UnixNano()) // fallback
	}
	tasks := make([]notes.Task, len(m.tasks))
	for i, t := range m.tasks {
		taskID, _ := notes.NewID()
		tasks[i] = notes.Task{ID: taskID, Text: t}
	}
	n := notes.Note{
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
	prioVal := lipgloss.NewStyle().Foreground(notePriorityColor(m.priority)).Bold(true).Render(m.priority.String())
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
	n          notes.Note
	taskCursor int
	onSave     func(updated notes.Note) tea.Cmd
}

func NewNoteDetailModal(n notes.Note, onSave func(updated notes.Note) tea.Cmd) NoteDetailModal {
	return NoteDetailModal{n: n, onSave: onSave}
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
		if m.n.Tasks[m.taskCursor].Done && noteRemainingTasks(m.n) == 0 && len(m.n.Tasks) > 0 {
			updatedNote := m.n
			onSave := m.onSave
			return nil, func() tea.Msg {
				return OpenModalMsg{M: noteAllDoneModal{
					noteID: updatedNote.ID,
					n:      updatedNote,
					onSave: onSave,
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
	prioStr := lipgloss.NewStyle().Foreground(notePriorityColor(m.n.Priority)).Bold(true).Render(m.n.Priority.String())
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

type noteAllDoneModal struct {
	noteID string
	n      notes.Note
	onSave func(updated notes.Note) tea.Cmd
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
		id := m.noteID
		return nil, func() tea.Msg {
			return noteRemovedMsg{id: id}
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


