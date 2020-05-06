package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
)

type dataChannelPipe struct {
	d *webrtc.DataChannel
}

func (pipe *dataChannelPipe) Write(p []byte) (n int, err error) {
	text := string(p)
	// TODO:ogging...
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

func NewWebRTCServer(config webrtc.Configuration) (pc *webrtc.PeerConnection, err error) {
	SET_SIZE_PREFIX := "A($%JFDS*(;dfjmlsdk9-0"

	pc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to open peer connection: %q", err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	// Register data channel creation handling
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		if d.Label() == "signaling" {
			return
		}
		var cmd *exec.Cmd
		var ptmx *os.File
		pipe := dataChannelPipe{d}
		cmdReady := make(chan bool, 1)
		d.OnOpen(func() {
			l := d.Label()
			log.Printf("New Data channel %q\n", l)
			c := strings.Split(l, " ")
			cmd = exec.Command(c[0], c[1:]...)
			ptmx, err = pty.Start(cmd)
			if err != nil {
				log.Panicf("Failed to attach a ptyi and start cmd: %v", err)
			}
			defer func() { _ = ptmx.Close() }() // Best effort.
			cmdReady <- true
			_, err = io.Copy(&pipe, ptmx)
			if err != nil {
				log.Printf("Failed to copy from pty: %v %v", err, cmd.ProcessState.String())
			}
			ptmx.Close()
			err = cmd.Process.Kill()
			if err != nil {
				log.Printf("Failed to kill process: %v %v", err, cmd.ProcessState.String())
			}
			d.Close()
			// TODO: do we ever need to pc.Close() ?
		})
		d.OnClose(func() {
			err = cmd.Process.Kill()
			if err != nil {
				log.Printf("Failed to kill process: %v %v", err, cmd.ProcessState.String())
			}
			log.Println("Data channel closed")
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			p := msg.Data
			log.Printf("< %v", p)
			<-cmdReady
			// l, err := ptmx.Write([]byte("ls\n"))
			if string(p[:len(SET_SIZE_PREFIX)]) == SET_SIZE_PREFIX {
				var ws pty.Winsize
				json.Unmarshal(msg.Data[len(SET_SIZE_PREFIX):], &ws)
				log.Printf("New size - %v", ws)
				pty.Setsize(ptmx, &ws)
			} else {
				l, err := ptmx.Write(p)
				if err != nil {
					log.Printf("Stdin Write returned an error: %v %v", err, cmd.ProcessState.String())
				}
				if l != len(p) {
					log.Printf("stdin write wrote %d instead of %d bytes", l, len(p))
				}
			}
			cmdReady <- true
		})
	})
	return pc, nil
}
