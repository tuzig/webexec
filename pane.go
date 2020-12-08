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
var Panes = NewPanesDB()

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	ID int
	// C holds the exectuted command
	C         *exec.Cmd
	IsRunning bool
	Tty       *os.File
	Buffer    *Buffer
	dcs       *DCsDB
	Ws        *pty.Winsize
	// st is a C based terminal emulator used for screen restore
	st  *C.Term
	stM sync.RWMutex
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
	pane := &Pane{
		C:         cmd,
		IsRunning: true,
		Tty:       tty,
		Buffer:    NewBuffer(100000), //TODO: get the number from conf
		dcs:       NewDCsDB(),
		Ws:        ws,
		st:        st,
	}
	Panes.Add(pane) // This will set pane.ID
	pane.dcs.Add(d)
	return pane, nil
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
	for {
		l, err := pane.Tty.Read(b)
		if l == 0 {
			break
		}
		// We need to get the dcs from Panes for an updated version
		pane := Panes.Get(id)
		Logger.Infof("@%d: Sending output to %d dcs", pane.ID, pane.dcs.Len())
		for _, dc := range pane.dcs.All() {
			s := dc.ReadyState()
			if s == webrtc.DataChannelStateOpen {
				err = dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			} else {
				Logger.Infof("closing & removing dc because state: %q", s)
				id := dc.ID()
				if id == nil {
					Logger.Error("Failed to delete a data channel with no id")
					return
				}
				pane.dcs.Delete(*id)
				dc.Close()
			}
		}
		if pane.st != nil {
			pane.stM.Lock()
			STWrite(pane.st, string(b[:l]))
			pane.stM.Unlock()
		}
		pane.Buffer.Add(b[:l])
	}

	Logger.Infof("Killing pane %d", id)
	pane = Panes.Get(id)
	if pane == nil {
		Logger.Errorf("no such pane %d", id)
	} else {
		pane.Kill()
	}
}

// Kill takes a pane to the sands of Rishon and buries it
func (pane *Pane) Kill() {
	Logger.Infof("Killing pane: %d", pane.ID)
	pane.dcs.CloseAll()
	if pane.IsRunning {
		pane.Tty.Close()
		err := pane.C.Process.Kill()
		if err != nil {
			Logger.Errorf("Failed to kill process: %v %v",
				err, pane.C.ProcessState.String())
		}
		pane.IsRunning = false
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
	if err == os.ErrClosed {
		pane.dcs.CloseAll()
		return
	}
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
			pane.stM.Lock()
			STResize(pane.st, ws.Cols, ws.Rows)
			pane.stM.Unlock()
		}
	}
}

// Restore restore the screen or buffer.
// If the peer has a marker data will be read from the buffer and sent over.
// If no marker, Restore uses our headless terminal emulator to restore the
// screen.
func (pane *Pane) Restore(d *webrtc.DataChannel, marker int) {
	if marker == -1 {
		if pane.st != nil {
			id := d.ID()
			if id == nil {
				Logger.Error(
					"Failed restoring to a data channel that has no id")
			}
			Logger.Infof(
				"Sending scrren dump to pane: %d, dc: %d", pane.ID, *id)
			dc := STDumpContext{pane.ID, *id}
			pane.stM.Lock()
			STDump(pane.st, &dc)
			pane.stM.Unlock()
			// position the cursor
			ps := fmt.Sprintf("\x1b[%d;%dH",
				int(pane.st.c.y)+1, int(pane.st.c.x)+1)
			d.Send([]byte(ps))
		} else {
			Logger.Warn("not restoring as st is null")
		}
	} else {
		d.Send(pane.Buffer.GetSinceMarker(marker))
	}
}
