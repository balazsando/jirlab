package debug

import (
"fmt"
"os"
"time"
)

var out *os.File

// Init opens the debug log file at path. No-op if path is empty.
func Init(path string) error {
	if path == "" {
		return nil
	}
	var err error
	out, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	return err
}

// Log writes a formatted message to the debug log file.
func Log(format string, args ...interface{}) {
	if out == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(out, "[%s] %s\n", time.Now().Format("15:04:05.000"), msg)
}

// Close closes the debug log file.
func Close() {
	if out != nil {
		_ = out.Close()
		out = nil
	}
}

// Enabled reports whether debug logging is active.
func Enabled() bool { return out != nil }
