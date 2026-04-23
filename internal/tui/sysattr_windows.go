//go:build windows

package tui

import "syscall"

// detachedSysProcAttr returns a SysProcAttr for Windows.
// Windows does not have Setsid; use CreationFlags to create a new process group.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
}
