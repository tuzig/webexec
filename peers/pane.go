// This file contains the Pane type and all associated functions
package peers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/pion/webrtc/v3"
	"github.com/shirou/gopsutil/v3/process"
)

const OutBufSize = 4096

// Panes is an array that hol;ds all the panes
var Panes = NewPanesDB()

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	ID     int
	parent int
	// C holds the exectuted command
	C            *exec.Cmd
	IsRunning    bool
	TTY          io.ReadWriteCloser
	Buffer       *Buffer
	Ws           *pty.Winsize
	vt           vt10x.VT
	outbuf       chan []byte
	cancelRWLoop context.CancelFunc
	ctx          context.Context
	peer         *Peer
}

// ExecCommand in ahelper function for executing a command
func ExecCommand(command []string, env map[string]string, ws *pty.Winsize, pID int, fp string) (*exec.Cmd, io.ReadWriteCloser, error) {

	var (
		tty *os.File
		dir string
		err error
		pwd string
	)
	cmd := exec.Command(command[0], command[1:]...)
	if pID != 0 {
		p, err := process.NewProcess(int32(pID))
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to find parent pane's process: %s %s", err, fp)
		}
		pwd, err = p.Cwd()
		if err != nil {
			return nil, nil, fmt.Errorf("Failed getting parent pane's cwd: %s %s", err, fp)
		}
		dir = pwd
	} else {
		dir, err = os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
	}
	cmd.Dir = dir
	if env != nil {
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if ws != nil {
		tty, err = PtyMux.StartWithSize(cmd, ws)
	} else {
		tty, err = PtyMux.Start(cmd)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("Failed launching %q: %q %s", command, err, fp)
	}
	return cmd, tty, nil
}

// NewPane opens a new pane
func NewPane(peer *Peer, ws *pty.Winsize, parent int) (*Pane, error) {

	var vt vt10x.VT
	if parent != 0 {
		parentPane := Panes.Get(parent)
		if parentPane == nil {
			return nil, fmt.Errorf(
				"Got a pane request with an illegal parrent pane id: %d", parent)
		}
		parent = parentPane.C.Process.Pid
	}
	if ws != nil {
		vt = vt10x.New()
		vt.Resize(int(ws.Cols), int(ws.Rows))
	}
	ctx, cancel := context.WithCancel(context.Background())
	pane := &Pane{
		parent:       parent,
		IsRunning:    false,
		Buffer:       NewBuffer(100000), //TODO: get the number from conf
		Ws:           ws,
		vt:           vt,
		outbuf:       make(chan []byte, OutBufSize),
		ctx:          ctx,
		cancelRWLoop: cancel,
		peer:         peer,
	}
	Panes.Add(pane) // This will set pane.ID
	return pane, nil
}

// start starts the command and pty
func (pane *Pane) run(command []string) error {
	logger := pane.peer.logger
	run := pane.peer.Conf.RunCommand
	if run == nil {
		run = ExecCommand
	}
	logger.Infof("Starting command: %v", command)
	cmd, tty, err := run(
		command, pane.peer.Conf.Env, pane.Ws, pane.parent, pane.peer.FP)
	if err != nil {
		logger.Warnf("command failed: %s", err)
		return err
	}
	pane.C = cmd
	pane.IsRunning = true
	pane.TTY = tty
	errbuf := new(bytes.Buffer)
	if cmd != nil {
		cmd.Stderr = errbuf
	}
	go pane.stderrLoop(errbuf)
	go pane.ReadLoop()
	return nil
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
	logger := pane.peer.logger
	conNull := 0
	id := pane.ID
	sctx, cancel := context.WithCancel(context.Background())
	go pane.sender(sctx)
	logger.Infof("readding from tty: %v", pane.TTY)
loop:
	for {
		select {
		case <-pane.ctx.Done():
			break loop
		default:
		}
		b := make([]byte, OutBufSize)
		l, rerr := pane.TTY.Read(b)
		if rerr == io.EOF {
			logger.Infof("@%d: got EOF in read loop", pane.ID)
			break loop
		}
		if rerr != nil {
			logger.Errorf("Got an error reqading from pty#%d: %s", id, rerr)
			break loop
		}
		if l == 0 {
			// 't kill the pane only on the third consequtive empty message
			conNull++
			logger.Infof("got lenght - rerr - %s", rerr)
			if conNull <= 3 {
				continue
			} else {
				logger.Infof("3rd connsecutive empty message, killin")
				break loop
			}
		}
		conNull = 0
		pane.outbuf <- b[:l]
	}

	// TODO: find a better way to wait for all the messages to be sent
	time.AfterFunc(time.Second/10, func() {
		cancel()
		pane = Panes.Get(id)
		pane.Kill()
	})
}

func (pane *Pane) sender(ctx context.Context) {
	logger := pane.peer.logger
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case m, ok := <-pane.outbuf:
			if !ok {
				break loop
			}
			// We need to get the dcs from Panes for an updated version
			cs := cdb.All4Pane(pane)
			logger.Infof("@%d: Sending %d bytes to %d dcs", pane.ID, len(m), len(cs))
			for _, d := range cs {
				s := d.dc.ReadyState()
				if s == webrtc.DataChannelStateOpen {
					err := d.dc.Send(m)
					if err != nil {
						logger.Errorf("got an error when sending message: %v", err)
					}
				} else {
					logger.Infof("closing & removing dc because state: %q", s)
					cdb.Delete(d)
					d.dc.Close()
				}
			}
			if pane.vt != nil {
				pane.vt.Write(m)
			}
			pane.Buffer.Add(m)
		}
	}
	logger.Infof("Exiting the sender loop for pane %d ", pane.ID)
}

// Kill takes a pane to the sands of Rishon and buries it
func (pane *Pane) Kill() {
	logger := pane.peer.logger
	logger.Infof("Killing a pane")
	for _, d := range cdb.All4Pane(pane) {
		if d.dc.ReadyState() == webrtc.DataChannelStateOpen {
			d.dc.Close()
		}
		cdb.Delete(d)
	}
	if pane.IsRunning {
		pane.cancelRWLoop()
		if pane.C != nil {
			err := pane.C.Process.Kill()
			if err != nil {
				logger.Errorf("Failed to kill process: %v %v",
					err, pane.C.ProcessState.String())
			}
		}
		pane.IsRunning = false
	}
	if pane.TTY != nil {
		pane.TTY.Close()
	}
}

// OnMessage is called when a new client message is recieved
func (pane *Pane) OnMessage(msg webrtc.DataChannelMessage) {
	logger := pane.peer.logger
	p := msg.Data
	l, err := pane.TTY.Write(p)
	if err == os.ErrClosed {
		logger.Infof("got an os.ErrClosed")
		pane.Kill()
		return
	}
	if err != nil {
		logger.Warnf("pty of %d write failed: %v",
			pane.ID, err)
	}
	if l != len(p) {
		logger.Warnf("pty of %d wrote %d instead of %d bytes",
			pane.ID, l, len(p))
	}
}

// Resize is used to resize the pane's tty.
// the function does nothing if it's given a nil size or the current size
func (pane *Pane) Resize(ws *pty.Winsize) {
	logger := pane.peer.logger
	if ws != nil && (ws.Rows != pane.Ws.Rows || ws.Cols != pane.Ws.Cols) {
		logger.Infof("Changing pty size for pane %d: %v", pane.ID, ws)
		pane.Ws = ws
		pty.Setsize(pane.TTY.(*os.File), ws)
		if pane.vt != nil {
			pane.vt.Resize(int(ws.Cols), int(ws.Rows))
		}
	}
}

func (pane *Pane) dumpVT() {
	logger := pane.peer.logger
	var (
		view []byte
		c    rune
	)
	t := pane.vt
	t.Lock()
	defer t.Unlock()
	rows, cols := t.Size()
	logger.Infof("dumping scree size %dx%d", rows, cols)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c, _, _ = t.Cell(x, y)
			view = append(view, byte(c))
		}
		if y < rows-1 {
			view = append(view, byte('\n'))
			view = append(view, byte('\r'))
		}
		pane.outbuf <- view
		view = nil
	}
	// position the cursor
	x, y := t.Cursor()
	logger.Infof("Got cursor at: %d, %d", x, y)
	ps := fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
	pane.outbuf <- []byte(ps)
}

// Restore restore the screen or buffer.
// If the peer has a marker data will be read from the buffer and sent over.
// If no marker, Restore uses our headless terminal emulator to restore the
// screen.
func (pane *Pane) Restore(d *webrtc.DataChannel, marker int) {
	logger := pane.peer.logger
	if marker == -1 {
		if pane.vt != nil {
			id := d.ID()
			if id == nil {
				logger.Error(
					"Failed restoring to a data channel that has no id")
			}
			logger.Infof(
				"Sending scrren dump to pane: %d, dc: %d", pane.ID, *id)
			//TODO: this and the next afterfunc is silly
			time.AfterFunc(time.Second/10, func() {
				pane.dumpVT()
			})
		} else {
			logger.Warn("not restoring as st is null")
		}
	} else {
		logger.Infof("Sending history buffer since marker: %d", marker)
		time.AfterFunc(time.Second/10, func() {
			pane.outbuf <- pane.Buffer.GetSinceMarker(marker)
		})
	}
}
func (pane *Pane) stderrLoop(errors *bytes.Buffer) {
	logger := pane.peer.logger
loop:
	for {
		select {
		case <-pane.ctx.Done():
			break loop
		default:
		}
		line, err := errors.ReadString('\n')
		if err == io.EOF {
			logger.Infof("@%d: got EOF in stderr loop", pane.ID)
			break loop
		}
		if err != nil {
			logger.Errorf("@%d: got an error reading stderr: %s", pane.ID, err)
		}
		logger.Errorf("@%d: stderr: %s", line)
	}
}
