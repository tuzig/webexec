// +build darwin freebsd openbsd

package pidfile

import (
	"os"
	"syscall"
)

func processExists(pid int) bool {
	if process, err := os.FindProcess(pid); err != nil {
		return false
		// MT: No need for else after return
	} else {
		if err = process.Signal(syscall.Signal(0)); err != nil {
			return false
		}
	}

	return true
}
