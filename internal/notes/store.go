package notes

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store persists and retrieves notes on the filesystem.
// Each note is stored as a separate file: <baseDir>/<id>.note
type Store interface {
	// Save creates or overwrites a note file.
	Save(n Note) error
	// Get retrieves a note by ID.
	Get(id string) (Note, error)
	// List returns all notes, newest first.
	List() ([]Note, error)
	// Delete removes the note file for the given ID.
	Delete(id string) error
}

type filesystemStore struct {
	baseDir string
}

// NewFilesystemStore returns a Store that persists notes under baseDir.
// baseDir is created on first Save if it does not exist.
func NewFilesystemStore(baseDir string) Store {
	return &filesystemStore{baseDir: baseDir}
}

// DefaultStore returns a Store rooted at the OS user cache directory.
// Path: <UserCacheDir>/jirlab/notes/
func DefaultStore() (Store, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("notes: resolve cache dir: %w", err)
	}
	return NewFilesystemStore(filepath.Join(cache, "jirlab", "notes")), nil
}

// NewID generates a collision-resistant 8-character hex ID.
func NewID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *filesystemStore) path(id string) string {
	return filepath.Join(s.baseDir, id+".note")
}

func (s *filesystemStore) Save(n Note) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("notes: mkdir: %w", err)
	}
	if !validID(n.ID) {
		return fmt.Errorf("notes: invalid id %q", n.ID)
	}
	return os.WriteFile(s.path(n.ID), Encode(n), 0o644)
}

func (s *filesystemStore) Get(id string) (Note, error) {
	if !validID(id) {
		return Note{}, fmt.Errorf("notes: invalid id %q", id)
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Note{}, fmt.Errorf("notes: get %q: %w", id, err)
	}
	return Decode(data)
}

func (s *filesystemStore) List() ([]Note, error) {
	entries, err := os.ReadDir(s.baseDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notes: list: %w", err)
	}

	var out []Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".note") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".note")
		if !validID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.baseDir, e.Name()))
		if err != nil {
			continue // skip unreadable files
		}
		n, err := Decode(data)
		if err != nil {
			continue // skip corrupt files
		}
		out = append(out, n)
	}

	// newest first
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *filesystemStore) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("notes: invalid id %q", id)
	}
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil // already gone
	}
	return err
}

// validID accepts only lowercase hex strings (8 or 19 digits for UnixNano IDs).
// Also accepts the UnixNano string IDs used by the existing TUI.
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
