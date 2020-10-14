// +build darwin freebsd openbsd

package pidfile

import (
	"os"
	"syscall"
)

func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err = process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}
