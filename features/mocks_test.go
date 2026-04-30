package features_test

// mockShell is a no-op ShellService used in feature tests.
type mockShell struct {
	lastURL       string
	lastClipboard string
}

func (m *mockShell) OpenBrowser(url string) error {
	m.lastURL = url
	return nil
}

func (m *mockShell) CopyToClipboard(text string) error {
	m.lastClipboard = text
	return nil
}

func (m *mockShell) ResolveEditorPath(editor string) (string, error) {
	return "/usr/bin/" + editor, nil
}

func (m *mockShell) FindMavenRoot(dir string) string {
	return ""
}
