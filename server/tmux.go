package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v2"
)

const (
	StateInit = iota
	StateNormal
	StateGettingWindows
	StateGettingPanes
)

type Terminal7Client struct {
	pty          io.ReadWriteCloser
	errors       []error
	LastTmuxTime time.Time
	Layout       *[]WindowLayout
	State        int
	Cdc          *webrtc.DataChannel
	outputBegun  bool
}

func NewTerminal7Client(pty io.ReadWriteCloser) (c Terminal7Client) {
	c = Terminal7Client{pty, nil, time.Unix(0, 0), nil, StateInit, nil, false}
	return
}

type PaneLayout struct {
}

type WindowLayout struct {
	id              string
	name            string
	zoomed          bool
	width           uint16
	height          uint16
	xoff            uint16
	yoff            uint16
	active          bool
	active_clients  int16
	active_sessions int16
	last_activity   int32
	bigger          bool
	flags           string
	index           uint16
	is_last         bool
	marked          bool
	stack_index     uint16
	layout          string
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
	Time          float64            `json:"time"`
	ResizePane    *ResizePaneArgs    `json:"resize_pane"`
	RefreshClient *RefreshClientArgs `json:"refresh_client"`
	Layout        []WindowLayout     `json:"layout"`
	// map[string]interface{}
}

func toInt(s string) uint16 {
	v, err := strconv.ParseInt(s, 0, 16)
	if err != nil {
		// log.Printf("Failed to convert field to int: %q, err: %v", s)
	}
	return uint16(v)

}
func (client *Terminal7Client) UpdateWindows(b []string) {
	log.Print(">>> Updating Terminal7Client.Layout with:")
	m := map[string]bool{"1": true, "0": false}
	for _, l := range b {
		a := strings.Split(l, ";")
		w := WindowLayout{a[0], a[1], m[a[2]],
			toInt(a[3]), toInt(a[4]), toInt(a[5]), toInt(a[6]), m[a[7]],
			toInt(a[8]), toInt(a[9]), a[10], a[11], a[12], a[13], a[14], a[15], a[16], a[17]}
		log.Print(l)
	}
	log.Print("<<<")

}
func (client *Terminal7Client) UpdatePanes(b []string) {
	log.Printf("Updating Terminal7Client.Layout with panes: \n%s",
		strings.Join(b, "\n"))
}
func (client *Terminal7Client) HandleTmuxReply(b []string) {
	if client.State == StateInit && client.Layout == nil {
		client.State = StateGettingWindows
		client.pty.Write([]byte("list-windows -F " +
			"'#{window_id};#{window_name};#{window_zoomed_flag};" +
			"#{window_width};#{window_height};#{window_offset_x};" +
			"#{window_offset_y};#{?window_active};#{window_active_clients};" +
			"#{window_active_sessions};#{window_activity};#{?window_bigger};" +
			"#{window_flags};#{window_index};#{?window_last_flag};" +
			"#{?window_marked_flag};#{window_stack_index};#{window_layout}'\n"))
	} else if client.State == StateGettingWindows {
		client.UpdateWindows(b)
		client.State = StateGettingPanes
		client.pty.Write([]byte("list-panes\n"))
	} else if client.State == StateGettingPanes {
		client.UpdatePanes(b)
		client.State = StateNormal
		m, e := json.Marshal(client.Layout)
		if e != nil {
			log.Printf("ERROR: Failed to marshal the layout: %v", e)
		}
		log.Printf("Terminal&Client is sending %q", m)
		client.Cdc.Send(m)
	}
}
func (client *Terminal7Client) updateTmuxTime(ts string) {
	var s int64
	fmt.Sscanf(ts, "%d", &s)
	t := time.Unix(s, 0)
	client.LastTmuxTime = t
}
func (client *Terminal7Client) TmuxReader(dc io.Writer) error {
	var b []string
	firstTime := true
	scanner := bufio.NewScanner(client.pty)
	for scanner.Scan() {
		l := scanner.Text()

		if firstTime {
			if l[:7] != "\x1bP1000p" {
				panic(fmt.Errorf("Got wrong 7 first chars from tmux: %q", string(l[:7])))
			}
			l = l[7:]
			firstTime = false
		}

		log.Print(l)
		if client.outputBegun && l[0] != byte('%') {
			b = append(b, l)
			continue
		}
		w := strings.Split(l, " ")
		switch w[0] {
		case "%begin":
			client.updateTmuxTime(w[1])
			log.Printf("%%begin from %v", client.LastTmuxTime)
			b = nil
			client.outputBegun = true

		case "%end":
			log.Printf("%%end at %v", client.LastTmuxTime)
			client.updateTmuxTime(w[1])
			client.HandleTmuxReply(b)

		case "%output":
		case "%session-changed":
		case "%sessions-changed":
		case "%window-renamed":
		case "%window-add":
		default:
			if len(l) > 0 {
				return fmt.Errorf("Failed to parse tmux message: %q", l)
			}
		}
	}
	return scanner.Err()
}

func (client *Terminal7Client) OnClientMessage(msg webrtc.DataChannelMessage) {
	var m Terminal7Message
	// fmt.Printf("Got a terminal7 message: %q", string(msg.Data))
	p := json.Unmarshal(msg.Data, &m)
	if m.RefreshClient != nil {
		c := fmt.Sprintf("refresh-client -F %dx%d", m.RefreshClient.sx,
			m.RefreshClient.sy)
		client.pty.Write([]byte(c))
	}

	log.Printf("< %v", p)
}
