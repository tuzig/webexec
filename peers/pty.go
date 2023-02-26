package peers

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type PtyMuxInterface interface {
	Start(c *exec.Cmd) (pty *os.File, err error)
	StartWithSize(c *exec.Cmd, sz *pty.Winsize) (pty *os.File, err error)
}

type PtyMuxType struct{}

func (pm PtyMuxType) Start(c *exec.Cmd) (*os.File, error) {
	return pty.Start(c)
}
func (pm PtyMuxType) StartWithSize(c *exec.Cmd, sz *pty.Winsize) (*os.File, error) {
	return pty.StartWithSize(c, sz)
}

var PtyMux PtyMuxInterface
