package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/andob/jirlab/internal/config"
	"github.com/andob/jirlab/internal/debug"
	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

var (
	// shared across all subcommands
	logger       *zap.Logger
	cfg          *config.Config
	jiraSvc      integration.JiraService
	gitlabSvc    integration.GitLabService
	gitlabClient service.GitLabClient
	debugMode    bool
)

// rootCmd is the Cobra root command.
var rootCmd = &cobra.Command{
	Use:   "jirlab",
	Short: "Terminal UI for Jira + GitLab",
	Long: `jirlab is a terminal user interface for managing Jira issues
and creating GitLab branches, all from your terminal.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error

		// Bootstrap logger (debug mode writes to file so it doesn't pollute the TUI)
		if debugMode {
			logger, err = zap.NewDevelopment()
			if initErr := debug.Init("/tmp/jirlab-debug.log"); initErr != nil {
				fmt.Fprintf(os.Stderr, "warn: could not open debug log: %v\n", initErr)
			}
		} else {
			logger, err = zap.NewProduction()
		}
		if err != nil {
			return fmt.Errorf("init logger: %w", err)
		}

		// Load config
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Wire service layer
		jiraClient := service.NewJiraClient(cfg.JiraURL, cfg.JiraAPIURL, cfg.JiraEmail, cfg.JiraToken)
		gitlabClient = service.NewGitLabClient(cfg.GitLabAPIURL, cfg.GitLabToken)

		jiraSvc = integration.NewJiraService(jiraClient)
		gitlabSvc = integration.NewGitLabService(gitlabClient)

		return nil
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false,
		"Enable debug mode: verbose logging written to /tmp/jirlab-debug.log")

	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(issueCmd)
	rootCmd.AddCommand(gitCmd)
}
