package server

import (
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

type TerminalChannel struct {
	dc  *webrtc.DataChannel
	cmd *exec.Cmd
	pty *os.File
}

type WebRTCServer struct {
	c        webrtc.Configuration
	CmdReady chan bool
	cmd      *exec.Cmd
	pc       *webrtc.PeerConnection
	// channels holds all the open channel we have with process ID as key
	channels map[string]*TerminalChannel
}

type DataChannelPipe struct {
	d *webrtc.DataChannel
}

var SET_SIZE_PREFIX = "A($%JFDS*(;dfjmlsdk9-0"

func (pipe *DataChannelPipe) Write(p []byte) (n int, err error) {
	text := string(p)
	// TODO: logging...
	if true {
		for _, r := range strings.Split(text, "\r\n") {
			if len(r) > 0 {
				log.Printf("> %q\n", r)
			}
		}
	}
	pipe.d.SendText(text)
	return len(p), nil
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
		pipe := DataChannelPipe{d}
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
			if c[0] == "terminal7" {
				err := NewTerminal7Client(d)
				if err != nil {
					log.Panicf("Failed to open a new terminal 7 client %v", err)

				}
				return
			}
			cmd := exec.Command(c[0], c[1:]...)
			ptmux, err = pty.Start(server.cmd)
			if err != nil {
				log.Panicf("Failed to start pty: %v", err)
			}
			// create the channel and add to the server map
			serverId := string(cmd.Process.Pid)
			server.channels[serverId] = &TerminalChannel{d, cmd, ptmux}
			d.OnMessage(server.channels[serverId].OnMessage)
			_, err = io.Copy(&pipe, ptmux)
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

func (server *WebRTCServer) start(remote string) []byte {
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
func (channel *TerminalChannel) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	log.Printf("< %v", p)
	// <-server.CmdReady
	l, err := channel.pty.Write(p)
	if err != nil {
		log.Panicf("Stdin Write returned an error: %v", err)
	}
	if l != len(p) {
		log.Panicf("stdin write wrote %d instead of %d bytes", l, len(p))
	}
	// server.CmdReady <- true
}
func (server *WebRTCServer) Shutdown() {
	server.pc.Close()
	if server.cmd != nil {
		log.Print("Shutting down WebRTC server")
		server.cmd.Process.Kill()
	}
}
