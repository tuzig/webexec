// Package server holds the code that runs a webrtc based service
// connecting commands with datachannels thru a pseudo tty.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/afittestide/webexec/signal"
	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
)

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 3 * time.Second

// type WebRTCServer is the singelton we use to store server globals
type WebRTCServer struct {
	c   webrtc.Configuration
	cmd *exec.Cmd
	pc  *webrtc.PeerConnection
	// channels holds all the open channel we have with process ID as key
	channels map[string]*TerminalChannel
}

func NewWebRTCServer() (server WebRTCServer, err error) {
	// Create a new API with a custom logger
	// This SettingEngine allows non-standard WebRTC behavior
	s := webrtc.SettingEngine{}
	s.SetConnectionTimeout(connectionTimeout, keepAliveInterval)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	server = WebRTCServer{c: config}
	//TODO: call func (e *SettingEngine) SetEphemeralUDPPortRange(portMin, portMax uint16)
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		err = fmt.Errorf("Failed to open peer connection: %q", err)
		return
	}
	server.pc = pc
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		s := connectionState.String()
		log.Printf("ICE Connection State change: %s\n", s)
		if s == "connected" {
			// TODO add initialization code
		}
	})
	// Register data channel creation handling
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		if d.Label() == "signaling" {
			return
		}
		d.OnOpen(func() {
			var ptmux *os.File

			l := d.Label()
			log.Printf("New Data channel %q\n", l)
			c := strings.Split(l, " ")
			if err != nil {
				log.Panicf("Failed to attach a ptyi and start cmd: %v", err)
			}
			defer func() { _ = ptmux.Close() }() // Best effort.
			// We get "terminal7" in c[0] as the first channel name
			// from a fresh client. This dc is used for as ctrl channel
			if c[0] == "%" {
				d.OnMessage(server.OnCTRLMsg)
				return
			}
			cmd := exec.Command(c[0], c[1:]...)
			ptmux, err = pty.Start(server.cmd)
			if err != nil {
				log.Panicf("Failed to start pty: %v", err)
			}
			// create the channel and add to the server map
			serverId := string(cmd.Process.Pid)
			channel := TerminalChannel{d, cmd, ptmux}
			server.channels[serverId] = &channel
			d.OnMessage(channel.OnMessage)
			_, err = io.Copy(&channel, ptmux)
			if err != nil {
				log.Printf("Failed to copy from command: %v %v", err,
					server.cmd.ProcessState.String())
			}
			ptmux.Close()
			err = server.cmd.Process.Kill()
			if err != nil {
				log.Printf("Failed to kill process: %v %v",
					err, server.cmd.ProcessState.String())
			}
			d.Close()
		})
		d.OnClose(func() {
			err = server.cmd.Process.Kill()
			if err != nil {
				log.Printf("Failed to kill process: %v %v", err, server.cmd.ProcessState.String())
			}
			log.Println("Data channel closed")
		})
	})
	return
}

// Listen
func (server *WebRTCServer) Listen(remote string) []byte {
	offer := webrtc.SessionDescription{}
	signal.Decode(remote, &offer)
	err := server.pc.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}
	answer, err := server.pc.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = server.pc.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}
	return []byte(signal.Encode(answer))
}

// Shutdown is called when it's time to go.
// Sweet dreams.
func (server *WebRTCServer) Shutdown() {
	server.pc.Close()
	if server.cmd != nil {
		log.Print("Shutting down WebRTC server")
		server.cmd.Process.Kill()
	}
}

// OnCTRLMsg handles incoming control messages
func (server *WebRTCServer) OnCTRLMsg(msg webrtc.DataChannelMessage) {
	var m CTRLMessage
	// fmt.Printf("Got a terminal7 message: %q", string(msg.Data))
	p := json.Unmarshal(msg.Data, &m)
	if m.resizePTY != nil {
		var ws pty.Winsize
		ws.Cols = m.resizePTY.sx
		ws.Rows = m.resizePTY.sy
		pty.Setsize(server.channels[m.resizePTY.id].pty, &ws)
	}
	// TODO: add more commands here: mouse, clipboard, etc.
	log.Printf("< %v", p)
}

// ErrorArgs is a type that holds the args for an error message
type ErrorArgs struct {
	Description string
	// Ref holds the message id the error refers to or 0 for system errors
	Ref uint32
}

// ResizePTYArgs is a type that holds the argumnet to the resize pty command
type ResizePTYArgs struct {
	id string
	sx uint16
	sy uint16
}

// CTRLMessage type holds control messages passed over the control channel
type CTRLMessage struct {
	time      float64
	resizePTY *ResizePTYArgs `json:"resize_pty"`
	Error     *ErrorArgs
}

// Type TerminalChannel holds the holy trinity: a data channel, a command and
// a pseudo tty.
type TerminalChannel struct {
	dc  *webrtc.DataChannel
	cmd *exec.Cmd
	pty *os.File
}

// Write send a buffer of data over the data channel
// TODO: rename this function, we use Write because of io.Copy
func (channel *TerminalChannel) Write(p []byte) (n int, err error) {
	text := string(p)
	// TODO: logging...
	if true {
		for _, r := range strings.Split(text, "\r\n") {
			if len(r) > 0 {
				log.Printf("> %q\n", r)
			}
		}
	}
	channel.dc.SendText(text)
	//TODO: can we get a truer value than `len(p)`
	return len(p), nil
}

// OnMessage is called on incoming messages from the data channel.
// It simply write the recieved data to the pseudo tty
func (channel *TerminalChannel) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	log.Printf("< %v", p)
	l, err := channel.pty.Write(p)
	if err != nil {
		log.Panicf("Stdin Write returned an error: %v", err)
	}
	if l != len(p) {
		log.Panicf("stdin write wrote %d instead of %d bytes", l, len(p))
	}
}
