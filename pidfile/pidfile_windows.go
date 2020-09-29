// +build windows

/* MT: According to the doc - FindProcess works cross platform and on *nix it
always succeed (that's why you do the kill(0) to check).

IMO here you can use it without the check and it'll work as expected.
*/

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
