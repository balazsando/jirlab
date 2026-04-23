package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/andob/jirlab/internal/tui"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Open the interactive Jira board TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		model := tui.NewAppModel(jiraSvc, gitlabSvc, gitlabClient, cfg.JiraBoardID, cfg.JiraMyAccountID, cfg.GitLabAPIURL, cfg.JiraURL, debugMode)

		p := tea.NewProgram(
			model,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			return err
		}
		return nil
	},
}
