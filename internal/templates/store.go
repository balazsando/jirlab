package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store persists and retrieves templates on the filesystem.
// Each template is stored as a separate file: <baseDir>/<id>.tmpl
type Store interface {
	// Save creates or overwrites a template file.
	Save(t Template) error
	// Get retrieves a template by ID.
	Get(id string) (Template, error)
	// List returns all templates, newest first.
	List() ([]Template, error)
	// Delete removes the template file for the given ID.
	Delete(id string) error
}

type filesystemStore struct {
	baseDir string
}

// NewFilesystemStore returns a Store that persists templates under baseDir.
// baseDir is created on first Save if it does not exist.
func NewFilesystemStore(baseDir string) Store {
	return &filesystemStore{baseDir: baseDir}
}

// DefaultStore returns a Store rooted at the OS user cache directory.
// Path: <UserCacheDir>/jirlab/templates/
func DefaultStore() (Store, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("templates: resolve cache dir: %w", err)
	}
	return NewFilesystemStore(filepath.Join(cache, "jirlab", "templates")), nil
}

func (s *filesystemStore) path(id string) string {
	return filepath.Join(s.baseDir, id+".tmpl")
}

func (s *filesystemStore) Save(t Template) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("templates: mkdir: %w", err)
	}
	if !validID(t.ID) {
		return fmt.Errorf("templates: invalid id %q", t.ID)
	}
	return os.WriteFile(s.path(t.ID), Encode(t), 0o644)
}

func (s *filesystemStore) Get(id string) (Template, error) {
	if !validID(id) {
		return Template{}, fmt.Errorf("templates: invalid id %q", id)
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Template{}, fmt.Errorf("templates: get %q: %w", id, err)
	}
	return Decode(data)
}

func (s *filesystemStore) List() ([]Template, error) {
	entries, err := os.ReadDir(s.baseDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("templates: list: %w", err)
	}

	var out []Template
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tmpl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".tmpl")
		if !validID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.baseDir, e.Name()))
		if err != nil {
			continue // skip unreadable files
		}
		t, err := Decode(data)
		if err != nil {
			continue // skip corrupt files
		}
		out = append(out, t)
	}

	// newest first
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *filesystemStore) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("templates: invalid id %q", id)
	}
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil // already gone
	}
	return err
}

// validID accepts only lowercase hex strings (and dashes for legacy IDs).
func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
			return false
		}
	}
	return true
}
