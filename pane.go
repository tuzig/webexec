// This file contains the Pane type and all associated functions
package main

import (
	"github.com/pion/webrtc/v2"
	"io"
	"os"
	"os/exec"
	"strconv"
)

var Panes []Pane

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	Id int
	// C holds the exectuted command
	C      *exec.Cmd
	Tty    *os.File
	Buffer [][]byte
	dcs    []*webrtc.DataChannel
}

// pane.SendId sends the pane id as a message on the channel
// In APIv1 the client expects this message as the first in the channel
func (pane *Pane) SendId(dc *webrtc.DataChannel) {
	s := strconv.Itoa(pane.Id)
	bs := []byte(s)
	// Logger.Infof("Added a command: id %d tty - %q id - %q", pId, tty.Name(), bs)
	// TODO: send the channel in a control message
	dc.Send(bs)
}

// pane.ReadLoop reads the tty and send data it finds to the open data channels
func (pane *Pane) ReadLoop() {
	// MT: io.Copy & https://golang.org/pkg/io/#MultiWriter
	// MT: You can use SIGCHILD to know if a child process died
	b := make([]byte, 4096)
	for pane.C.ProcessState.String() != "killed" {
		l, err := pane.Tty.Read(b)
		Logger.Infof("> %d: %s", l, b[:l])
		if l == 0 {
			break
		}
		for i := 0; i < len(pane.dcs); i++ {
			dc := pane.dcs[i]
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				Logger.Infof("> %d: %s", l, b[:l])
				err = dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
	pane.Kill()
}

// pane.Kill takes a pane to the sands of Rishon and buries it
func (pane *Pane) Kill() {
	Logger.Infof("killing pane %d", pane.Id)
	for i := 0; i < len(pane.dcs); i++ {
		dc := pane.dcs[i]
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			dc.Close()
		}
	}
	pane.Tty.Close()
	if pane.C.ProcessState.String() != "killed" {
		err := pane.C.Process.Kill()
		if err != nil {
			Logger.Errorf("Failed to kill process: %v %v",
				err, pane.C.ProcessState.String())
		}
	}
}
func (pane *Pane) OnClose() {
	Logger.Infof("pane #%d: Data channel closed", pane.Id)
}

func (pane *Pane) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	Logger.Infof("> %q", p)
	l, err := pane.Tty.Write(p)
	if err != nil {
		Logger.Warnf("pty of %d write failed: %v",
			pane.Id, err)
	}
	if l != len(p) {
		Logger.Warnf("pty of %d wrote %d instead of %d bytes",
			pane.Id, l, len(p))
	}
}
