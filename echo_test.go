package webexec

import (
	"fmt"
	"github.com/pion/webrtc/v2"
	"testing"
	"time"
    "os/exec"
    "log"
)

// expectedLabel represents the label of the data channel we are trying to test.
// Some other channels may have been created during initialization (in the Wasm
// bindings this is a requirement).
const expectedLabel = "data"

func signalPair(pcOffer *webrtc.PeerConnection, pcAnswer *webrtc.PeerConnection) error {
	iceGatheringState := pcOffer.ICEGatheringState()
	offerChan := make(chan webrtc.SessionDescription, 1)

	if iceGatheringState != webrtc.ICEGatheringStateComplete {
		pcOffer.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				offerChan <- *pcOffer.PendingLocalDescription()
			}
		})
	}
	// Note(albrow): We need to create a data channel in order to trigger ICE
	// candidate gathering in the background for the JavaScript/Wasm bindings. If
	// we don't do this, the complete offer including ICE candidates will never be
	// generated.
	if _, err := pcOffer.CreateDataChannel("signaling", nil); err != nil {
		return err
	}

	offer, err := pcOffer.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pcOffer.SetLocalDescription(offer); err != nil {
		return err
	}

	if iceGatheringState == webrtc.ICEGatheringStateComplete {
		offerChan <- offer
	}
	select {
	case <-time.After(3 * time.Second):
		return fmt.Errorf("timed out waiting to receive offer")
	case offer := <-offerChan:
		if err := pcAnswer.SetRemoteDescription(offer); err != nil {
			return err
		}

		answer, err := pcAnswer.CreateAnswer(nil)
		if err != nil {
			return err
		}

		if err = pcAnswer.SetLocalDescription(answer); err != nil {
			return err
		}

		err = pcOffer.SetRemoteDescription(answer)
		if err != nil {
			return err
		}
		return nil
	}
}

var out []string
func mockSender(msg string) error {
    out = append(out, msg)
    return nil
}
func TestAAA(t *testing.T) {
    cmd := exec.Command("echo", "hello", "world")
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        log.Panicf("failed to open cmd stdout: %v", err)
    }
    err = cmd.Start()
    if err != nil {
        t.Fatalf("Failed to run command %q", err)
    }
    err = ReadNSend(stdout, mockSender)
    if err != nil {
        t.Fatalf("ReanNSend return an err - %q", err)
    }
    if len(out) != 1 {
        t.Fatalf("Bad output len %d", len(out))
    }
    if out[0] != "hello world" {
        t.Fatalf("Bad output string %q", out[0])
    }
    
}
func TestSimpleEcho(t *testing.T) {
	done := make(chan bool)
	server, err := NewServer("password")
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	fmt.Printf("%v", server)

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("webexec", nil)
	dc.OnOpen(func() {
		err := dc.SendText("password")
		if err != nil {
			t.Fatalf("Failed to send string on data channel")
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString && string(msg.Data) != "hello world" {
			t.Fatalf("Got bad msg: %q", msg.Data)
		}
		done <- true
	})
	signalPair(client, server)
	<-done
}
