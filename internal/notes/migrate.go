package notes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// jsonNote mirrors the old single-file JSON schema from ~/.jirlab/notes.json.
type jsonNote struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Category    string        `json:"category"`
	Priority    int           `json:"priority"`
	Description string        `json:"description"`
	Tasks       []jsonTask    `json:"tasks"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type jsonTask struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// LegacyJSONPath returns the path of the old notes.json file.
func LegacyJSONPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".jirlab", "notes.json")
}

// MigrateFromJSON reads the legacy notes.json file into dst and removes it.
// If the file does not exist, MigrateFromJSON is a no-op.
// Existing notes in dst are preserved; only notes whose IDs are not already
// present are imported.
func MigrateFromJSON(dst Store, jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var raw []jsonNote
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for _, jn := range raw {
		if _, err := dst.Get(jn.ID); err == nil {
			continue // already migrated
		}
		tasks := make([]Task, len(jn.Tasks))
		for i, jt := range jn.Tasks {
			tasks[i] = Task{ID: jt.ID, Text: jt.Text, Done: jt.Done}
		}
		n := Note{
			ID:          jn.ID,
			Title:       jn.Title,
			Category:    jn.Category,
			Priority:    Priority(jn.Priority),
			Description: jn.Description,
			Tasks:       tasks,
			CreatedAt:   jn.CreatedAt,
			UpdatedAt:   jn.UpdatedAt,
		}
		if err := dst.Save(n); err != nil {
			return err
		}
	}

	return os.Remove(jsonPath)
}
