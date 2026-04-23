package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// truncStr truncates s to max runes, appending "…" when it exceeds the limit.
func truncStr(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// openBrowserCmd launches a URL in the system browser non-blocking.
func openBrowserCmd(shell integration.ShellService, url string) tea.Cmd {
	return func() tea.Msg {
		_ = shell.OpenBrowser(url)
		return nil
	}
}

// extractIssueKey pulls the first Jira-style issue key from a branch name.
// Returns an uppercase key (e.g. "PROJ-123") or an empty string.
func extractIssueKey(branch string) string {
	if m := service.IssueKeyRe.FindStringSubmatch(branch); len(m) >= 2 {
		return strings.ToUpper(m[1])
	}
	return ""
}

// wordWrap hard-wraps text so no line exceeds width characters.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	var lines []string
	line := ""
	for _, w := range words {
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// copyToClipboard returns a Cmd that writes text to the system clipboard.
func copyToClipboard(shell integration.ShellService, text string) tea.Cmd {
	return func() tea.Msg {
		_ = shell.CopyToClipboard(text)
		return clipboardCopiedMsg{text: text}
	}
}

// clipboardCopiedMsg is returned after a clipboard copy attempt.
type clipboardCopiedMsg struct{ text string }

// overlayCenter renders a modal string centred over the background string,
// keeping the underlying background content visible around the modal box.
// It falls back to simple lipgloss.Place when dimensions are unavailable.
func overlayCenter(bg, fg string, w, h int) string {
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	fgH := len(fgLines)
	fgW := 0
	for _, l := range fgLines {
		if lw := lipgloss.Width(l); lw > fgW {
			fgW = lw
		}
	}

	startRow := (h - fgH) / 2
	startCol := (w - fgW) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	out := make([]string, h)
	for i := range out {
		if i < len(bgLines) {
			out[i] = bgLines[i]
		}
	}

	for fi, fgLine := range fgLines {
		ri := startRow + fi
		if ri >= h {
			break
		}
		bgLine := ""
		if ri < len(bgLines) {
			bgLine = bgLines[ri]
		}
		out[ri] = overlayLine(bgLine, fgLine, startCol, w)
	}

	return strings.Join(out, "\n")
}

// splitAtVisualCol splits s at the given visible-column position, preserving
// ANSI escape sequences verbatim. Returns (left, right) where left contains
// the first col visible characters (plus any ANSI codes they carry) and right
// is the remainder.
func splitAtVisualCol(s string, col int) (left, right string) {
	vis := 0
	i := 0
	for i < len(s) {
		if vis >= col {
			break
		}
		if s[i] == '\x1b' {
			// Collect ESC sequence up to and including the final byte.
			j := i + 1
			for j < len(s) && s[j] != 'm' && s[j] != 'K' && s[j] != 'J' && s[j] != 'H' && s[j] != 'A' && s[j] != 'B' && s[j] != 'C' && s[j] != 'D' {
				j++
			}
			if j < len(s) {
				j++ // include the terminator
			}
			i = j
			continue
		}
		// Ordinary rune — counts as 1 visible column.
		_, size := []rune(s[i:])[0], len(string([]rune(s[i:])[0:1]))
		vis++
		i += size
	}
	return s[:i], s[i:]
}

// leadingANSI returns the leading ANSI escape sequences in s (i.e., all ESC
// codes that appear before the first visible character). These are re-emitted
// after the overlay foreground so the right portion of the background row
// retains its original color.
func leadingANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			j := i + 1
			for j < len(s) && s[j] != 'm' && s[j] != 'K' && s[j] != 'J' && s[j] != 'H' && s[j] != 'A' && s[j] != 'B' && s[j] != 'C' && s[j] != 'D' {
				j++
			}
			if j < len(s) {
				j++
			}
			out.WriteString(s[i:j])
			i = j
			continue
		}
		// First visible character — stop collecting.
		break
	}
	return out.String()
}

// overlayLine places fgLine on top of bgLine at the given column offset,
// preserving the ANSI color state of bgLine so background highlights survive.
func overlayLine(bgLine, fgLine string, col, termW int) string {
	// Split bg at the column where fg starts.
	left, rest := splitAtVisualCol(bgLine, col)

	// Pad left portion if bg is shorter than col.
	leftVis := lipgloss.Width(left)
	if leftVis < col {
		left += strings.Repeat(" ", col-leftVis)
	}

	// Skip the visible columns covered by fg in the right remainder.
	fgW := lipgloss.Width(fgLine)
	_, right := splitAtVisualCol(rest, fgW)

	// Reset after fg, then restore the bg row's leading color state so right
	// portion renders with the correct color (handles uniform-color rows).
	combined := left + fgLine + "\x1b[0m" + leadingANSI(bgLine) + right

	// Pad to terminal width.
	visW := lipgloss.Width(combined)
	if visW < termW {
		combined += strings.Repeat(" ", termW-visW)
	}
	return combined
}

// stripANSI removes ANSI escape sequences from s returning plain text.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// overlayHCenterAt renders fg horizontally centered on the screen and
// vertically near anchorY (the screen row of the selected table entry).
// When anchorY is in the upper half the palette appears below it; in the
// lower half it appears above it. Used exclusively for the command palette.
func overlayHCenterAt(bg, fg string, anchorY, w, h int) string {
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	fgH := len(fgLines)
	fgW := 0
	for _, l := range fgLines {
		if lw := lipgloss.Width(l); lw > fgW {
			fgW = lw
		}
	}

	startCol := (w - fgW) / 2
	if startCol < 0 {
		startCol = 0
	}

	var startRow int
	if anchorY < h/2 {
		startRow = anchorY + 1
	} else {
		startRow = anchorY - fgH
	}
	if startRow < 0 {
		startRow = 0
	}
	if startRow+fgH > h {
		startRow = h - fgH
	}
	if startRow < 0 {
		startRow = 0
	}

	out := make([]string, h)
	for i := range out {
		if i < len(bgLines) {
			out[i] = bgLines[i]
		}
	}
	for fi, fgLine := range fgLines {
		ri := startRow + fi
		if ri >= h {
			break
		}
		bgLine := ""
		if ri < len(bgLines) {
			bgLine = bgLines[ri]
		}
		out[ri] = overlayLine(bgLine, fgLine, startCol, w)
	}
	return strings.Join(out, "\n")
}

// subPanePlaceholder returns a styled string for loading/error/empty subtab
// states. Returns "" when the caller should proceed to render the table.
// loading suppresses render only when items == 0 so stale data stays visible
// during background refresh.
func subPanePlaceholder(loading bool, items int, loadMsg, errStr, emptyMsg string) string {
	if loading && items == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("  ↻ " + loadMsg)
	}
	if errStr != "" {
		return lipgloss.NewStyle().Foreground(colorRed).Render("  ✗ " + errStr)
	}
	if items == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("  " + emptyMsg)
	}
	return ""
}
