//go:build windows

package gateway

import (
	"os/exec"
	"syscall"
)

const windowsDetachedProcess = 0x00000008

func detachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsDetachedProcess}
}
