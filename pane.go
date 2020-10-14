// This file contains the Pane type and all associated functions
package main

// #cgo CFLAGS: -g -Wall
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

// Panes is an array that hol;ds all the panes
var Panes []Pane
var paneIDM sync.Mutex

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	ID int
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
	paneIDM.Lock()
	pane := Pane{
		ID:     len(Panes) + 1,
		C:      cmd,
		Tty:    tty,
		Buffer: nil,
		dcs:    []*webrtc.DataChannel{d},
		Ws:     ws,
		st:     st,
	}
	Panes = append(Panes, pane)
	paneIDM.Unlock()

	return &pane, nil
}

// SendID sends the pane id as a message on the channel
// In APIv1 the client expects this message as the first in the channel
func (pane *Pane) SendID(dc *webrtc.DataChannel) {
	s := strconv.Itoa(pane.ID)
	bs := []byte(s)
	// TODO: send the channel in a control message
	dc.Send(bs)
}

// ReadLoop reads the tty and send data it finds to the open data channels
func (pane *Pane) ReadLoop() {
	b := make([]byte, 4096)
	id := pane.ID
	for pane.C.ProcessState.String() != "killed" {
		l, err := pane.Tty.Read(b)
		if l == 0 {
			break
		}
		// We need to get the dcs from Panes for an updated version
		pane := Panes[id-1]
		Logger.Infof("Sending output to %d dcs", len(pane.dcs))
		for i := 0; i < len(pane.dcs); i++ {
			dc := pane.dcs[i]
			s := dc.ReadyState()
			if s == webrtc.DataChannelStateOpen {
				err = dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			} else {
				Logger.Infof("removind dc because state: %q", s)
				if id == 0 {
					Panes[id-1].dcs = pane.dcs[1:]
				} else {
					Panes[id-1].dcs = append(pane.dcs[:i], pane.dcs[i+1:]...)
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

// Kill takes a pane to the sands of Rishon and buries it
func (pane *Pane) Kill() {
	Logger.Infof("killing pane %d", pane.ID)
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

// OnCloseDC is called when the client closes a pane
func (pane *Pane) OnCloseDC() {
	// TODO: remove the dc from the slice
	Logger.Infof("pane #%d: Data channel closed", pane.ID)
}

// OnMessage is called when a new client message is recieved
func (pane *Pane) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	l, err := pane.Tty.Write(p)
	if err != nil {
		Logger.Warnf("pty of %d write failed: %v",
			pane.ID, err)
	}
	if l != len(p) {
		Logger.Warnf("pty of %d wrote %d instead of %d bytes",
			pane.ID, l, len(p))
	}
}

// Resize is used to resize the pane's tty.
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Resize(ws *pty.Winsize) {
	if ws != nil && (ws.Rows != pane.Ws.Rows || ws.Cols != pane.Ws.Cols) {
		Logger.Infof("Changing pty size for pane %d: %v", pane.ID, ws)
		pane.Ws = ws
		pty.Setsize(pane.Tty, ws)
		if pane.st != nil {
			STResize(pane.st, ws.Cols, ws.Rows)
			// STResize := pane.Term.SetSize(uint(ws.Cols), uint(ws.Rows))
		}
	}
}

// Restore sends the panes' visible lines line to a data channel
// the data channel is specified by it index in the pane.dcs slice
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Restore(dIdx int) {
	if pane.st != nil {
		Logger.Infof("Sending scrren dump to %d", pane.ID)
		dc := STDumpContext{pane.ID, dIdx}
		STDump(pane.st, &dc)
		// position the cursor
		ps := fmt.Sprintf("\x1b[%d;%dH",
			int(pane.st.c.y)+1, int(pane.st.c.x)+1)
		pane.dcs[dIdx].Send([]byte(ps))
	} else {
		Logger.Warn("not restoring as st is null")
	}
}
