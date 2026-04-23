package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andob/jirlab/internal/actions"
)

// issueCmd is the parent for issue sub-commands.
var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage Jira issues from the CLI (non-TUI)",
}

var issueTransitionCmd = &cobra.Command{
	Use:   "transition <issue-key> <status>",
	Short: "Move an issue to the given status",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		targetStatus := args[1]

		if err := actions.TransitionIssue(jiraSvc, issueKey, targetStatus); err != nil {
			return err
		}
		fmt.Printf("Moved %s → %s\n", issueKey, targetStatus)
		return nil
	},
}

var issuePickupCmd = &cobra.Command{
	Use:   "pickup <issue-key>",
	Short: "Pickup an issue (move to In Progress)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		if err := actions.PickupIssue(jiraSvc, issueKey); err != nil {
			return err
		}
		fmt.Printf("Picked up %s → In Progress\n", issueKey)
		return nil
	},
}

var issueCommentCmd = &cobra.Command{
	Use:   "comment <issue-key> <body>",
	Short: "Add a comment to an issue",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		body := args[1]
		if err := jiraSvc.AddComment(issueKey, body); err != nil {
			return err
		}
		fmt.Printf("Comment added to %s\n", issueKey)
		return nil
	},
}

func init() {
	issueCmd.AddCommand(issueTransitionCmd)
	issueCmd.AddCommand(issuePickupCmd)
	issueCmd.AddCommand(issueCommentCmd)
}
