//go:build !powershell

package integration

import "os"

func (g *gitCmdService) WriteNavPath(repoPath string) error {
	return os.WriteFile("/tmp/jirlab_nav", []byte(repoPath), 0600)
}
