package background

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// RefreshMsg is delivered to the root model on every background tick.
// Generation lets the root model ignore stale ticks after an 0 reset.
type RefreshMsg struct {
	Generation uint64
}

// Worker returns a one-shot tea.Cmd that fires a RefreshMsg after interval.
// Use tea.Tick (not tea.Every) so the caller can restart with a new generation
// when the user manually refreshes (0), effectively resetting the timer.
func Worker(interval time.Duration, generation uint64) tea.Cmd {
	return tea.Tick(interval, func(_ time.Time) tea.Msg {
		return RefreshMsg{Generation: generation}
	})
}
