package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andob/jirlab/internal/notes"
)

// noteCmd groups all note subcommands.
var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage local notes",
	// Override root PersistentPreRunE so Jira/GitLab config is not required.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
}

var noteAddCmd = &cobra.Command{
	Use:   "add <title> [body]",
	Short: "Create a new note",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := notes.DefaultStore()
		if err != nil {
			return err
		}

		id, err := notes.NewID()
		if err != nil {
			return fmt.Errorf("generate id: %w", err)
		}

		now := time.Now()
		body := ""
		if len(args) == 2 {
			body = args[1]
		}

		category, _ := cmd.Flags().GetString("category")
		prioVal, _ := cmd.Flags().GetInt("priority")

		n := notes.Note{
			ID:          id,
			Title:       args[0],
			Category:    category,
			Priority:    notes.Priority(prioVal),
			Description: body,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := store.Save(n); err != nil {
			return err
		}
		fmt.Printf("Created note %s\n", id)
		return nil
	},
}

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all notes (newest first)",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := notes.DefaultStore()
		if err != nil {
			return err
		}
		list, err := store.List()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("No notes found.")
			return nil
		}
		for _, n := range list {
			tasks := fmt.Sprintf("%d tasks", len(n.Tasks))
			cat := n.Category
			if cat == "" {
				cat = "-"
			}
			fmt.Printf("%-10s  %-8s  %-10s  %-8s  %s\n",
				n.ID,
				n.Priority.String(),
				cat,
				tasks,
				n.Title,
			)
		}
		return nil
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Print a note's full content",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := notes.DefaultStore()
		if err != nil {
			return err
		}
		n, err := store.Get(args[0])
		if err != nil {
			return fmt.Errorf("note %q not found", args[0])
		}

		fmt.Printf("ID:       %s\n", n.ID)
		fmt.Printf("Title:    %s\n", n.Title)
		if n.Category != "" {
			fmt.Printf("Category: %s\n", n.Category)
		}
		fmt.Printf("Priority: %s\n", n.Priority.String())
		fmt.Printf("Created:  %s\n", n.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Updated:  %s\n", n.UpdatedAt.Format(time.RFC3339))
		if len(n.Tasks) > 0 {
			fmt.Println("Tasks:")
			for _, t := range n.Tasks {
				check := "[ ]"
				if t.Done {
					check = "[x]"
				}
				fmt.Printf("  %s %s\n", check, t.Text)
			}
		}
		if n.Description != "" {
			fmt.Println("---")
			fmt.Println(strings.TrimRight(n.Description, "\n"))
		}
		return nil
	},
}

var noteDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a note and remove its file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := notes.DefaultStore()
		if err != nil {
			return err
		}
		id := args[0]
		// Verify it exists before deleting so we can give a clear error.
		if _, err := store.Get(id); err != nil {
			return fmt.Errorf("note %q not found", id)
		}
		if err := store.Delete(id); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Deleted note %s\n", id)
		return nil
	},
}

func init() {
	noteAddCmd.Flags().StringP("category", "c", "", "Note category (e.g. work, personal)")
	noteAddCmd.Flags().IntP("priority", "p", 0, "Priority: 0=Low 1=Med 2=High")

	noteCmd.AddCommand(noteAddCmd)
	noteCmd.AddCommand(noteListCmd)
	noteCmd.AddCommand(noteShowCmd)
	noteCmd.AddCommand(noteDeleteCmd)
}
