package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/pion/webrtc/v2"
)

type dataChannelPipe struct {
	d *webrtc.DataChannel
}

func (pipe *dataChannelPipe) Write(p []byte) (n int, err error) {
	pipe.d.SendText(string(p))
	log.Printf("> %v", p)
	return len(p), nil
}

func NewWebRTCServer(config webrtc.Configuration) (pc *webrtc.PeerConnection, err error) {
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
		var cmdStdin io.Writer
		var cmd *exec.Cmd
		var stderr bytes.Buffer
		pipe := dataChannelPipe{d}
		cmdReady := make(chan bool, 1)
		d.OnOpen(func() {
			l := d.Label()
			log.Printf("New Data channel %q\n", l)
			c := strings.Split(l, " ")
			cmd = exec.Command(c[0], c[1:]...)
			cmdStdin, err = cmd.StdinPipe()
			if err != nil {
				log.Panicf("failed to open cmd stdin: %v", err)
			}
			cmd.Stdout = &pipe
			cmd.Stderr = &stderr
			err = cmd.Start()
			if err != nil {
				log.Panicf("failed to start cmd: %v %v", err, stderr.String())
			}
			cmdReady <- true
			log.Println("Waiting for command to finish")
			err = cmd.Wait()
			if err != nil {
				log.Printf("cmd.Wait returned: %v", stderr.String())
			}
			d.Close()
		})
		d.OnClose(func() {
			// kill the command
			log.Println("Data channel closed")
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			p := msg.Data
			<-cmdReady
			log.Printf("< %v ", p)
			l, err := cmdStdin.Write(p)
			if err != nil {
				log.Printf("Stdin Write returned an error: %v", err)
			}
			if l != len(p) {
				log.Printf("stdin write wrote %d instead of %d bytes", l, len(p))
			}
			cmdReady <- true
		})
	})
	return pc, nil
}
