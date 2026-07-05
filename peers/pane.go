// This file contains the Pane type and all associated functions
package peers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v4"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/tuzig/vt10x"
)

const OutBufSize = 4096

// Panes is an array that hol;ds all the panes
var Panes = NewPanesDB()

// Pane type hold a command, a pseudo tty and the connected data channels
type Pane struct {
	sync.Mutex
	ID     int
	parent int
	// C holds the exectuted command
	C            *exec.Cmd
	IsRunning    bool
	TTY          io.ReadWriteCloser
	Buffer       *Buffer
	Ws           *pty.Winsize
	vt           vt10x.Terminal
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
	go func() {
		cmd.Process.Wait()
	}()

	return cmd, tty, nil
}

// NewPane opens a new pane
func NewPane(peer *Peer, ws *pty.Winsize, parent int) (*Pane, error) {

	var vt vt10x.Terminal
	if parent != 0 {
		parentPane := Panes.Get(parent)
		if parentPane == nil {
			return nil, fmt.Errorf(
				"Got a pane request with an illegal parrent pane id: %d", parent)
		}
		//TODO: handle a crash here, when there's a parent pane but no process yet
		//      https://github.com/tuzig/webexec/issues/106
		parent = parentPane.C.Process.Pid
	}
	if ws != nil {
		vt = vt10x.New(vt10x.WithSize(int(ws.Cols), int(ws.Rows)))
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
func (pane *Pane) Run(command []string) error {
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
	pane.Lock()
	pane.IsRunning = true
	pane.Unlock()
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
		filtered := pane.filterPTYOutput(b[:l])
		if len(filtered) > 0 {
			pane.outbuf <- filtered
		}
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
			cs := CDB.All4Pane(pane)
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
					CDB.Delete(d)
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
	for _, d := range CDB.All4Pane(pane) {
		if d.dc.ReadyState() == webrtc.DataChannelStateOpen {
			d.dc.Close()
		}
		CDB.Delete(d)
	}
	pane.Lock()
	defer pane.Unlock()
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

// OnMessage is called when a new client message is recieved.
// The sender *Peer identifies which client sent the message; this is used
// for active-peer tracking and OSC color query filtering.
// Terminal query escape sequences are intercepted before reaching the PTY to
// prevent N² amplification when multiple clients share a pane:
//   - CPR/DSR/DA queries are answered directly from vt10x state or hardcoded
//     responses and routed through outbuf to all clients.
//   - OSC 10/11 color queries are forwarded to the PTY only from the
//     most-recently-active client; other clients' color queries are dropped.
//   - All other input passes through to the PTY unchanged.
func (pane *Pane) OnMessage(sender *Peer, msg webrtc.DataChannelMessage) {
	logger := pane.peer.logger
	p := msg.Data

	// Intercept terminal query escape sequences before hitting the PTY.
	if response, handled := pane.interceptQuery(sender, p); handled {
		if len(response) > 0 {
			// Synthesized response: push to outbuf so sender()
			// broadcasts it to all clients (pane-wide state).
			pane.outbuf <- response
		}
		return
	}

	// Only real input (keystrokes, etc.) updates the active peer —
	// not intercepted query sequences.  This prevents the race where
	// SetLastPeer would run before OnMessage, making every OSC color
	// query appear to come from the active peer.
	SetLastPeer(sender)

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

// Terminal query response constants shared by interceptQuery and filterPTYOutput.
const (
	dsrStatusResponse   = "\x1b[0n"
	primaryDAResponse   = "\x1b[?1;2c"
	secondaryDAResponse = "\x1b[>0;115;0c"
)

// matchCSIQuery checks if data[pos] starts with a known CSI terminal query.
// Returns (response, queryLength, true) if matched.
// queryLength includes the \x1b[ prefix.
func (pane *Pane) matchCSIQuery(data []byte, pos int) ([]byte, int, bool) {
	// Caller ensures data[pos] == \x1b and data[pos+1] == '['
	if pos+2 >= len(data) {
		return nil, 0, false
	}
	b := data[pos+2:] // bytes after \x1b[

	switch {
	// CSI 6n — cursor position query → CPR from vt10x
	case len(b) >= 2 && b[0] == '6' && b[1] == 'n':
		return pane.synthesizeCPR(), 4, true
	// CSI 5n — device status query → "no malfunction"
	case len(b) >= 2 && b[0] == '5' && b[1] == 'n':
		return []byte(dsrStatusResponse), 4, true
	// CSI >c — secondary DA (check before >0c)
	case len(b) >= 2 && b[0] == '>' && b[1] == 'c':
		return []byte(secondaryDAResponse), 4, true
	// CSI >0c — secondary DA
	case len(b) >= 3 && b[0] == '>' && b[1] == '0' && b[2] == 'c':
		return []byte(secondaryDAResponse), 5, true
	// CSI c — primary DA
	case len(b) >= 1 && b[0] == 'c':
		return []byte(primaryDAResponse), 3, true
	// CSI 0c — primary DA
	case len(b) >= 2 && b[0] == '0' && b[1] == 'c':
		return []byte(primaryDAResponse), 4, true
	}
	return nil, 0, false
}

// interceptQuery checks if data is a known terminal query escape sequence.
// The sender *Peer identifies which client sent the data; this is used for
// OSC color query filtering against the active peer.
// Returns (response, handled):
//   - handled=false: not a known query — pass through to PTY.
//   - handled=true, len(response)>0: synthesized answer — push to outbuf.
//   - handled=true, len(response)==0: silently dropped (non-active OSC query).
func (pane *Pane) interceptQuery(sender *Peer, p []byte) ([]byte, bool) {
	if len(p) < 3 || p[0] != 0x1b {
		return nil, false
	}

	// CSI queries — use shared matchCSIQuery helper
	if p[1] == '[' {
		resp, qLen, matched := pane.matchCSIQuery(p, 0)
		if matched && qLen == len(p) {
			return resp, true
		}
	}

	switch {
	// OSC 10/11 color queries — forward to PTY from active peer only
	case isOSCColorQuery(p):
		activePeer := GetActivePeer()
		if activePeer != sender {
			return nil, true // drop from non-active peer
		}
		return nil, false // pass through to PTY from active peer

	// OSC 10/11 color responses (not queries) — forward to PTY from
	// active peer only, to prevent N copies of the response from reaching
	// the program when N clients all respond to the broadcast query.
	case isOSCColorResponse(p):
		activePeer := GetActivePeer()
		if activePeer != sender {
			return nil, true // drop from non-active peer
		}
		return nil, false // pass through to PTY from active peer
	}

	return nil, false
}

// filterPTYOutput scans PTY output for terminal query escape sequences
// (OSC 10/11 color queries and CSI 6n/5n/c/>c queries).
// When a query is found, it synthesizes a response and writes it directly
// back to the PTY (so the running program gets its answer), then strips
// the query from the data before broadcasting to clients (so clients never
// see it and never respond, preventing N² amplification).
func (pane *Pane) filterPTYOutput(data []byte) []byte {
	for i := 0; i < len(data); i++ {
		if data[i] != 0x1b {
			continue
		}
		if i+1 >= len(data) {
			continue
		}

		// CSI queries: \x1b[ followed by 6n, 5n, c, 0c, >c, >0c
		if data[i+1] == '[' {
			response, qLen, matched := pane.matchCSIQuery(data, i)
			if matched {
				if pane.TTY != nil {
					pane.TTY.Write(response)
				}
				data = append(data[:i], data[i+qLen:]...)
				i-- // re-check same position
				continue
			}
			continue
		}

		// OSC color queries: \x1b]10;? or \x1b]11;? terminated by BEL or ST
		if data[i+1] != ']' {
			continue
		}
		// Check for "10;?" or "11;?" — ensure i+5 is in bounds first
		if i+5 >= len(data) || data[i+4] != ';' || data[i+5] != '?' {
			continue
		}
		color := byte(0)
		if data[i+2] == '1' && data[i+3] == '0' {
			color = 10
		} else if data[i+2] == '1' && data[i+3] == '1' {
			color = 11
		} else {
			continue
		}
		// Find the terminator: BEL (\x07) or ST (\x1b\\)
		end := -1
		for j := i + 6; j < len(data); j++ {
			if data[j] == 0x07 {
				end = j + 1
				break
			}
			if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '\\' {
				end = j + 2
				break
			}
		}
		if end == -1 {
			continue // incomplete sequence, leave it
		}
		// Synthesize a response and write it back to the PTY
		var response string
		if color == 11 {
			response = "\x1b]11;rgb:0000/0000/0000\x1b\\"
		} else {
			response = "\x1b]10;rgb:ffff/ffff/ffff\x1b\\"
		}
		if pane.TTY != nil {
			pane.TTY.Write([]byte(response))
		}
		// Strip the query from the data
		data = append(data[:i], data[end:]...)
		i-- // compensate for the for loop's i++ to re-check same position
	}
	return data
}

// isOSCColorResponse reports whether p is an OSC 10 or 11 color response
// (not a query).  Responses have the form:
//
//	\x1b]11;rgb:....\x07  or  \x1b]11;rgb:....\x1b\\
//
// The key difference from a query: the 6th byte is NOT '?'.
func isOSCColorResponse(p []byte) bool {
	if len(p) < 7 || p[0] != 0x1b || p[1] != ']' {
		return false
	}
	// Must start with "10;" or "11;"
	if p[4] != ';' {
		return false
	}
	if !((p[2] == '1' && p[3] == '0') || (p[2] == '1' && p[3] == '1')) {
		return false
	}
	// Must NOT be a query (no '?' after ';')
	if p[5] == '?' {
		return false
	}
	// Must end with BEL (\x07) or ST (\x1b\\)
	last := len(p) - 1
	return p[last] == 0x07 || (last >= 1 && p[last-1] == 0x1b && p[last] == '\\')
}

// isOSCColorQuery reports whether p is an OSC 10 or 11 color query.
// Accepts both BEL (\x07) and ST (\x1b\\) terminators.
func isOSCColorQuery(p []byte) bool {
	if len(p) < 6 || p[0] != 0x1b || p[1] != ']' {
		return false
	}
	// Must start with "10;?" or "11;?"
	if p[4] != ';' || p[5] != '?' {
		return false
	}
	if !((p[2] == '1' && p[3] == '0') || (p[2] == '1' && p[3] == '1')) {
		return false
	}
	// Must end with BEL (\x07) or ST (\x1b\\)
	last := len(p) - 1
	return p[last] == 0x07 || (last >= 1 && p[last-1] == 0x1b && p[last] == '\\')
}

// synthesizeCPR builds a CPR (Cursor Position Report) from vt10x cursor state.
func (pane *Pane) synthesizeCPR() []byte {
	if pane.vt == nil {
		return []byte("\x1b[1;1R")
	}
	pane.vt.Lock()
	c := pane.vt.Cursor()
	pane.vt.Unlock()
	return []byte(fmt.Sprintf("\x1b[%d;%dR", c.Y+1, c.X+1))
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

func (pane *Pane) dumpVT() []byte {

	var result string
	var prevFG, prevBG vt10x.Color

	t := pane.vt
	t.Lock()
	defer t.Unlock()
	cols, rows := t.Size()
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			glyph := t.Cell(x, y)
			if glyph.FG != prevFG || glyph.BG != prevBG {
				result += printColorChange(glyph.FG, glyph.BG)
				prevFG = glyph.FG
				prevBG = glyph.BG
			}
			// TODO: Handle attributes such as bold, italic, underline, etc. using glyph.Mode
			result += string(glyph.Char)
		}
		if y < rows-1 {
			result += "\r\n"
		}
	}
	c := t.Cursor()
	result += fmt.Sprintf("\x1b[%d;%dH", c.Y+1, c.X+1)

	pane.peer.logger.Infof("Sending %d bytes of screen dump", len(result))
	return []byte(result)
}

func printColorChange(fg, bg vt10x.Color) string {
	ret := ""
	if fg == vt10x.DefaultFG {
		ret += "\x1b[39m"
	} else {
		fgR := (fg & 0xff0000) >> 16
		fgG := (fg & 0x00ff00) >> 8
		fgB := (fg & 0x0000ff)
		ret += fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fgR, fgG, fgB)
	}
	if bg == vt10x.DefaultBG {
		ret += "\x1b[49m"
	} else {
		bgR := (bg & 0xff0000) >> 16
		bgG := (bg & 0x00ff00) >> 8
		bgB := (bg & 0x0000ff)
		ret += fmt.Sprintf("\x1b[48;2;%d;%d;%dm", bgR, bgG, bgB)
	}
	return ret
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
				d.Send(pane.dumpVT())
			})
		} else {
			logger.Warn("not restoring as st is null")
		}
	} else {
		logger.Infof("Sending history buffer since marker: %d", marker)
		time.AfterFunc(time.Second/10, func() {
			d.Send(pane.Buffer.GetSinceMarker(marker))
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
