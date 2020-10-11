// This file contains the Pane type and all associated functions
package main

// #cgo CFLAGS: -g -Wall -Wtypedef-redefinition
// #include <stdlib.h>
// #include "st.h"
import "C"

import (
	"fmt"
	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

var Panes []Pane
var paneIdM sync.Mutex

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	Id int
	// C holds the exectuted command
	C      *exec.Cmd
	Tty    *os.File
	Buffer [][]byte
	dcs    []*webrtc.DataChannel
	Ws     *pty.Winsize
	// st is a C based terminal emulator used for screen restore
	st *C.Term
}

// NewPane opens a new pane and start its command and pty
func NewPane(command []string, d *webrtc.DataChannel,
	ws *pty.Winsize) (*Pane, error) {

	var (
		err error
		tty *os.File
		st  *C.Term
	)

	cmd := exec.Command(command[0], command[1:]...)
	if ws != nil {
		tty, err = pty.StartWithSize(cmd, ws)
		st = STNew(ws.Cols, ws.Rows)
		Logger.Infof("Got a new ST: %p", st)
	} else {
		// don't use a pty, just pipe the input and output
		tty, err = pty.Start(cmd)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed launching %q: %q", command, err)
	}
	paneIdM.Lock()
	pane := Pane{
		Id:     len(Panes) + 1,
		C:      cmd,
		Tty:    tty,
		Buffer: nil,
		dcs:    []*webrtc.DataChannel{d},
		Ws:     ws,
		st:     st,
	}
	Panes = append(Panes, pane)
	paneIdM.Unlock()

	return &pane, nil
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
	// BD: turns out the data channel does not implement the Write interface
	// MT: You can use SIGCHILD to know if a child process died
	// BD: Readability wise, I prefer the code as is. Is there any advatage to
	//      using SIGCHILD?
	b := make([]byte, 4096)
	id := pane.Id
	for pane.C.ProcessState.String() != "killed" {
		l, err := pane.Tty.Read(b)
		if l == 0 {
			break
		}
		// We need to get the dcs from Panes or we don't get an updated version
		pane := Panes[id-1]
		Logger.Infof("Sending output to %d dcs", len(pane.dcs))
		for i := 0; i < len(pane.dcs); i++ {
			dc := pane.dcs[i]
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				err = dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			}
		}
		// TODO: does this work?
		if pane.st != nil {
			STWrite(pane.st, string(b[:l]))
		}
	}
	Panes[id-1].Kill()
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
func (pane *Pane) OnCloseDC() {
	// TODO: remove the dc from the slice
	Logger.Infof("pane #%d: Data channel closed", pane.Id)
}

func (pane *Pane) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
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

// pane.Resize is used to resize the pane's tty.
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Resize(ws *pty.Winsize) {
	if ws != nil && (ws.Rows != pane.Ws.Rows || ws.Cols != pane.Ws.Cols) {
		Logger.Infof("Changing pty size for pane %d: %v", pane.Id, ws)
		pane.Ws = ws
		pty.Setsize(pane.Tty, ws)
		if pane.st != nil {
			STResize(pane.st, ws.Cols, ws.Rows)
			// STResize := pane.Term.SetSize(uint(ws.Cols), uint(ws.Rows))
		}
	}
}

// pane.Restore sends the panes' visible lines line to a data channel
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Restore(d *webrtc.DataChannel) {
	if pane.st != nil {
		b, l := STDump(pane.st)
		if l > 0 {
			d.Send(b)
			// position the cursor
			ps := fmt.Sprintf("\x1b[%d;%dH",
				int(pane.st.c.y)+1, int(pane.st.c.x)+1)
			d.Send([]byte(ps))
		} else {
			Logger.Info("not restoring as dump len is 0")
		}
	} else {
		Logger.Info("not restoring as st is null")
	}
}
