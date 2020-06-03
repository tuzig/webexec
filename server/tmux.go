package server

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
)

const (
	StateInit = iota
	StateNormal
	StateGettingWindows
	StateGettingPanes
)

type Terminal7Client struct {
	cdc   *webrtc.DataChannel
	State int
}

func NewTerminal7Client(cdc *webrtc.DataChannel) error {
	c = Terminal7Client{cdc, StateInit, nil}
	cdc.OnMessage(c.OnControl)
	return nil
}

type PaneLayout struct {
}

type ResizePTYArgs struct {
	id string
	sx uint16
	sy uint16
}

type Terminal7Message struct {
	time      float64
	resizePTY *ResizePTYArgs `json:"resize_pty"`
}

func (client *Terminal7Client) HandleChannel(tc TerminalChannel) {
	d.OnMessage(server.terminal7ClienthandleDCMessages)
}
func (client *Terminal7Client) OnControlMessage(msg webrtc.DataChannelMessage) {
	var m Terminal7Message
	// fmt.Printf("Got a terminal7 message: %q", string(msg.Data))
	p := json.Unmarshal(msg.Data, &m)
	if m.resizePTY != nil {
		var ws pty.Winsize
		ws.Cols = m.resizePTY.sx
		ws.Rows = m.resizePTY.sy
		pty.Setsize(client.panes[m.resizePTY.id].pty, &ws)
		/*
			c := fmt.Sprintf("refresh-client -F %dx%d -t ", m.resizePTY.sx,
				m.resizePTY.sy)
			client.pty.Write([]byte(c))
		*/
	}

	log.Printf("< %v", p)
}
