package notes_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andob/jirlab/internal/notes"
)

// ---------------------------------------------------------------------------
// Encode / Decode
// ---------------------------------------------------------------------------

func TestEncodeDecodeRoundtrip(t *testing.T) {
	now := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	original := notes.Note{
		ID:          "a1b2c3d4",
		Title:       "Test note",
		Category:    "work",
		Priority:    notes.PriorityHigh,
		Description: "Line one\nLine two",
		Tasks: []notes.Task{
			{ID: "t1", Text: "Buy groceries", Done: false},
			{ID: "t2", Text: "Do: laundry", Done: true},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data := notes.Encode(original)
	got, err := notes.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	assertEqual(t, "ID", original.ID, got.ID)
	assertEqual(t, "Title", original.Title, got.Title)
	assertEqual(t, "Category", original.Category, got.Category)
	if got.Priority != original.Priority {
		t.Errorf("Priority: want %d, got %d", original.Priority, got.Priority)
	}
	assertEqual(t, "Description", original.Description, got.Description)
	if got.CreatedAt.UnixNano() != original.CreatedAt.UnixNano() {
		t.Errorf("CreatedAt: want %v, got %v", original.CreatedAt, got.CreatedAt)
	}
	if len(got.Tasks) != len(original.Tasks) {
		t.Fatalf("Tasks len: want %d, got %d", len(original.Tasks), len(got.Tasks))
	}
	for i, task := range original.Tasks {
		assertEqual(t, fmt.Sprintf("task[%d].ID", i), task.ID, got.Tasks[i].ID)
		assertEqual(t, fmt.Sprintf("task[%d].Text", i), task.Text, got.Tasks[i].Text)
		if task.Done != got.Tasks[i].Done {
			t.Errorf("task[%d].Done: want %v, got %v", i, task.Done, got.Tasks[i].Done)
		}
	}
}

func TestDecodeEmptyDescription(t *testing.T) {
	n := notes.Note{
		ID:        "aabbccdd",
		Title:     "No desc",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	got, err := notes.Decode(notes.Encode(n))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Description != "" {
		t.Errorf("expected empty description, got %q", got.Description)
	}
}

func TestDecodeBlankBodyLines(t *testing.T) {
	n := notes.Note{
		ID:          "11223344",
		Title:       "Blank lines",
		Description: "\nfoo\n\nbar\n",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	got, err := notes.Decode(notes.Encode(n))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	assertEqual(t, "Description", n.Description, got.Description)
}

func TestDecodeCRLF(t *testing.T) {
	n := notes.Note{
		ID:        "deadbeef",
		Title:     "CRLF",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	raw := notes.Encode(n)
	crlf := []byte(strings.ReplaceAll(string(raw), "\n", "\r\n"))
	got, err := notes.Decode(crlf)
	if err != nil {
		t.Fatalf("Decode CRLF: %v", err)
	}
	assertEqual(t, "ID", n.ID, got.ID)
}

func TestDecodeMissingID(t *testing.T) {
	data := []byte("title=Orphan\ncreated=0\nupdated=0\n---\n")
	_, err := notes.Decode(data)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestDecodeTaskTextWithColons(t *testing.T) {
	n := notes.Note{
		ID:    "cafebabe",
		Title: "Colons",
		Tasks: []notes.Task{
			{ID: "t1", Text: "http://example.com:8080/path", Done: false},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	got, err := notes.Decode(notes.Encode(n))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got.Tasks))
	}
	assertEqual(t, "task text", n.Tasks[0].Text, got.Tasks[0].Text)
}

// ---------------------------------------------------------------------------
// Store CRUD
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) notes.Store {
	t.Helper()
	return notes.NewFilesystemStore(t.TempDir())
}

func makeNote(id, title string) notes.Note {
	now := time.Now()
	return notes.Note{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestStoreCreate(t *testing.T) {
	s := newTestStore(t)
	n := makeNote("a1b2c3d4", "Hello")
	if err := s.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get("a1b2c3d4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertEqual(t, "Title", "Hello", got.Title)
}

func TestStoreUpdate(t *testing.T) {
	s := newTestStore(t)
	n := makeNote("a1b2c3d4", "Original")
	_ = s.Save(n)

	n.Title = "Updated"
	if err := s.Save(n); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := s.Get("a1b2c3d4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertEqual(t, "Title", "Updated", got.Title)
}

func TestStoreList(t *testing.T) {
	s := newTestStore(t)

	older := notes.Note{ID: "00000001", Title: "Older", CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now()}
	newer := notes.Note{ID: "00000002", Title: "Newer", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = s.Save(older)
	_ = s.Save(newer)

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(list))
	}
	// newest first
	assertEqual(t, "first ID", newer.ID, list[0].ID)
	assertEqual(t, "second ID", older.ID, list[1].ID)
}

func TestStoreListEmpty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d notes", len(list))
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)
	n := makeNote("a1b2c3d4", "To delete")
	_ = s.Save(n)

	if err := s.Delete("a1b2c3d4"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get("a1b2c3d4")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestStoreDeleteFileRemoved(t *testing.T) {
	dir := t.TempDir()
	s := notes.NewFilesystemStore(dir)
	n := makeNote("a1b2c3d4", "File check")
	_ = s.Save(n)

	path := filepath.Join(dir, "a1b2c3d4.note")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	_ = s.Delete("a1b2c3d4")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should be removed after delete")
	}
}

func TestStoreDeleteNonExistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete("a1b2c3d4"); err != nil {
		t.Errorf("deleting non-existent note should not error, got: %v", err)
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("a1b2c3d4")
	if err == nil {
		t.Fatal("expected error for missing note")
	}
}

func TestStoreInvalidID(t *testing.T) {
	s := newTestStore(t)
	n := makeNote("../../etc/passwd", "Injection")
	if err := s.Save(n); err == nil {
		t.Fatal("expected error for path-traversal ID")
	}
	if err := s.Delete("../../etc/passwd"); err == nil {
		t.Fatal("expected error for path-traversal ID in Delete")
	}
	if _, err := s.Get("../../etc/passwd"); err == nil {
		t.Fatal("expected error for path-traversal ID in Get")
	}
}

// ---------------------------------------------------------------------------
// Migration
// ---------------------------------------------------------------------------

func TestMigrateFromJSON(t *testing.T) {
	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "notes.json")

	// Write a minimal legacy JSON file
	legacy := `[
  {
    "id": "1714262400000000000",
    "title": "Legacy note",
    "category": "test",
    "priority": 1,
    "description": "Old format",
    "tasks": [{"id":"t1","text":"Task A","done":false}],
    "created_at": "2024-04-28T00:00:00Z",
    "updated_at": "2024-04-28T00:00:00Z"
  }
]`
	if err := os.WriteFile(jsonFile, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := notes.NewFilesystemStore(filepath.Join(dir, "notes"))
	if err := notes.MigrateFromJSON(dst, jsonFile); err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}

	// JSON file should be removed
	if _, err := os.Stat(jsonFile); !os.IsNotExist(err) {
		t.Error("legacy JSON file should be deleted after migration")
	}

	// Note should be accessible in the new store
	n, err := dst.Get("1714262400000000000")
	if err != nil {
		t.Fatalf("Get migrated note: %v", err)
	}
	assertEqual(t, "Title", "Legacy note", n.Title)
	assertEqual(t, "Category", "test", n.Category)
	if len(n.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(n.Tasks))
	}
	assertEqual(t, "task text", "Task A", n.Tasks[0].Text)
}

func TestMigrateFromJSONNoFile(t *testing.T) {
	dst := newTestStore(t)
	if err := notes.MigrateFromJSON(dst, "/nonexistent/path/notes.json"); err != nil {
		t.Errorf("MigrateFromJSON with missing file should be no-op, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertEqual(t *testing.T, label, want, got string) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %q, got %q", label, want, got)
	}
}
