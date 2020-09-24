// +build windows

package pidfile

import (
	"golang.org/x/sys/windows"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func processExists(pid int) bool {
	if h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid)); err != nil {
		return false
	} else {
		defer windows.Close(h)

		var c uint32
		if err := windows.GetExitCodeProcess(h, &c); err != nil {
			return c == stillActive
		}
	}

	return true
}
