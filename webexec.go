package webexec

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/afittestide/webexec/signal"
	"github.com/pion/webrtc/v2"
)

func NewServer(password string) (pc *webrtc.PeerConnection, err error) {
	// Everything below is the Pion WebRTC API
	// Prepare the configuration
	// config := webrtc.Configuration{
	//	ICEServers: []webrtc.ICEServer{
	//		{
	//			URLs: []string{"stun:stun.l.google.com:19302"},
	//		},
	//	},
	// }
	// config = webrtc.Configuration
	// Create a new RTCPeerConnection
	// pc, err := webrtc.NewPeerConnection(config)
	pc, err = webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("Failed to open peer connection: %q", err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	// Register data channel creation handling
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())
		// Register channel opening handling
		d.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open. Attaching tmux.\n", d.Label(), d.ID())
		})

		// Register text message handling
		state := "init"
		cmd := exec.Command("tmux")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			log.Panicf("failed to open cmd tmux  stdin: %v", err)
		}
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			data := string(msg.Data)
			switch state {
			case "init":
				// start with a password
				if data != password {
					log.Panicf("Received the wrong password")
				}
				log.Printf(">> password checks, switching to 'sauthorized'")
				state = "authorized"

			case "authorized":
				// We should do this: cmd := exec.Command(data)
				/// and then set stdin
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					log.Panicf("failed to open cmd %q stdout: %v", data, err)
				}
				scanner := bufio.NewScanner(stdout)
				if err := scanner.Err(); err != nil {
					fmt.Printf("tmux output scanner error: %s\n", err)
				}
				for scanner.Scan() {
					message := scanner.Text()
					log.Printf(">> Sending %q\n", message)
					// Send the message as text
					sendErr := d.SendText(message)
					if sendErr != nil {
						log.Panicf("Sening message %q failed: %s", message, err)
					}
				}
				state = "running"

			case "running":
				log.Printf(">> %q recieved: %q ", d.Label(), data)
				io.WriteString(stdin, data)

			default:
				log.Panicf("Recieved a message in a bad state: %q", state)
			}
		})
	})
	return pc, nil
}

func main() {
	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &offer)

	// Set the remote SessionDescription
	pc, err := NewServer("password-should-be-arg")
	if err != nil {
		panic(err)
	}
	err = pc.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Create an answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = pc.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(answer))

	// Block forever
	select {}
}
