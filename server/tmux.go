package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/pion/webrtc/v2"
)

type Terminal7Client struct {
	ptmux io.File
}

type PaneLayout struct {
}
type WindowLayout struct {
	id              string
	name            string
	zoomed          bool
	sx              uint16
	sy              uint16
	xoff            uint16
	yoff            uint16
	active          bool
	active_clients  int16
	active_sessions int16
	last_activity   float64
	bigger          bool
	flags           string
	index           uint16
	is_last         bool
	marked          bool
	silent_alert    bool
	stack_index     uint16
	panes           []PaneLayout
}

type RefreshClientArgs struct {
	sx int16
	sy int16
}

type ResizePaneArgs struct {
	Id    string
	Up    int16
	Down  int16
	Right int16
	left  int16
}

type Terminal7Message struct {
	Version       int16              `json:"version"`
	Time          float64            `json:"time"` // want to change this to `json:"name"`
	ResizePane    *ResizePaneArgs    `json:"resize_pane"`
	RefreshClient *RefreshClientArgs `json:"refresh_client"`
	Layout        *[]WindowLayout    `json:"layout"`

	// map[string]interface{}
}

func (client *Terminal7Client) TmuxReader(dc io.Writer) error {
	schwantz := ""
	b := make([]byte, 1024)
	for {
		_, err := clientptmux.Read(b)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("Failed reading tmux output: %v", err)
		}

		lines := strings.Split(schwantz+string(b), "\r\n")
		for i, l := range lines {
			w := strings.Split(l, " ")
			switch w[0] {
			case "%begin":
			case "%end":
			case "%output":
			case "%window-add":
			default:
				if i == len(lines)-1 && b[len(b)-1] != 10 {
					schwantz = l
				} else {
					return fmt.Errorf("Failed to parse tmux message: %q", l)
				}
			}
		}
	}
	return nil
}

func (client *Terminal7Client) OnClientMessage(msg webrtc.DataChannelMessage) {
	var m Terminal7Message
	p := json.Unmarshal(msg.Data, &m)
	<-CmdReady
	if m.RefreshClient != nil {
		client.ptmux.Write(fmt.Sprintf("refresh-client -F %sx%s", m.RefreshClient.sx,
			m.RefreshClient.sy))
	}

	log.Printf("< %v", p)
	CmdReady <- true
}
