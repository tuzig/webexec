// This file contains the Pane type and all associated functions
package main

import (
	"fmt"
	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
	"github.com/tuzig/webexec/terminal"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

var Panes []Pane

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	Id int
	// C holds the exectuted command
	C        *exec.Cmd
	Tty      *os.File
	Buffer   [][]byte
	dcs      []*webrtc.DataChannel
	Ws       *pty.Winsize
	Term     *terminal.Terminal
	termChan chan rune
}

// NewPane opens a new pane and start its command and pty
func NewPane(command []string, d *webrtc.DataChannel,
	ws *pty.Winsize) (*Pane, error) {
	var (
		m     sync.Mutex
		err   error
		tty   *os.File
		term  *terminal.Terminal
		termc chan rune
	)

	cmd := exec.Command(command[0], command[1:]...)
	if ws != nil {
		tty, err = pty.StartWithSize(cmd, ws)
	} else {
		// TODO: don't use a pty, just pipe the input and output
		tty, err = pty.Start(cmd)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed launching %q: %q", command, err)
	}
	if ws != nil {
		term = terminal.New(Logger, Config)
		termc = make(chan rune, 4096)
		err = term.SetSize(uint(ws.Cols), uint(ws.Rows))
		if err != nil {
			return nil, fmt.Errorf("Failed to set terminal size: %s", err)
		}
		go term.UpdateLoop(termc)
	}

	m.Lock()
	pane := Pane{
		Id:       len(Panes) + 1,
		C:        cmd,
		Tty:      tty,
		Buffer:   nil,
		dcs:      []*webrtc.DataChannel{d},
		Ws:       ws,
		Term:     terminal.New(Logger, Config),
		termChan: termc,
	}
	Panes = append(Panes, pane)
	m.Unlock()
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
		dcs := Panes[id-1].dcs
		for i := 0; i < len(dcs); i++ {
			dc := dcs[i]
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				err = dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			}
		}
		// update the local term emulator
		if pane.termChan != nil {
			for i := 0; i < l; i++ {
				pane.termChan <- rune(b[i])
			}
		}
		if err == io.EOF {
			break
		}
	}
	// Panes[id-1].Kill()
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
		// TODO: send resize message to all connected dcs
	}
}

// pane.Restore sends the panes' visible lines line to a data channel
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Restore(d *webrtc.DataChannel) {
	if pane.Term == nil {
		Logger.Info("Pane.Term is nil")
	}
	buffer := pane.Term.ActiveBuffer()
	b := buffer.Dump()
	Logger.Infof("Restore got a dump size: %d", len(b))
	if len(b) > 0 {
		d.Send(b)
	}
	// TODO: move cursor where belongs
}
