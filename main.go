package main

import (
    "io"
	"fmt"
    "log"
    "bufio"
    "os/exec"

    "github.com/pion/webrtc/v2"
    "github.com/daonb/tmux4web/signal"
)

func main() {
	// Everything below is the Pion WebRTC API! Thanks for using it ❤️.

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})

	// Register data channel creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())
        cmd := exec.Command("tmux", "-C", "-L", "tmux4web", "attach")
        stdin, err := cmd.StdinPipe()
        if err != nil {
            log.Fatal(err)
        }
        stdout, err := cmd.StdoutPipe()
        if err != nil {
            log.Fatal(err)
        }

		// Register channel opening handling
		d.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open. Attaching tmux.\n",d.Label(), d.ID())
            err := cmd.Start()
            if err != nil {
                log.Fatal(err)
            }
            scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
                message := scanner.Text()

				fmt.Printf("Sending '%s'\n", message)

				// Send the message as text
				sendErr := d.SendText(message)
				if sendErr != nil {
					panic(sendErr)
				}
			}
            if err := scanner.Err(); err != nil {
                fmt.Printf("tmux output scanner error: %s\n", err)
            }
		})

		// Register text message handling
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
            data := string(msg.Data)
			fmt.Printf("Message from DataChannel '%s': '%s'\n", d.Label(), data)
            io.WriteString(stdin, data + "\n")
		})
	})

	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &offer)

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(answer))

	// Block forever
	select {}
}
