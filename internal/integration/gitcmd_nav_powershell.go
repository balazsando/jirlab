//go:build powershell

package integration

import (
	"os"
	"path/filepath"
)

func (g *gitCmdService) WriteNavPath(repoPath string) error {
	tmp := os.Getenv("TEMP")
	if tmp == "" {
		tmp = os.TempDir()
	}
	navFile := filepath.Join(tmp, "jirlab_nav.txt")
	return os.WriteFile(navFile, []byte(repoPath), 0600)
}
