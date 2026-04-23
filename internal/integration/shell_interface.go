package integration

// ShellService abstracts OS-specific operations such as opening URLs in a
// browser, copying text to the clipboard, and launching external editors.
// Implementations can be swapped per-platform (WSL, native Linux, PowerShell).
type ShellService interface {
	// OpenBrowser opens a URL in the system's default browser.
	OpenBrowser(url string) error
	// CopyToClipboard writes text to the system clipboard.
	CopyToClipboard(text string) error
	// ResolveEditorPath returns the absolute path to the given editor binary.
	// Supported names: "code", "idea", "vim".
	ResolveEditorPath(editor string) (string, error)
	// FindMavenRoot walks upward from dir until a pom.xml is found.
	FindMavenRoot(dir string) string
}
