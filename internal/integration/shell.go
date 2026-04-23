//go:build !powershell

package integration

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NewShellService returns the platform-appropriate ShellService.
// Returns the WSL/Linux implementation.
func NewShellService() ShellService {
	return &unixShellService{}
}

// --- WSL / Linux implementation ---

type unixShellService struct{}

func (s *unixShellService) OpenBrowser(url string) error {
	return exec.Command("wslview", url).Start()
}

func (s *unixShellService) CopyToClipboard(text string) error {
	cmds := [][]string{
		{"clip.exe"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return nil // best-effort
}

func (s *unixShellService) ResolveEditorPath(editor string) (string, error) {
	switch editor {
	case "code":
		wslPath := "/mnt/c/Program Files/Microsoft VS Code/bin/code"
		if _, err := os.Stat(wslPath); err == nil {
			return wslPath, nil
		}
		if p, err := exec.LookPath("code"); err == nil {
			return p, nil
		}
		return "code", nil

	case "idea":
		distDir := os.ExpandEnv("$HOME/.cache/JetBrains/RemoteDev/dist")
		var found string
		_ = filepath.WalkDir(distDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if !d.IsDir() && d.Name() == "idea.sh" {
				found = p
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
		return "", os.ErrNotExist

	default:
		if p, err := exec.LookPath(editor); err == nil {
			return p, nil
		}
		return editor, nil
	}
}

func (s *unixShellService) FindMavenRoot(dir string) string {
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
