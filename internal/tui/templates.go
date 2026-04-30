package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/templates"
)

// ---------------------------------------------------------------------------
// Persistence tea.Cmds
// ---------------------------------------------------------------------------

// templateSavedMsg is sent when a template is created or updated.
type templateSavedMsg struct {
	tmpl  templates.Template
	isNew bool
}

// templateRemovedMsg is sent when a template should be deleted.
type templateRemovedMsg struct{ id string }

func templatePersistCmd(store templates.Store, t templates.Template) tea.Cmd {
	return func() tea.Msg {
		if err := store.Save(t); err != nil {
			return errMsg{source: "templates", err: err}
		}
		return struct{}{}
	}
}

func templateDeleteFileCmd(store templates.Store, id string) tea.Cmd {
	return func() tea.Msg {
		if err := store.Delete(id); err != nil {
			return errMsg{source: "templates", err: err}
		}
		return struct{}{}
	}
}

// ---------------------------------------------------------------------------
// TemplateCreateModal
// ---------------------------------------------------------------------------

type templateCreateField int

const (
	tmplFieldName templateCreateField = iota
	tmplFieldBody
	tmplFieldCount
)

// TemplateCreateModal is a two-field modal for creating or editing a template.
type TemplateCreateModal struct {
	fields    [tmplFieldCount]textinput.Model
	focusIdx  templateCreateField
	existing  *templates.Template // non-nil in edit mode
	onConfirm func(t templates.Template) tea.Cmd
}

// NewTemplateCreateModal returns a modal for creating a new template.
func NewTemplateCreateModal(onConfirm func(t templates.Template) tea.Cmd) TemplateCreateModal {
	return newTemplateModal(nil, onConfirm)
}

// NewTemplateEditModal returns a modal pre-filled with an existing template for editing.
func NewTemplateEditModal(existing templates.Template, onConfirm func(t templates.Template) tea.Cmd) TemplateCreateModal {
	return newTemplateModal(&existing, onConfirm)
}

func newTemplateModal(existing *templates.Template, onConfirm func(t templates.Template) tea.Cmd) TemplateCreateModal {
	m := TemplateCreateModal{
		existing:  existing,
		onConfirm: onConfirm,
	}

	nameTI := textinput.New()
	nameTI.Placeholder = "Template name"
	nameTI.CharLimit = 120

	bodyTI := textinput.New()
	bodyTI.Placeholder = "Comment body (multi-line text is stored as-is)"
	bodyTI.CharLimit = 0 // no limit

	if existing != nil {
		nameTI.SetValue(existing.Name)
		bodyTI.SetValue(existing.Body)
	}

	m.fields[tmplFieldName] = nameTI
	m.fields[tmplFieldBody] = bodyTI
	m.fields[tmplFieldName].Focus()
	return m
}

func (m TemplateCreateModal) update(msg tea.Msg) (modal, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		m.fields[m.focusIdx], cmd = m.fields[m.focusIdx].Update(msg)
		return m, cmd
	}

	switch k.String() {
	case "esc":
		return nil, nil

	case "tab":
		m.fields[m.focusIdx].Blur()
		m.focusIdx = (m.focusIdx + 1) % tmplFieldCount
		m.fields[m.focusIdx].Focus()
		return m, nil

	case "shift+tab":
		m.fields[m.focusIdx].Blur()
		m.focusIdx = (m.focusIdx + tmplFieldCount - 1) % tmplFieldCount
		m.fields[m.focusIdx].Focus()
		return m, nil

	case "enter":
		return m.confirm()
	}

	var cmd tea.Cmd
	m.fields[m.focusIdx], cmd = m.fields[m.focusIdx].Update(msg)
	return m, cmd
}

func (m TemplateCreateModal) confirm() (modal, tea.Cmd) {
	name := strings.TrimSpace(m.fields[tmplFieldName].Value())
	if name == "" {
		return m, nil // require at least a name
	}
	now := time.Now()

	var id string
	var createdAt time.Time
	if m.existing != nil {
		id = m.existing.ID
		createdAt = m.existing.CreatedAt
	} else {
		var err error
		id, err = templates.NewID()
		if err != nil {
			id = fmt.Sprintf("%d", now.UnixNano())
		}
		createdAt = now
	}

	t := templates.Template{
		ID:        id,
		Name:      name,
		Body:      m.fields[tmplFieldBody].Value(),
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	return nil, m.onConfirm(t)
}

func (m TemplateCreateModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 52 {
		maxW = 52
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Width(10)
	activeLabel := lipgloss.NewStyle().Bold(true).Foreground(colorActive).Width(10)

	lbl := func(i templateCreateField, text string) string {
		if i == m.focusIdx {
			return activeLabel.Render("▶ " + text)
		}
		return labelStyle.Render("  " + text)
	}

	title := "New Template"
	if m.existing != nil {
		title = "Edit Template"
	}

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(title) + "\n\n")

	sb.WriteString(lbl(tmplFieldName, "Name") + "\n")
	m.fields[tmplFieldName].Width = maxW - 14
	sb.WriteString("          " + m.fields[tmplFieldName].View() + "\n\n")

	sb.WriteString(lbl(tmplFieldBody, "Body") + "\n")
	m.fields[tmplFieldBody].Width = maxW - 14
	sb.WriteString("          " + m.fields[tmplFieldBody].View() + "\n")

	sb.WriteString(modalHintStyle.Render("\ntab/shift+tab: next/prev field  enter: confirm  esc: cancel"))
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---------------------------------------------------------------------------
// Comment template picker
// ---------------------------------------------------------------------------

// newCommentWithTemplateModal returns a ListSelectModal that lets the user
// pick a stored template before composing a Jira comment. Selecting a template
// pre-fills the comment input. The first item is always "(edit without template)".
func newCommentWithTemplateModal(tmpls []templates.Template, commentTitle string, onConfirm func(string) tea.Cmd) modal {
	items := make([]string, 0, len(tmpls)+1)
	items = append(items, "(edit without template)")
	for _, t := range tmpls {
		preview := truncStr(t.Body, 40)
		preview = strings.ReplaceAll(preview, "\n", " ")
		items = append(items, t.Name+" — "+preview)
	}

	return ListSelectModal{
		title: "Select Template",
		items: items,
		onSelect: func(idx int) tea.Cmd {
			return func() tea.Msg {
				if idx == 0 {
					return OpenModalMsg{M: newCommentInputModal(commentTitle, "", onConfirm)}
				}
				body := tmpls[idx-1].Body
				return OpenModalMsg{M: newCommentInputModal(commentTitle, body, onConfirm)}
			}
		},
	}
}

// newCommentInputModal creates an InputModal suitable for composing Jira comments.
// It uses a higher character limit than the default InputModal.
func newCommentInputModal(title, prefill string, onSubmit func(string) tea.Cmd) InputModal {
	ti := textinput.New()
	ti.Placeholder = "Type your comment..."
	ti.CharLimit = 0 // no limit for comment bodies
	if prefill != "" {
		ti.SetValue(prefill)
	}
	ti.Focus()
	return InputModal{title: title, input: ti, onSubmit: onSubmit}
}
