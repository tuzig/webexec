// This file contains the Pane type and all associated functions
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/pion/webrtc/v3"
)

// Panes is an array that hol;ds all the panes
var Panes = NewPanesDB()

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	ID int
	// C holds the exectuted command
	C         *exec.Cmd
	IsRunning bool
	TTY       *os.File
	Buffer    *Buffer
	Ws        *pty.Winsize
	vt        vt10x.VT
}

// execCommand in ahelper function for executing a command
func execCommand(command []string, ws *pty.Winsize) (*exec.Cmd, *os.File, error) {
	var (
		tty *os.File
		err error
	)
	Logger.Infof("Starting command %s", command[0])
	cmd := exec.Command(command[0], command[1:]...)
	if Conf.env != nil {
		for k, v := range Conf.env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if ws != nil {
		tty, err = pty.StartWithSize(cmd, ws)
		if err != nil {
			Logger.Errorf("got an error starting with size %s", err)
			return nil, nil, fmt.Errorf("Failed starting command: %q", err)
		}
	} else {
		// TODO: remove the pty
		tty, err = pty.Start(cmd)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("Failed launching %q: %q", command, err)
	}
	return cmd, tty, nil
}

// NewPane opens a new pane and start its command and pty
func NewPane(command []string, peer *Peer, ws *pty.Winsize) (*Pane, error) {

	var vt vt10x.VT
	cmd, tty, err := execCommand(command, ws)
	if err != nil {
		return nil, err
	}
	if ws != nil {
		vt = vt10x.New()
		vt.Resize(int(ws.Cols), int(ws.Rows))
	}
	pane := &Pane{
		C:         cmd,
		IsRunning: true,
		TTY:       tty,
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
	conL := 0
	b := make([]byte, 4096)
	id := pane.ID
	Logger.Infof("readding from tty: %v", pane.TTY)
	for {
		l, rerr := pane.TTY.Read(b)
		if rerr == io.EOF {
			Logger.Infof("Got an EOF, Killing pane %d", id)
			break
		}
		if rerr != nil && rerr != io.EOF {
			Logger.Errorf("Got an error reqading from pty#%d: %s", id, rerr)
		}
		if l == 0 {
			// 't kill the pane only on the third consequtive empty message
			conL++
			Logger.Infof("got lenght - rerr - %s", rerr)
			if conL <= 3 {
				continue
			} else {
				Logger.Infof("3rd connsecutive empty message, killin")
				break
			}
		}
		conL = 0
		// We need to get the dcs from Panes for an updated version
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
	}

	Logger.Infof("Exiting the readloop")
	pane = Panes.Get(id)
	if pane == nil {
		Logger.Errorf("no such pane %d", id)
	} else {
		pane.Kill()
	}
}

// Kill takes a pane to the sands of Rishon and buries it
func (pane *Pane) Kill() {
	Logger.Infof("Killing a pane")
	for _, d := range cdb.All4Pane(pane) {
		if d.dc.ReadyState() == webrtc.DataChannelStateOpen {
			d.dc.Close()
		}
		cdb.Delete(d)
	}
	if pane.IsRunning {
		pane.TTY.Close()
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
	l, err := pane.TTY.Write(p)
	if err == os.ErrClosed {
		Logger.Infof("got an os.ErrClosed")
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
		pty.Setsize(pane.TTY, ws)
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

		err := d.Send(view)
		if err != nil {
			Logger.Errorf("Failed restoring screen: %s", err)
		}
		view = nil
	}
	// position the cursor
	x, y := t.Cursor()
	Logger.Infof("Got cursor at: %d, %d", x, y)
	ps := fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
	err := d.Send([]byte(ps))
	if err != nil {
		Logger.Errorf("Failed restoring cursor pos: %s", err)
	}
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
