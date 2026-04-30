package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// modal is implemented by anything that can be displayed as an overlay.
// update returns nil to signal that the modal should be closed.
type modal interface {
	update(msg tea.Msg) (modal, tea.Cmd)
	view(width, height int) string
}

// OpenModalMsg is sent by sections when they want to open a modal.
type OpenModalMsg struct{ M modal }

// ---------------------------------------------------------------------------
// HelpModal
// ---------------------------------------------------------------------------

// HelpModal shows all global + section-specific hotkeys, with scroll support.
type HelpModal struct {
	sectionKeys        []HelpEntry
	sectionName        string
	scroll             int
	topColorEntries    []legendEntry // color hints for the top table
	topColorLabel      string
	bottomColorEntries []legendEntry // color hints for the active bottom table (nil = no bottom table)
	bottomColorLabel   string
}

func (m HelpModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "?", "q":
			return nil, nil
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "pgup":
			m.scroll -= 10
			if m.scroll < 0 {
				m.scroll = 0
			}
		case "pgdown":
			m.scroll += 10
		}
	}
	return m, nil
}

func (m HelpModal) view(width, height int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}

	colW := (maxW - 6) / 2 // 6 = border+padding overhead per side
	if colW < 18 {
		colW = 18
	}

	plain := lipgloss.NewStyle()
	sectionLabel := lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	leftCol := plain.Width(colW)
	rightCol := plain.Width(colW)

	keyLine := func(e HelpEntry) string {
		return fmt.Sprintf("  %-10s %s", e.Key, e.Desc)
	}

	// --- Build 2-column key rows ---
	globalRows := GlobalKeys
	sectionRows := m.sectionKeys

	nRows := len(globalRows)
	if len(sectionRows) > nRows {
		nRows = len(sectionRows)
	}

	var lines []string
	// Column headers
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
		leftCol.Render(sectionLabel.Render("Global")),
		rightCol.Render(sectionLabel.Render(m.sectionName)),
	)
	lines = append(lines, headerRow)

	for i := 0; i < nRows; i++ {
		left, right := "", ""
		if i < len(globalRows) {
			left = keyLine(globalRows[i])
		}
		if i < len(sectionRows) {
			right = keyLine(sectionRows[i])
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top,
			leftCol.Render(left),
			rightCol.Render(right),
		))
	}
	lines = append(lines, "")

	// --- Colour legend: left = top table, right = active bottom table (mirrors hotkey layout) ---
	if len(m.topColorEntries) > 0 {
		// keyColStyle pads to the same 10-char slot used by keyLine's %-10s,
		// so the label text aligns with the key-description column above.
		keyColStyle := lipgloss.NewStyle().Width(10)
		entryLine := func(e legendEntry) string {
			bullet := lipgloss.NewStyle().Foreground(e.color).Render("●")
			return "  " + keyColStyle.Render(bullet) + " " + e.label
		}

		leftEntries := m.topColorEntries
		rightEntries := m.bottomColorEntries

		nColorRows := len(leftEntries)
		if len(rightEntries) > nColorRows {
			nColorRows = len(rightEntries)
		}

		// Header row for color columns
		leftLabel := m.topColorLabel
		if leftLabel == "" {
			leftLabel = "Colours"
		}
		rightLabel := m.bottomColorLabel
		colorHeaderRow := lipgloss.JoinHorizontal(lipgloss.Top,
			leftCol.Render(sectionLabel.Render(leftLabel)),
			rightCol.Render(sectionLabel.Render(rightLabel)),
		)
		lines = append(lines, colorHeaderRow)

		for i := 0; i < nColorRows; i++ {
			left, right := "", ""
			if i < len(leftEntries) {
				left = entryLine(leftEntries[i])
			}
			if i < len(rightEntries) {
				right = entryLine(rightEntries[i])
			}
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top,
				leftCol.Render(left),
				rightCol.Render(right),
			))
		}
	}

	total := len(lines)

	// border(2) + padding top/bottom(2) + title+gap(2) + hint(2) = 8 rows overhead
	const overhead = 8
	visibleH := height - overhead
	if visibleH < 5 {
		visibleH = 5
	}

	// Clamp scroll.
	maxScroll := total - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visibleH
	if end > total {
		end = total
	}
	visible := lines[scroll:end]

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render("Keyboard Shortcuts") + "\n\n")

	if scroll > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  ↑ more above") + "\n")
	}
	sb.WriteString(strings.Join(visible, "\n"))
	if end < total {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(colorMuted).Render("  ↓ more below"))
	}

	hint := "esc/? to close"
	if total > visibleH {
		hint += "  ↑/↓: scroll"
	}
	sb.WriteString(modalHintStyle.Render("\n" + hint))

	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---------------------------------------------------------------------------
// InputModal
// ---------------------------------------------------------------------------

// InputModal displays a single-line text input with a prompt.
// onSubmit is called with the entered value when the user presses Enter.
type InputModal struct {
	title    string
	input    textinput.Model
	onSubmit func(string) tea.Cmd
}

// NewInputModal creates an InputModal ready to use.
func NewInputModal(title, placeholder string, onSubmit func(string) tea.Cmd) InputModal {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 200
	ti.Focus()
	return InputModal{title: title, input: ti, onSubmit: onSubmit}
}

// NewInputModalWithValue creates an InputModal with a pre-filled value.
func NewInputModalWithValue(title, value string, onSubmit func(string) tea.Cmd) InputModal {
	ti := textinput.New()
	ti.CharLimit = 200
	ti.SetValue(value)
	ti.Focus()
	return InputModal{title: title, input: ti, onSubmit: onSubmit}
}

func (m InputModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "enter":
			var cmd tea.Cmd
			if m.onSubmit != nil {
				cmd = m.onSubmit(m.input.Value())
			}
			return nil, cmd
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m InputModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}
	body := modalTitleStyle.Render(m.title) + "\n\n" +
		m.input.View() +
		modalHintStyle.Render("\n\nenter: confirm  esc: cancel")
	return modalBoxStyle.Width(maxW).Render(body)
}

// ---------------------------------------------------------------------------
// NumericHoursModal
// ---------------------------------------------------------------------------

// NumericHoursModal is a single-character numeric input for logging hours.
// Each digit keypress overwrites the current value; non-digit input is ignored.
// The default value is "8". onSubmit receives the hours as an integer.
type NumericHoursModal struct {
	title    string
	value    string // always a single digit string, default "8"
	onSubmit func(hours int) tea.Cmd
}

// NewNumericHoursModal creates a NumericHoursModal with a default of 8 hours.
func NewNumericHoursModal(title string, onSubmit func(hours int) tea.Cmd) NumericHoursModal {
	return NumericHoursModal{title: title, value: "8", onSubmit: onSubmit}
}

func (m NumericHoursModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "enter":
			if m.onSubmit != nil {
				hours := int(m.value[0] - '0')
				if hours == 0 {
					hours = 1 // prevent 0-hour log
				}
				return nil, m.onSubmit(hours)
			}
			return nil, nil
		default:
			m.value = NumericInputHandleKey(m.value, k.String())
		}
	}
	return m, nil
}

func (m NumericHoursModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}
	display := lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render(m.value + "h")
	body := modalTitleStyle.Render(m.title) + "\n\n" +
		"Hours: " + display +
		modalHintStyle.Render("\n\n0-9: set hours  enter: confirm  esc: cancel")
	return modalBoxStyle.Width(maxW).Render(body)
}

// ---------------------------------------------------------------------------
// ConfirmModal
// ---------------------------------------------------------------------------

// ConfirmModal asks the user to confirm an action with y/n.
type ConfirmModal struct {
	message   string
	onConfirm tea.Cmd
}

func (m ConfirmModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y":
			return nil, m.onConfirm
		case "n", "N", "esc":
			return nil, nil
		}
	}
	return m, nil
}

func (m ConfirmModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}
	body := modalTitleStyle.Render("Confirm") + "\n\n" +
		m.message +
		modalHintStyle.Render("\n\ny: yes  n/esc: cancel")
	return modalBoxStyle.Width(maxW).Render(body)
}

// ---------------------------------------------------------------------------
// DescriptionModal
// ---------------------------------------------------------------------------

// DescriptionModal shows full details of a Jira issue.
type DescriptionModal struct {
	Issue        service.Issue
	scrollOffset int
}

func (m DescriptionModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q", "d", "enter":
			return nil, nil
		case "j", "down":
			m.scrollOffset++
		case "k", "up":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		}
	}
	return m, nil
}

func (m DescriptionModal) view(width, height int) string {
	issue := m.Issue
	label := lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Width(14)
	value := lipgloss.NewStyle().Foreground(colorFg)

	row := func(k, v string) string {
		return label.Render(k+":") + " " + value.Render(v) + "\n"
	}

	maxW := width - 10
	if maxW < 50 {
		maxW = 50
	}
	bodyW := maxW - 4 // inner padding

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(fmt.Sprintf("Issue %s", issue.Key)) + "\n\n")
	sb.WriteString(row("Summary", issue.Summary))
	sb.WriteString(row("Status", issue.Status))
	sb.WriteString(row("Assignee", issue.Assignee))
	sb.WriteString(row("Priority", issue.Priority))
	sb.WriteString(row("Type", issue.IssueType))
	sb.WriteString(row("Sprint", issue.SprintName))
	if issue.BranchName != "" {
		sb.WriteString(row("Branch", issue.BranchName))
	}

	// Description
	if issue.Description != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Render("Description:") + "\n")
		wrapped := wordWrap(issue.Description, bodyW)
		sb.WriteString(wrapped + "\n")
	}

	// Fix versions
	if len(issue.FixVersions) > 0 {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Render("Fix Versions:") + "\n")
		for _, fv := range issue.FixVersions {
			sb.WriteString("  • " + fv + "\n")
		}
	}

	// Comments
	if len(issue.Comments) > 0 {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Render(fmt.Sprintf("Comments (%d):", len(issue.Comments))) + "\n")
		for _, c := range issue.Comments {
			sb.WriteString(fmt.Sprintf("\n  %s  %s\n",
				lipgloss.NewStyle().Foreground(colorBlue).Render(c.Author),
				lipgloss.NewStyle().Foreground(colorSubtle).Render(c.Created.Format("2006-01-02")),
			))
			wrapped := wordWrap(c.Body, bodyW-4)
			for _, line := range strings.Split(wrapped, "\n") {
				sb.WriteString("    " + line + "\n")
			}
		}
	}

	sb.WriteString(modalHintStyle.Render("\nj/k: scroll  esc: close"))

	// Apply scrolling by trimming top lines
	lines := strings.Split(sb.String(), "\n")
	maxVisible := height - 6
	if maxVisible < 5 {
		maxVisible = 5
	}
	offset := m.scrollOffset
	if offset > len(lines)-maxVisible {
		offset = len(lines) - maxVisible
	}
	if offset < 0 {
		offset = 0
	}
	visible := lines[offset:]
	if len(visible) > maxVisible {
		visible = visible[:maxVisible]
	}

	return modalBoxStyle.Width(maxW).Render(strings.Join(visible, "\n"))
}

// ---------------------------------------------------------------------------
// LoadingModal
// ---------------------------------------------------------------------------

// LoadingModal is a simple spinner-style placeholder shown while data loads.
type LoadingModal struct {
	Message string
}

func (m LoadingModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
		return nil, nil
	}
	return m, nil
}

func (m LoadingModal) view(width, _ int) string {
	maxW := width - 20
	if maxW < 40 {
		maxW = 40
	}
	body := modalTitleStyle.Render("Please wait") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorMuted).Render(m.Message) +
		modalHintStyle.Render("\n\nesc: cancel")
	return modalBoxStyle.Width(maxW).Render(body)
}

// ---------------------------------------------------------------------------
// EditorSelectModal
// ---------------------------------------------------------------------------

// EditorSelectModal lets the user choose an editor to open a repo path.
type EditorSelectModal struct {
	RepoPath string
	Shell    integration.ShellService
}

func (m EditorSelectModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		var editor string
		switch k.String() {
		case "esc":
			return nil, nil
		case "c":
			editor = "code"
		case "i":
			editor = "idea"
		case "v":
			editor = "vim"
		}
		if editor != "" {
			cmd := openEditorCmd(m.Shell, editor, m.RepoPath)
			return nil, cmd
		}
	}
	return m, nil
}

func (m EditorSelectModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}
	body := modalTitleStyle.Render("Open in Editor") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorMuted).Render("Path: ") +
		m.RepoPath + "\n\n" +
		"  c  VS Code\n" +
		"  i  IntelliJ\n" +
		"  v  vim\n" +
		modalHintStyle.Render("\nesc: cancel")
	return modalBoxStyle.Width(maxW).Render(body)
}

// openEditorCmd launches the chosen editor in the given directory.
// GUI editors (code, idea) are started non-blocking in a new session so they
// don't inherit the TUI's controlling terminal and don't steal stdin/stdout.
// The terminal editor vim uses tea.ExecProcess to suspend the TUI.
func openEditorCmd(shell integration.ShellService, editor, path string) tea.Cmd {
	switch editor {
	case "vim":
		// Try neovim at explicit path first, then fall back to system vim.
		// (exec.Command doesn't resolve shell aliases, so we check the path directly)
		vimPath := "vim"
		if _, err := os.Stat("/opt/nvim/bin/nvim"); err == nil {
			vimPath = "/opt/nvim/bin/nvim"
		}
		return tea.ExecProcess(exec.Command(vimPath, path), nil)
	case "idea":
		return func() tea.Msg {
			bin, err := shell.ResolveEditorPath("idea")
			if err != nil || bin == "" {
				return errMsg{source: "repos", err: fmt.Errorf("IntelliJ IDEA not found")}
			}
			projectDir := shell.FindMavenRoot(path)
			cmd := exec.Command(bin, projectDir)
			cmd.SysProcAttr = detachedSysProcAttr()
			_ = cmd.Start()
			return nil
		}
	default: // "code" and anything else
		return func() tea.Msg {
			bin, _ := shell.ResolveEditorPath("code")
			cmd := exec.Command(bin, path)
			cmd.SysProcAttr = detachedSysProcAttr()
			_ = cmd.Start()
			return nil
		}
	}
}

// ---------------------------------------------------------------------------
// ListSelectModal
// ---------------------------------------------------------------------------

// ListSelectModal shows a scrollable list for the user to select one item.
type ListSelectModal struct {
	title    string
	items    []string
	cursor   int
	onSelect func(idx int) tea.Cmd
}

func (m ListSelectModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.onSelect != nil && m.cursor < len(m.items) {
				cmd := m.onSelect(m.cursor)
				return nil, cmd
			}
			return nil, nil
		}
	}
	return m, nil
}

func (m ListSelectModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 40 {
		maxW = 40
	}
	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(m.title) + "\n\n")
	for i, item := range m.items {
		if i == m.cursor {
			sb.WriteString(selectedRowStyle.Render("▶ "+item) + "\n")
		} else {
			sb.WriteString("  " + item + "\n")
		}
	}
	sb.WriteString(modalHintStyle.Render("\nj/k: navigate  enter: select  esc: cancel"))
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---- KubeOutputModal -------------------------------------------------------
// Scrollable read-only modal for displaying kubectl command output.

type KubeOutputModal struct {
	title        string
	lines        []string
	scrollOffset int
	visibleLines int
}

func NewKubeOutputModal(title, content string) KubeOutputModal {
	lines := strings.Split(content, "\n")
	return KubeOutputModal{title: title, lines: lines, visibleLines: 30}
}

func (m KubeOutputModal) update(msg tea.Msg) (modal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return nil, nil
		case "j", "down":
			maxOff := len(m.lines) - m.visibleLines
			if maxOff < 0 {
				maxOff = 0
			}
			if m.scrollOffset < maxOff {
				m.scrollOffset++
			}
		case "k", "up":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "g":
			m.scrollOffset = 0
		case "G":
			maxOff := len(m.lines) - m.visibleLines
			if maxOff < 0 {
				maxOff = 0
			}
			m.scrollOffset = maxOff
		case "ctrl+f", "pgdown":
			m.scrollOffset += m.visibleLines / 2
			maxOff := len(m.lines) - m.visibleLines
			if m.scrollOffset > maxOff {
				if maxOff < 0 {
					maxOff = 0
				}
				m.scrollOffset = maxOff
			}
		case "ctrl+b", "pgup":
			m.scrollOffset -= m.visibleLines / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
	case tea.WindowSizeMsg:
		m.visibleLines = msg.Height - 10
		if m.visibleLines < 5 {
			m.visibleLines = 5
		}
	}
	return m, nil
}

func (m KubeOutputModal) view(width, height int) string {
	visH := height - 10
	if visH < 5 {
		visH = 5
	}
	m.visibleLines = visH

	maxW := width - 8
	if maxW < 60 {
		maxW = 60
	}

	start := m.scrollOffset
	end := start + visH
	if end > len(m.lines) {
		end = len(m.lines)
	}
	visible := m.lines[start:end]

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(m.title) + "\n\n")
	for _, ln := range visible {
		if len(ln) > maxW-4 {
			ln = ln[:maxW-4]
		}
		sb.WriteString(ln + "\n")
	}
	pct := ""
	if len(m.lines) > 0 {
		pct = fmt.Sprintf("%d/%d", m.scrollOffset+1, len(m.lines))
	}
	sb.WriteString(modalHintStyle.Render("\nj/k: scroll  g/G: top/bottom  ctrl+f/b: page  esc: close  " + pct))
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---------------------------------------------------------------------------
// RefSelectModal
// ---------------------------------------------------------------------------

// RefSelectModal lets the user choose between the current branch and the base
// (default) branch as the ref for a pipeline trigger.
type RefSelectModal struct {
	currentBranch string
	baseBranch    string
	cursor        int // 0 = current branch, 1 = base branch
	onSelect      func(ref string) tea.Cmd
}

func (m RefSelectModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "j", "down":
			m.cursor = 1
		case "k", "up":
			m.cursor = 0
		case "c":
			return nil, m.onSelect(m.currentBranch)
		case "b":
			return nil, m.onSelect(m.baseBranch)
		case "enter":
			ref := m.currentBranch
			if m.cursor == 1 {
				ref = m.baseBranch
			}
			return nil, m.onSelect(ref)
		}
	}
	return m, nil
}

func (m RefSelectModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 52 {
		maxW = 52
	}
	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render("Select Pipeline Ref") + "\n\n")
	items := []string{
		"c: current  — " + m.currentBranch,
		"b: base     — " + m.baseBranch,
	}
	for i, item := range items {
		if i == m.cursor {
			sb.WriteString(selectedRowStyle.Render("▶ "+item) + "\n")
		} else {
			sb.WriteString("  " + item + "\n")
		}
	}
	sb.WriteString(modalHintStyle.Render("\nj/k: navigate  enter: select  b/c: quick pick  esc: cancel"))
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// PipelineTriggerModal
// ---------------------------------------------------------------------------

// pipelineTriggerParam holds one editable CI variable.
type pipelineTriggerParam struct {
	key   string
	desc  string
	input textinput.Model
}

// PipelineTriggerModal lets the user fill in configurable pipeline variables
// before triggering a run. tab cycles between fields; enter confirms.
type PipelineTriggerModal struct {
	title      string
	params     []pipelineTriggerParam
	focusedIdx int
	onConfirm  func(map[string]string) tea.Cmd
}

// NewPipelineTriggerModal creates a PipelineTriggerModal for the given variables.
// If vars is empty the modal will still open but show a "no parameters" message.
func NewPipelineTriggerModal(repoName, ref string, vars []service.PipelineVariable, onConfirm func(map[string]string) tea.Cmd) PipelineTriggerModal {
	params := make([]pipelineTriggerParam, len(vars))
	for i, v := range vars {
		ti := textinput.New()
		ti.Placeholder = v.Value
		ti.SetValue(v.Value)
		ti.CharLimit = 200
		if i == 0 {
			ti.Focus()
		}
		params[i] = pipelineTriggerParam{key: v.Key, desc: v.Description, input: ti}
	}
	return PipelineTriggerModal{
		title:     fmt.Sprintf("Trigger Pipeline — %s @ %s", repoName, ref),
		params:    params,
		onConfirm: onConfirm,
	}
}

func (m PipelineTriggerModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "tab":
			if len(m.params) > 0 {
				m.params[m.focusedIdx].input.Blur()
				m.focusedIdx = (m.focusedIdx + 1) % len(m.params)
				m.params[m.focusedIdx].input.Focus()
			}
			return m, nil
		case "enter":
			values := make(map[string]string, len(m.params))
			for _, p := range m.params {
				values[p.key] = p.input.Value()
			}
			onConfirm := m.onConfirm
			summary := "Trigger pipeline"
			if len(values) > 0 {
				parts := make([]string, 0, len(values))
				for k, v := range values {
					parts = append(parts, k+"="+v)
				}
				summary = "Trigger pipeline with: " + strings.Join(parts, ", ")
			}
			return nil, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message:   summary,
					onConfirm: onConfirm(values),
				}}
			}
		}
	}
	if len(m.params) > 0 {
		var cmd tea.Cmd
		m.params[m.focusedIdx].input, cmd = m.params[m.focusedIdx].input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m PipelineTriggerModal) view(width, _ int) string {
	maxW := width - 10
	if maxW < 52 {
		maxW = 52
	}

	// Compute fixed key-column width so all variable names align.
	keyW := 12
	for _, p := range m.params {
		if l := len(p.key) + 2; l > keyW { // +2 for "▶ " / "  " prefix
			keyW = l
		}
	}

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Width(keyW)

	var sb strings.Builder
	sb.WriteString(modalTitleStyle.Render(m.title) + "\n\n")

	if len(m.params) == 0 {
		sb.WriteString(modalHintStyle.Render("\nenter: trigger  esc: cancel"))
	} else {
		for i, p := range m.params {
			focused := i == m.focusedIdx
			prefix := "  "
			if focused {
				prefix = "▶ "
			}
			sb.WriteString(keyStyle.Render(prefix+p.key) + "\n")
			p.input.Width = maxW - 6
			sb.WriteString("  " + p.input.View() + "\n\n")
		}
		sb.WriteString(modalHintStyle.Render("tab: next field  enter: confirm  esc: cancel"))
	}
	return modalBoxStyle.Width(maxW).Render(sb.String())
}

// ---------------------------------------------------------------------------
// CommandPaletteModal
// ---------------------------------------------------------------------------

// CommandEntry represents one action in the command palette.
type CommandEntry struct {
	Key  string
	Desc string
	Cmd  tea.Cmd
}

// CommandPaletteModal shows context-specific actions for the selected table row.
// It is positioned on the right side of the screen near the selected row
// (anchorY is the approximate screen row to anchor to).
type CommandPaletteModal struct {
	title    string
	commands []CommandEntry
	cursor   int
	AnchorY  int // exported so app.go can read it for positioning
}

func (m CommandPaletteModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "left":
			return nil, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.commands)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.commands) {
				return nil, m.commands[m.cursor].Cmd
			}
		}
	}
	return m, nil
}

func (m CommandPaletteModal) view(width, _ int) string {
	// Width = longest description + border/padding overhead (paletteBoxStyle: Padding(0,1) + border = 4).
	maxDescLen := 1
	for _, cmd := range m.commands {
		if l := len(cmd.Desc); l > maxDescLen {
			maxDescLen = l
		}
	}
	maxW := maxDescLen + 4
	if maxW > width-4 {
		maxW = width - 4
	}

	// Descriptions fill the interior: maxW - 4 accounts for border(2) + padding(2)
	innerW := maxW - 4

	var sb strings.Builder
	for i, cmd := range m.commands {
		if i == m.cursor {
			sb.WriteString(selectedRowStyle.Width(innerW).Render(cmd.Desc) + "\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(colorFg).Width(innerW).Render(cmd.Desc) + "\n")
		}
	}
	content := strings.TrimRight(sb.String(), "\n")
	return paletteBoxStyle.Width(maxW).Render(content)
}
