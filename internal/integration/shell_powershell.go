//go:build powershell

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// intellijDirRe matches "IntelliJ IDEA <version>" subdirectory names under the JetBrains install dir.
var intellijDirRe = regexp.MustCompile(`^IntelliJ IDEA [\d.]+$`)

// NewShellService returns the PowerShell/Windows implementation of ShellService.
func NewShellService() ShellService {
	return &powerShellService{}
}

// --- PowerShell / Windows implementation ---

type powerShellService struct{}

func (s *powerShellService) OpenBrowser(url string) error {
	// explorer.exe can open URLs on Windows/PowerShell environments.
	return exec.Command("explorer.exe", url).Start() //nolint:gosec
}

func (s *powerShellService) CopyToClipboard(text string) error {
	// clip.exe is available on all modern Windows versions.
	cmd := exec.Command("clip.exe")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run() // best-effort
	return nil
}

func (s *powerShellService) ResolveEditorPath(editor string) (string, error) {
	switch editor {
	case "idea":
		// Look in the standard installation directory:
		//   C:\Program Files\JetBrains\IntelliJ IDEA <version>\bin\idea64.exe
		jetbrainsDir := filepath.Join(os.Getenv("PROGRAMFILES"), "JetBrains")
		entries, err := os.ReadDir(jetbrainsDir)
		if err == nil {
			var candidates []string
			for _, e := range entries {
				if e.IsDir() && intellijDirRe.MatchString(e.Name()) {
					exe := filepath.Join(jetbrainsDir, e.Name(), "bin", "idea64.exe")
					if _, statErr := os.Stat(exe); statErr == nil {
						candidates = append(candidates, exe)
					}
				}
			}
			// Sort descending so the newest version (lexicographically highest) is first.
			sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
			if len(candidates) > 0 {
				return candidates[0], nil
			}
		}

		return "", os.ErrNotExist
	default:
		if p, err := exec.LookPath(editor); err == nil {
			return p, nil
		}
		return editor, nil
	}
}

func (s *powerShellService) FindMavenRoot(dir string) string {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, "pom.xml")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dir
}
