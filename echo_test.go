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
	server, err := NewServer(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("echo hello world", nil)
	dc.OnOpen(func() {
        fmt.Println("Channel opened")
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
func TestMultiMessage(t *testing.T) {
	done := make(chan bool)
	server, err := NewServer(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("cat", nil)
	dc.OnOpen(func() {
        dc.SendText("123\n")
        dc.SendText("456\n")
        dc.SendText("EOF\x04")
	})
    var out []string
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString {
			t.Fatalf("Got bad msg: %q", msg.Data)
		}
        data := string(msg.Data)
        out = append(out, data)
        if data == "EOF" {
            done <- true
        }
	})
	signalPair(client, server)
	<-done
    if len(out) != 3 {
        t.Fatalf("Wrong number of strings in out - %v", out)
    }
    if out[0] != "123" || out[1] != "456" || out[2] != "EOF" {
        t.Fatalf("Got bad output - %v", out)
    }
}
