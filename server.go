package webexec

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/pion/webrtc/v2"
)

type bytesSender func([]byte) error

func readNSend(r io.Reader, s bytesSender) error {
	b := make([]byte, 1024)
	for { // r.isOpen?
		l, e := r.Read(b)
		d := b[:l]
		log.Printf("> %v", d)
		if e == io.EOF {
			log.Printf("<< readNSend finished")
			return nil
		}
		if e != nil {
			return fmt.Errorf("Read failed: %s", e)
		}
		if l > 0 {
			e := s(d)
			if e != nil {
				return fmt.Errorf("Sening msg %q failed: %s", d, e)
			}
		}
	}
}

func NewServer(config webrtc.Configuration) (pc *webrtc.PeerConnection, err error) {
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
		log.Printf("New DataChannel %s %d\n", d.Label(), d.ID())
		// Register channel opening handling

		var cmdStdin io.Writer
		cmdReady := make(chan bool, 1)
		d.OnOpen(func() {
			l := d.Label()
			log.Printf("Data channel '%s'-'%d' open.\n", l, d.ID())
			c := strings.Split(l, " ")
			cmd := exec.Command(c[0], c[1:]...)
			cmdStdin, err = cmd.StdinPipe()
			if err != nil {
				log.Panicf("failed to open cmd stdin: %v", err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				log.Panicf("failed to open cmd stdout: %v", err)
			}
			cmdReady <- true
			cmd.Start()
			log.Println(">> command started")
			cmdStdin.Write([]byte("123\n456\nEOF\n"))
			err = readNSend(stdout, d.Send)
			if err != nil {
				log.Panicf("readNSend ended with an error: %v", err)
			}
			d.Close()
		})
		d.OnClose(func() {
			// kill the command
			log.Println("Data channel closed")
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			p := msg.Data
			if cmdStdin == nil {
				log.Printf("< [First message]")
				<-cmdReady
			}
			log.Printf("< %v ", p)
			l, err := cmdStdin.Write(p)
			if err != nil {
				log.Printf("Stdin Write returned an error: %v", err)
			}
			if l != len(p) {
				log.Printf("stdin write wrote %d instead of %d bytes", l, len(p))
			}

		})
		// err = cmd.Wait()
		// if err != nil {
		// log.Panicf("cmd.Wait returned: %v", err)
		// }
	})
	return pc, nil
}
