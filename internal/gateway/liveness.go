//go:build unix

package gateway

import (
	"os"
	"syscall"
)

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	if errno, ok := err.(syscall.Errno); ok {
		switch errno {
		case syscall.ESRCH:
			return false
		case syscall.EPERM:
			return true
		}
	}
	return false
}
