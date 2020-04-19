package webexec

import (
    "bufio"
    "strings"
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/afittestide/webexec/signal"
	"github.com/pion/webrtc/v2"
)
type TextSender func(msg string) error 

func ReadNSend(r io.Reader, textSender TextSender) error { 
    scanner := bufio.NewScanner(r)
    if err := scanner.Err(); err != nil {
        return fmt.Errorf("tmux output scanner error: %s\n", err)
    }
    for scanner.Scan() {
        msg := scanner.Text()
        log.Printf("<< Sending %q\n", msg)
        // Send the msg as text
        err := textSender(msg)
        if err != nil {
            return fmt.Errorf("Sening msg %q failed: %s", msg, err)
        }
    }
    log.Printf("<< ReadNSend finished")
    return nil
}

func NewServer(config webrtc.Configuration) (pc *webrtc.PeerConnection, err error) {
	pc, err = webrtc.NewPeerConnection(config)
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
        if d.Label() == "signaling" {
            return
        }
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())
		// Register channel opening handling

        var cmdStdin io.Writer
        stdinReady := make(chan bool, 1)
		d.OnOpen(func() {
			log.Printf("Data channel '%s'-'%d' open.\n", d.Label(), d.ID())
            command := strings.Split(d.Label(), " ")
            log.Printf("preparing command %q", command)
            cmd := exec.Command(command[0], command[1:]...)
            cmdStdin, err = cmd.StdinPipe()
            if err != nil {
                log.Panicf("failed to open cmd stdin: %v", err)
            }
            stdinReady<-true
            stdout, err := cmd.StdoutPipe()
            if err != nil {
                log.Panicf("failed to open cmd stdout: %v", err)
            }
            cmd.Start()
            log.Println(">> command started")
            ReadNSend(stdout, d.SendText)
            log.Println("Finished reading the commnd output")
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			data := string(msg.Data)
            log.Printf(">> recieved: %q ", data)
            if cmdStdin == nil {
                <-stdinReady
            }
            io.WriteString(cmdStdin, data)

		})
        // err = cmd.Wait()
        // if err != nil {
            // log.Panicf("cmd.Wait returned: %v", err)
        // }
	})
	return pc, nil
}

func main() {
	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &offer)

	// Set the remote SessionDescription
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	pc, err := NewServer(config)
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
