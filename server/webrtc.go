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

type WebRTCServer struct {
	c        webrtc.Configuration
	CmdReady chan bool
	ptmux    *os.File
	cmd      *exec.Cmd
	pc       *webrtc.PeerConnection
	ctrlDC   *webrtc.DataChannel
	paneDC   []webrtc.DataChannel
}

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 3 * time.Second

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
			server.ctrlDC = d
			l := d.Label()
			log.Printf("New Data channel %q\n", l)
			c := strings.Split(l, " ")
			server.cmd = exec.Command(c[0], c[1:]...)
			server.ptmux, err = pty.Start(server.cmd)
			if err != nil {
				log.Panicf("Failed to attach a ptyi and start cmd: %v", err)
			}
			defer func() { _ = server.ptmux.Close() }() // Best effort.
			// server.CmdReady <- true
			if c[0] == "tmux" && c[1] == "-CC" {
				c := NewTerminal7Client(server.ptmux)
				d.OnMessage(c.OnClientMessage)
				err = c.TmuxReader(&pipe)
				if err != nil {
					log.Printf("tmux reader failed: %v", err)
				}
			} else {
				d.OnMessage(server.handleDCMessages)
				_, err = io.Copy(&pipe, server.ptmux)
				if err != nil {
					log.Printf("Failed to copy from command: %v %v", err,
						server.cmd.ProcessState.String())
				}
			}
			server.ptmux.Close()
			err = server.cmd.Process.Kill()
			if err != nil {
				log.Printf("Failed to kill process: %v %v", err, server.cmd.ProcessState.String())
			}
			d.Close()
			// TODO: do we ever need to pc.Close() ?
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
func (server *WebRTCServer) handleDCMessages(msg webrtc.DataChannelMessage) {
	p := msg.Data
	log.Printf("< %v", p)
	// <-server.CmdReady
	if string(p[:len(SET_SIZE_PREFIX)]) == SET_SIZE_PREFIX {
		var ws pty.Winsize
		json.Unmarshal(msg.Data[len(SET_SIZE_PREFIX):], &ws)
		log.Printf("New size - %v", ws)
		pty.Setsize(server.ptmux, &ws)
	} else {
		l, err := server.ptmux.Write(p)
		if err != nil {
			log.Printf("Stdin Write returned an error: %v %v", err, server.cmd.ProcessState.String())
		}
		if l != len(p) {
			log.Printf("stdin write wrote %d instead of %d bytes", l, len(p))
		}
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
