//go:build !windows

package tui

import "syscall"

// detachedSysProcAttr returns a SysProcAttr that starts the child process in
// a new session, detaching it from the TUI's controlling terminal.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
