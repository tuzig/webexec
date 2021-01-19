// This file contains the Pane type and all associated functions
package main

import (
	"fmt"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/pion/webrtc/v3"
	"io"
	"os"
	"os/exec"
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
	Ws        *pty.Winsize
	vt        vt10x.VT
}

// NewPane opens a new pane and start its command and pty
func NewPane(command []string, d *webrtc.DataChannel, peer *Peer,
	ws *pty.Winsize) (*Pane, error) {

	var (
		err error
		tty *os.File
		vt  vt10x.VT
	)

	cmd := exec.Command(command[0], command[1:]...)
	if ws != nil {
		tty, err = pty.StartWithSize(cmd, ws)
		if err != nil {
			return nil, fmt.Errorf("Failed starting command: %q", err)
		}
		vt = vt10x.New()
		vt.Resize(int(ws.Cols), int(ws.Rows))
	} else {
		// TODO: remove the pty
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
		Ws:        ws,
		vt:        vt,
	}
	Panes.Add(pane) // This will set pane.ID
	return pane, nil
}

// sendFirstMessage sends the pane id and dimensions
func (pane *Pane) sendFirstMessage(dc *webrtc.DataChannel) {
	var r string
	if pane.Ws != nil {
		r = fmt.Sprintf("%d,%dx%d", pane.ID, pane.Ws.Rows, pane.Ws.Cols)
	} else {
		r = fmt.Sprintf("%d", pane.ID)
	}
	dc.Send([]byte(r))
}

// ReadLoop reads the tty and send data it finds to the open data channels
func (pane *Pane) ReadLoop() {
	b := make([]byte, 4096)
	id := pane.ID
	for {
		l, rerr := pane.Tty.Read(b)
		if rerr != nil && rerr != io.EOF {
			Logger.Errorf("Got an error reqading from pty#%d: %s", id, rerr)
		}
		if l == 0 {
			break
		}
		// We need to get the dcs from Panes for an updated version
		pane := Panes.Get(id)
		cs := cdb.All4Pane(pane)
		Logger.Infof("@%d: Sending output to %d dcs", pane.ID, len(cs))
		for _, d := range cs {
			s := d.dc.ReadyState()
			if s == webrtc.DataChannelStateOpen {
				err := d.dc.Send(b[:l])
				if err != nil {
					Logger.Errorf("got an error when sending message: %v", err)
				}
			} else {
				Logger.Infof("closing & removing dc because state: %q", s)
				cdb.Delete(d)
				d.dc.Close()
			}
		}
		if pane.vt != nil {
			pane.vt.Write(b[:l])
		}
		pane.Buffer.Add(b[:l])
		if rerr == io.EOF {
			break
		}
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

	for _, d := range cdb.All4Pane(pane) {
		if d.dc.ReadyState() == webrtc.DataChannelStateOpen {
			d.dc.Close()
		}
		cdb.Delete(d)
	}
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

// OnMessage is called when a new client message is recieved
func (pane *Pane) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	l, err := pane.Tty.Write(p)
	if err == os.ErrClosed {
		pane.Kill()
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
		if pane.vt != nil {
			pane.vt.Resize(int(ws.Cols), int(ws.Rows))
		}
	}
}

func (pane *Pane) dumpVT(d *webrtc.DataChannel) {
	var (
		view []byte
		c    rune
	)
	t := pane.vt
	t.Lock()
	defer t.Unlock()
	rows, cols := t.Size()
	Logger.Infof("dumping scree size %dx%d", rows, cols)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c, _, _ = t.Cell(x, y)
			view = append(view, byte(c))
		}
		if y < rows-1 {
			view = append(view, byte('\n'))
			view = append(view, byte('\r'))
		}
		d.Send(view)
		view = nil
	}
	// position the cursor
	x, y := t.Cursor()
	Logger.Infof("Got cursor at: %d, %d", x, y)
	ps := fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
	d.Send([]byte(ps))
}

// Restore restore the screen or buffer.
// If the peer has a marker data will be read from the buffer and sent over.
// If no marker, Restore uses our headless terminal emulator to restore the
// screen.
func (pane *Pane) Restore(d *webrtc.DataChannel, marker int) {
	if marker == -1 {
		if pane.vt != nil {
			id := d.ID()
			if id == nil {
				Logger.Error(
					"Failed restoring to a data channel that has no id")
			}
			Logger.Infof(
				"Sending scrren dump to pane: %d, dc: %d", pane.ID, *id)
			pane.dumpVT(d)
		} else {
			Logger.Warn("not restoring as st is null")
		}
	} else {
		Logger.Infof("Sending history buffer since marker: %d", marker)
		d.Send(pane.Buffer.GetSinceMarker(marker))
	}
}
