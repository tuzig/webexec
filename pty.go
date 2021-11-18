package main

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type ptyMuxInterface interface {
	Start(c *exec.Cmd) (pty *os.File, err error)
	StartWithSize(c *exec.Cmd, sz *pty.Winsize) (pty *os.File, err error)
}

type ptyMuxType struct{}

func (pm ptyMuxType) Start(c *exec.Cmd) (*os.File, error) {
	return pty.Start(c)
}
func (pm ptyMuxType) StartWithSize(c *exec.Cmd, sz *pty.Winsize) (*os.File, error) {
	return pty.StartWithSize(c, sz)
}

var ptyMux ptyMuxInterface
