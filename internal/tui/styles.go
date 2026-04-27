package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// --- Catppuccin Mocha colour palette ---
	colorActive   = lipgloss.Color("#cba6f7") // Mauve  — active tab / highlight / borders
	colorMuted    = lipgloss.Color("#585b70") // Overlay0 — dim text, hints
	colorBorder   = lipgloss.Color("#45475a") // Surface1 — separator lines
	colorStatusBg = lipgloss.Color("#313244") // Surface0 — status bar background
	colorFg       = lipgloss.Color("#bac2de") // Subtext1 — normal row text
	colorHeader   = lipgloss.Color("#cdd6f4") // Text      — bold headers

	// Issue / MR state colours
	colorWhite     = lipgloss.Color("#cdd6f4") // Text
	colorDarkBlue  = lipgloss.Color("#b4befe") // Lavender
	colorLightBlue = lipgloss.Color("#89dceb") // Sky
	colorBlue      = lipgloss.Color("#89b4fa") // Blue
	colorSubtle    = lipgloss.Color("#6c7086") // Overlay1
	colorYellow    = lipgloss.Color("#f9e2af") // Yellow
	colorGreen     = lipgloss.Color("#a6e3a1") // Green
	colorOrange    = lipgloss.Color("#fab387") // Peach
	colorRed       = lipgloss.Color("#f38ba8") // Red
	colorPink      = lipgloss.Color("#f5c2e7") // Pink — new comment in last 24 h

	// Row background tints — subtle so foreground text stays readable.
	colorAssignedBg = lipgloss.Color("#1e1e2e") // Base      — assigned to me (board)
	colorActiveBg   = lipgloss.Color("#1e3a2f") // dark green — non-default branch (repos)
	colorWarningBg  = lipgloss.Color("#3b1929") // dark red  — non-default + no sprint issue
	colorOwnedMRBg  = lipgloss.Color("#1e2657") // dark blue — MR owned by me

	// Selection backgrounds — dark mauve for focused (clearly distinct), Surface0 for unfocused.
	colorSelVivid = lipgloss.Color("#3d2452") // dark mauve — focused row / active tab
	colorSelDim   = lipgloss.Color("#313244") // Surface0   — unfocused row
)

// --- Tab bar ---

var (
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			Background(colorSelVivid).
			Padding(0, 2)

	tabDimStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			Background(colorSelDim).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 2)
)

// --- Layout ---

var (
	statusBarStyle = lipgloss.NewStyle().
			Background(colorStatusBg).
			Foreground(colorFg).
			PaddingLeft(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			PaddingLeft(1)

	sectionBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)
)

// --- Table rows ---

var (
	// selectedRowStyle: vivid bg, no forced foreground so status text colour shows through.
	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Background(colorSelVivid)

	normalRowStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	// selectedRowDimStyle: dim bg for unfocused pane cursor, status text colour shows through.
	selectedRowDimStyle = lipgloss.NewStyle().
				Background(colorSelDim)

	// columnHeaderStyle renders a table column header (bold, bright blue like tracker).
	columnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorBlue)
)

// sectionSepLine returns a full-width solid separator line (─ repeated) used
// between a tab bar and its content table.
func sectionSepLine(width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", width))
}

// tableHeaderSepLine returns a full-width dashed separator used between a
// table's column header row and its body rows.
func tableHeaderSepLine(width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("-", width))
}


// --- Modals ---

var (
	modalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorActive).
			Padding(1, 3)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			MarginBottom(1)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	// paletteBoxStyle is the minimal border used for the command palette.
	// No background so terminal default color shows through — avoids color
	// inconsistency around input edges and inside the box.
	paletteBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorActive).
			Padding(0, 1)
)
