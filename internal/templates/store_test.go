package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andob/jirlab/internal/templates"
)

// ---------------------------------------------------------------------------
// Encode / Decode
// ---------------------------------------------------------------------------

func TestEncodeDecodeRoundtrip(t *testing.T) {
	now := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	original := templates.Template{
		ID:        "a1b2c3d4",
		Name:      "Bug report",
		Body:      "Steps to reproduce:\n1. Open app\n2. Click button",
		CreatedAt: now,
		UpdatedAt: now,
	}

	data := templates.Encode(original)
	got, err := templates.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	assertEqual(t, "ID", original.ID, got.ID)
	assertEqual(t, "Name", original.Name, got.Name)
	assertEqual(t, "Body", original.Body, got.Body)
	if got.CreatedAt.UnixNano() != original.CreatedAt.UnixNano() {
		t.Errorf("CreatedAt: want %v, got %v", original.CreatedAt, got.CreatedAt)
	}
	if got.UpdatedAt.UnixNano() != original.UpdatedAt.UnixNano() {
		t.Errorf("UpdatedAt: want %v, got %v", original.UpdatedAt, got.UpdatedAt)
	}
}

func TestEncodeDecodeEmptyBody(t *testing.T) {
	tmpl := templates.Template{
		ID:        "aabbccdd",
		Name:      "Empty body",
		Body:      "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	got, err := templates.Decode(templates.Encode(tmpl))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Body != "" {
		t.Errorf("expected empty body, got %q", got.Body)
	}
}

func TestEncodeDecodeMultilineBody(t *testing.T) {
	tmpl := templates.Template{
		ID:        "11223344",
		Name:      "Multi",
		Body:      "\nfoo\n\nbar\n",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	got, err := templates.Decode(templates.Encode(tmpl))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	assertEqual(t, "Body", tmpl.Body, got.Body)
}

func TestEncodeDecodeCRLF(t *testing.T) {
	tmpl := templates.Template{
		ID:        "deadbeef",
		Name:      "CRLF",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	raw := templates.Encode(tmpl)
	crlf := []byte(strings.ReplaceAll(string(raw), "\n", "\r\n"))
	got, err := templates.Decode(crlf)
	if err != nil {
		t.Fatalf("Decode CRLF: %v", err)
	}
	assertEqual(t, "ID", tmpl.ID, got.ID)
}

func TestDecodeMissingID(t *testing.T) {
	data := []byte("name=Orphan\ncreated=0\nupdated=0\n---\n")
	_, err := templates.Decode(data)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestDecodeLongBody(t *testing.T) {
	longBody := strings.Repeat("a", 2000)
	tmpl := templates.Template{
		ID:        "cafebabe",
		Name:      "Long",
		Body:      longBody,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	got, err := templates.Decode(templates.Encode(tmpl))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	assertEqual(t, "Body", longBody, got.Body)
}

// ---------------------------------------------------------------------------
// Store CRUD
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) templates.Store {
	t.Helper()
	return templates.NewFilesystemStore(t.TempDir())
}

func makeTemplate(id, name, body string) templates.Template {
	now := time.Now()
	return templates.Template{
		ID:        id,
		Name:      name,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestStoreCreate(t *testing.T) {
	s := newTestStore(t)
	tmpl := makeTemplate("a1b2c3d4", "Hello", "Body text")
	if err := s.Save(tmpl); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get("a1b2c3d4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertEqual(t, "Name", "Hello", got.Name)
	assertEqual(t, "Body", "Body text", got.Body)
}

func TestStoreUpdate(t *testing.T) {
	s := newTestStore(t)
	tmpl := makeTemplate("a1b2c3d4", "Original", "old body")
	_ = s.Save(tmpl)

	tmpl.Name = "Updated"
	tmpl.Body = "new body"
	if err := s.Save(tmpl); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := s.Get("a1b2c3d4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertEqual(t, "Name", "Updated", got.Name)
	assertEqual(t, "Body", "new body", got.Body)
}

func TestStoreList(t *testing.T) {
	s := newTestStore(t)

	older := templates.Template{
		ID:        "00000001",
		Name:      "Older",
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now(),
	}
	newer := templates.Template{
		ID:        "00000002",
		Name:      "Newer",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = s.Save(older)
	_ = s.Save(newer)

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(list))
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
		t.Errorf("expected empty list, got %d templates", len(list))
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)
	tmpl := makeTemplate("a1b2c3d4", "To delete", "body")
	_ = s.Save(tmpl)

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
	s := templates.NewFilesystemStore(dir)
	tmpl := makeTemplate("a1b2c3d4", "File check", "body")
	_ = s.Save(tmpl)

	path := filepath.Join(dir, "a1b2c3d4.tmpl")
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
		t.Errorf("deleting non-existent template should not error, got: %v", err)
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("a1b2c3d4")
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestStoreInvalidID(t *testing.T) {
	s := newTestStore(t)
	tmpl := makeTemplate("../../etc/passwd", "Injection", "body")
	if err := s.Save(tmpl); err == nil {
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
// helpers
// ---------------------------------------------------------------------------

func assertEqual(t *testing.T, label, want, got string) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %q, got %q", label, want, got)
	}
}
