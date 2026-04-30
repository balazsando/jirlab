package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/notes"
	"github.com/andob/jirlab/internal/templates"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// chatsLoadedMsg is delivered when the pinned-chats fetch completes.
type chatsLoadedMsg struct {
	chats []integration.Chat
	err   error
}

// chatsAuthStartedMsg carries the device code info after StartDeviceCodeFlow.
type chatsAuthStartedMsg struct {
	info integration.DeviceCodeInfo
	err  error
}

// chatsAuthDoneMsg is delivered after ExchangeDeviceCode completes.
type chatsAuthDoneMsg struct{ err error }

// fetchChatsCmd fetches pinned chats from Microsoft Graph.
func fetchChatsCmd(ms integration.MSGraphService) tea.Cmd {
	return func() tea.Msg {
		chats, err := ms.GetPinnedChats(context.Background())
		return chatsLoadedMsg{chats: chats, err: err}
	}
}

// startDeviceCodeCmd starts the device code flow.
func startDeviceCodeCmd(ms integration.MSGraphService) tea.Cmd {
	return func() tea.Msg {
		info, err := ms.StartDeviceCodeFlow(context.Background())
		return chatsAuthStartedMsg{info: info, err: err}
	}
}

// exchangeDeviceCodeCmd exchanges a device code for a token.
func exchangeDeviceCodeCmd(ms integration.MSGraphService, deviceCode string) tea.Cmd {
	return func() tea.Msg {
		err := ms.ExchangeDeviceCode(context.Background(), deviceCode)
		return chatsAuthDoneMsg{err: err}
	}
}

// ---------------------------------------------------------------------------
// ChatsPane — which table has focus
// ---------------------------------------------------------------------------

// ChatsPane identifies which pane of ChatsSection is focused.
// Exported so acceptance tests can inspect the active pane.
type ChatsPane int

const (
	// ChatsPaneTop is the chats (Teams) table at the top.
	ChatsPaneTop ChatsPane = iota
	// ChatsPaneBottom is the notes/templates table at the bottom.
	ChatsPaneBottom
)

// ---------------------------------------------------------------------------
// ChatsSection
// ---------------------------------------------------------------------------

// ChatsSection is the [5] Chats tab — top pane shows pinned Teams chats,
// bottom pane shows notes and templates.
type ChatsSection struct {
	// Chats is the current list of pinned/favorited Teams conversations.
	// Exported so acceptance tests can inspect the state directly.
	Chats       []integration.Chat
	cursor      int
	loading     bool
	statusMsg   string
	authPending bool
	deviceCode  string
	msgraph     integration.MSGraphService
	shell       integration.ShellService
	activePane  ChatsPane

	// Notes sub-pane (mirrors TrackerSection logic)
	notes         []notes.Note
	notesFiltered []notes.Note
	notesCursor   int
	notesSort     notesSortMode
	notesFilter   string
	notesStore    notes.Store

	// Templates sub-pane
	templates       []templates.Template
	templatesCursor int
	templatesStore  templates.Store

	// Bottom sub-pane selector (notes vs templates within the bottom pane)
	bottomSubPane trackerPane // trackerPaneNotes or trackerPaneTemplates
}

// NewChatsSection creates a ChatsSection.
func NewChatsSection(ms integration.MSGraphService, shell integration.ShellService) ChatsSection {
	store, _ := notes.DefaultStore()
	if store == nil {
		store = notes.NewFilesystemStore(os.TempDir() + "/jirlab-notes")
	}
	_ = notes.MigrateFromJSON(store, notes.LegacyJSONPath())
	notesList, _ := store.List()

	tmplStore, _ := templates.DefaultStore()
	if tmplStore == nil {
		tmplStore = templates.NewFilesystemStore(os.TempDir() + "/jirlab-templates")
	}
	tmplList, _ := tmplStore.List()

	s := ChatsSection{
		msgraph:        ms,
		shell:          shell,
		loading:        ms != nil && ms.IsAuthenticated(),
		notes:          notesList,
		notesStore:     store,
		templates:      tmplList,
		templatesStore: tmplStore,
		bottomSubPane:  trackerPaneNotes,
	}
	s.rebuildNotesView()
	return s
}

// ShowAuthPlaceholder returns true when the user must authenticate before chats
// can be displayed.
// Exported so acceptance tests can inspect without rendering.
func (s ChatsSection) ShowAuthPlaceholder() bool {
	return s.msgraph == nil || !s.msgraph.IsAuthenticated()
}

// ActivePane returns which pane currently has keyboard focus.
func (s ChatsSection) ActivePane() ChatsPane { return s.activePane }

// WithChats returns a copy of ChatsSection with the given chat list applied.
// Used by acceptance tests to inject state without running tea.Cmd.
func (s ChatsSection) WithChats(chats []integration.Chat) ChatsSection {
	s.Chats = chats
	s.loading = false
	return s
}

// Init implements tea.Model.
func (s ChatsSection) Init() tea.Cmd {
	if s.msgraph != nil && s.msgraph.IsAuthenticated() {
		return fetchChatsCmd(s.msgraph)
	}
	return nil
}

func (s *ChatsSection) rebuildNotesView() {
	filtered := filterNotes(s.notes, s.notesFilter)
	s.notesFiltered = sortNotes(filtered, s.notesSort)
	if s.notesCursor >= len(s.notesFiltered) {
		s.notesCursor = max(0, len(s.notesFiltered)-1)
	}
}

// Update implements tea.Model (called via app.go routing).
func (s ChatsSection) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cs, cmd := s.update(msg)
	return cs, cmd
}

func (s ChatsSection) update(msg tea.Msg) (ChatsSection, tea.Cmd) {
	switch msg := msg.(type) {
	case chatsLoadedMsg:
		s.loading = false
		if msg.err != nil {
			return s, func() tea.Msg { return errMsg{source: "chats", err: msg.err} }
		}
		s.Chats = msg.chats

	case chatsAuthStartedMsg:
		s.authPending = false
		if msg.err != nil {
			s.statusMsg = "Auth failed: " + msg.err.Error()
			return s, nil
		}
		s.deviceCode = msg.info.DeviceCode
		s.statusMsg = msg.info.Message
		var cmds []tea.Cmd
		if s.shell != nil {
			if msg.info.VerificationURI != "" {
				cmds = append(cmds, openBrowserCmd(s.shell, msg.info.VerificationURI))
			}
			if msg.info.UserCode != "" {
				cmds = append(cmds, copyToClipboard(s.shell, msg.info.UserCode))
			}
		}
		ms := s.msgraph
		dc := msg.info.DeviceCode
		cmds = append(cmds, func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: "Press Enter after signing in to complete authentication",
				onConfirm: func() tea.Msg {
					return exchangeDeviceCodeCmd(ms, dc)()
				},
			}}
		})
		return s, tea.Batch(cmds...)

	case chatsAuthDoneMsg:
		if msg.err != nil {
			s.statusMsg = "Auth error: " + msg.err.Error()
			return s, nil
		}
		s.statusMsg = "Authenticated — loading chats…"
		s.loading = true
		return s, fetchChatsCmd(s.msgraph)

	case notesSavedMsg:
		// fire-and-forget

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

func (s ChatsSection) handleKey(msg tea.KeyMsg) (ChatsSection, tea.Cmd) {
	// Tab: toggle between top (chats) and bottom (notes/templates)
	if msg.String() == "tab" {
		if s.activePane == ChatsPaneTop {
			s.activePane = ChatsPaneBottom
		} else {
			s.activePane = ChatsPaneTop
		}
		return s, nil
	}

	switch s.activePane {
	case ChatsPaneBottom:
		switch msg.String() {
		case "o":
			s.bottomSubPane = trackerPaneNotes
			return s, nil
		case "t":
			s.bottomSubPane = trackerPaneTemplates
			return s, nil
		}
		ts := s.asTrackerSection()
		switch s.bottomSubPane {
		case trackerPaneTemplates:
			ts2, cmd := ts.handleTemplatesKey(msg)
			s.applyFromTracker(ts2)
			return s, cmd
		default:
			ts2, cmd := ts.handleNotesKey(msg)
			s.applyFromTracker(ts2)
			return s, cmd
		}
	default: // ChatsPaneTop
		return s.handleChatsKey(msg)
	}
}

func (s ChatsSection) handleChatsKey(msg tea.KeyMsg) (ChatsSection, tea.Cmd) {
	n := len(s.Chats)
	switch msg.String() {
	case "j", "down":
		if s.cursor < n-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "enter", "w":
		if chat := s.selectedChat(); chat != nil && chat.WebURL != "" && s.shell != nil {
			return s, openBrowserCmd(s.shell, chat.WebURL)
		}
		if s.ShowAuthPlaceholder() && s.msgraph != nil && !s.authPending {
			s.authPending = true
			return s, startDeviceCodeCmd(s.msgraph)
		}
	case "a":
		if s.msgraph != nil && !s.authPending {
			s.authPending = true
			return s, startDeviceCodeCmd(s.msgraph)
		}
	case "r":
		if s.msgraph != nil && s.msgraph.IsAuthenticated() {
			s.loading = true
			return s, fetchChatsCmd(s.msgraph)
		}
	case "right":
		if chat := s.selectedChat(); chat != nil {
			cmds := s.buildCommandPalette(*chat)
			if len(cmds) > 0 {
				anchorY := 4 + s.cursor
				return s, func() tea.Msg {
					return OpenModalMsg{M: CommandPaletteModal{commands: cmds, AnchorY: anchorY}}
				}
			}
		}
	}
	return s, nil
}

func (s ChatsSection) buildCommandPalette(chat integration.Chat) []CommandEntry {
	var entries []CommandEntry
	if chat.WebURL != "" && s.shell != nil {
		entries = append(entries, CommandEntry{Key: "enter/w", Desc: "open in browser", Cmd: openBrowserCmd(s.shell, chat.WebURL)})
	}
	if s.msgraph != nil && s.msgraph.IsAuthenticated() {
		ms := s.msgraph
		entries = append(entries, CommandEntry{Key: "r", Desc: "refresh chats", Cmd: fetchChatsCmd(ms)})
	}
	return entries
}

func (s ChatsSection) selectedChat() *integration.Chat {
	if s.cursor < 0 || s.cursor >= len(s.Chats) {
		return nil
	}
	c := s.Chats[s.cursor]
	return &c
}

// asTrackerSection creates a temporary TrackerSection so we can reuse its
// notes/templates key handlers without duplicating the logic.
func (s ChatsSection) asTrackerSection() TrackerSection {
	return TrackerSection{
		notes:           s.notes,
		notesFiltered:   s.notesFiltered,
		notesCursor:     s.notesCursor,
		notesSort:       s.notesSort,
		notesFilter:     s.notesFilter,
		notesStore:      s.notesStore,
		templates:       s.templates,
		templatesCursor: s.templatesCursor,
		templatesStore:  s.templatesStore,
	}
}

// applyFromTracker copies notes/templates state back from a temporary TrackerSection.
func (s *ChatsSection) applyFromTracker(ts TrackerSection) {
	s.notes = ts.notes
	s.notesFiltered = ts.notesFiltered
	s.notesCursor = ts.notesCursor
	s.notesSort = ts.notesSort
	s.notesFilter = ts.notesFilter
	s.notesStore = ts.notesStore
	s.templates = ts.templates
	s.templatesCursor = ts.templatesCursor
	s.templatesStore = ts.templatesStore
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View implements tea.Model.
func (s ChatsSection) View() string {
	return s.view(80, 24)
}

func (s ChatsSection) view(width, height int) string {
	topH := height * 55 / 100
	if topH < 4 {
		topH = 4
	}
	botH := height - topH - 3
	if botH < 5 {
		botH = 5
	}

	chatsFocused := s.activePane == ChatsPaneTop
	bottomFocused := s.activePane == ChatsPaneBottom

	top := s.viewChats(width, topH, chatsFocused)
	sep := sectionSepLine(width)
	paneBar := s.viewBottomPaneBar(width)
	paneSep := sectionSepLine(width)

	ts := s.asTrackerSection()
	ts.activePane = s.bottomSubPane
	var bot string
	switch s.bottomSubPane {
	case trackerPaneTemplates:
		bot = ts.viewTemplates(width, botH, bottomFocused)
	default:
		bot = ts.viewNotes(width, botH, bottomFocused)
	}

	return strings.Join([]string{top, sep, paneBar, paneSep, bot}, "\n")
}

func (s ChatsSection) viewChats(width, height int, focused bool) string {
	selRow := selectedRowDimStyle
	if focused {
		selRow = selectedRowStyle
	}

	loadingStr := ""
	if s.loading {
		loadingStr = " ⣾ loading…"
	}

	header := lipgloss.NewStyle().Bold(true).Padding(0, 2).
		Render("Pinned Chats" + loadingStr)

	if s.ShowAuthPlaceholder() {
		placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Padding(0, 2).
			Render("Authentication required — press Enter or 'a' to sign in with Microsoft")
		hint := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).
			Render("Browser will open and the user code will be copied to clipboard")
		nav := lipgloss.NewStyle().Foreground(colorSubtle).Render("  a: authenticate  tab: notes/templates")
		if s.statusMsg != "" {
			return strings.Join([]string{header, "", placeholder, hint, "", nav, "", lipgloss.NewStyle().Foreground(colorGreen).Padding(0, 2).Render(s.statusMsg)}, "\n")
		}
		return strings.Join([]string{header, "", placeholder, hint, "", nav}, "\n")
	}

	if len(s.Chats) == 0 && !s.loading {
		empty := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2).Render("No pinned chats")
		nav := lipgloss.NewStyle().Foreground(colorSubtle).Render("  r: refresh  a: re-authenticate  tab: notes/templates")
		return strings.Join([]string{header, "", empty, "", nav}, "\n")
	}

	typeW := 10
	topicW := width - typeW - 6
	if topicW < 20 {
		topicW = 20
	}

	colH := func(str string, w int) string {
		return columnHeaderStyle.Width(w).Render(truncStr(str, w))
	}
	tableHeader := colH("TYPE", typeW) + " " + colH("TOPIC", topicW)

	maxRows := height - 5
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.cursor, len(s.Chats), maxRows)

	var lines []string
	lines = append(lines, header, tableHeader, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		c := s.Chats[i]
		line := fmt.Sprintf("%-*s %-*s", typeW, truncStr(chatTypeLabel(c.ChatType), typeW), topicW, truncStr(c.Topic, topicW))
		if i == s.cursor {
			lines = append(lines, selRow.Width(width).Render(line))
		} else {
			lines = append(lines, normalRowStyle.Width(width).Render(line))
		}
	}
	lines = append(lines, tableHeaderSepLine(width))
	lines = append(lines, lipgloss.NewStyle().Foreground(colorSubtle).Render(
		"  j/k: navigate  enter/w: open  r: refresh  a: auth  →: commands  tab: notes"))
	if s.statusMsg != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGreen).Padding(0, 2).Render(s.statusMsg))
	}
	return strings.Join(lines, "\n")
}

func chatTypeLabel(t string) string {
	switch t {
	case "oneOnOne":
		return "1:1"
	case "meeting":
		return "Meeting"
	default:
		return "Group"
	}
}

func (s ChatsSection) viewBottomPaneBar(width int) string {
	notesLabel := fmt.Sprintf("Notes (%d)", len(s.notes))
	if s.notesFilter != "" {
		notesLabel += " [" + s.notesFilter + "]"
	}
	tmplLabel := fmt.Sprintf("Templates (%d)", len(s.templates))

	notesTab := tabDimStyle.Render("[o] " + notesLabel)
	tmplTab := tabDimStyle.Render("[t] " + tmplLabel)

	if s.activePane == ChatsPaneBottom {
		switch s.bottomSubPane {
		case trackerPaneTemplates:
			tmplTab = tabActiveStyle.Render("[t] " + tmplLabel)
		default:
			notesTab = tabActiveStyle.Render("[o] " + notesLabel)
		}
	}
	return notesTab + "  " + tmplTab
}
