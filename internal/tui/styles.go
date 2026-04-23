package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// --- Colour palette ---
	colorActive   = lipgloss.Color("62")  // purple — active tab / highlight
	colorMuted    = lipgloss.Color("240") // grey
	colorBorder   = lipgloss.Color("238")
	colorStatusBg = lipgloss.Color("236")
	colorFg       = lipgloss.Color("252")
	colorHeader   = lipgloss.Color("255")

	// Issue / MR state colours
	colorWhite     = lipgloss.Color("252")
	colorDarkBlue  = lipgloss.Color("75") // brighter blue – readable on purple bg
	colorLightBlue = lipgloss.Color("117")
	colorBlue      = lipgloss.Color("33")
	colorSubtle    = lipgloss.Color("243")
	colorYellow    = lipgloss.Color("220")
	colorGreen     = lipgloss.Color("76")
	colorOrange    = lipgloss.Color("214")
	colorRed       = lipgloss.Color("196")
	colorPink      = lipgloss.Color("213") // new comment in last 24 h

	// Row background tints – subtle so foreground text stays readable.
	colorAssignedBg = lipgloss.Color("17") // dark navy   – assigned to me (board)
	colorActiveBg   = lipgloss.Color("22") // dark green  – non-default branch (repos)
	colorWarningBg  = lipgloss.Color("52") // dark maroon – non-default + no sprint issue
	colorOwnedMRBg  = lipgloss.Color("18") // dark blue   – MR owned by me

	// Selection backgrounds — vivid purple for focused, dark grey for unfocused.
	colorSelVivid = lipgloss.Color("62")  // vivid purple – focused selection
	colorSelDim   = lipgloss.Color("237") // dark grey    – unfocused selection
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
			Foreground(colorMuted).
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
