package integration

import (
	"os"
	"path/filepath"
)

// FilesystemService abstracts file I/O operations used by the TUI layer,
// allowing them to be replaced with test doubles in unit tests.
type FilesystemService interface {
	// SaveFile writes data to path, creating any missing parent directories.
	SaveFile(path string, data []byte) error
	// PathExists returns true when path exists on the filesystem.
	PathExists(path string) bool
}

type filesystemService struct{}

// NewFilesystemService returns the production FilesystemService implementation.
func NewFilesystemService() FilesystemService { return filesystemService{} }

func (filesystemService) SaveFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (filesystemService) PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
