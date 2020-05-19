package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/pion/transport/test"
	"github.com/pion/webrtc/v2"
)

//TODO: move this function to a test_utils.go file
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
		return fmt.Errorf("timed mockedMsgs waiting to receive offer")
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

var mockedMsgs []string

func mockSender(msg []byte) error {
	fmt.Printf("Mock sending - %v", msg)
	mockedMsgs = append(mockedMsgs, string(msg))
	return nil
}
func TestCat(t *testing.T) {
	cmd := exec.Command("cat", "<<EOF")
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Panicf("failed to open cmd stdin: %v", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Panicf("failed to open cmd stdout: %v", err)
	}
	cmd.Start()
	in.Write([]byte("Hello\n"))
	in.Write([]byte("World\nEOF\n"))
	b := make([]byte, 1024)
	var r []byte
	for {
		l, e := out.Read(b)
		if e == io.EOF {
			break
		}
		if l > 0 {
			r = append(r, b[:l]...)
		}
	}
	if reflect.DeepEqual(r, []byte("Hello\nWorld")) {
		t.Fatalf("got wrong stdout: %v", r)
	}
}
func TestSimpleEcho(t *testing.T) {
	done := make(chan bool)
	server, err := NewWebRTCServer()
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
		if !msg.IsString && string(msg.Data) != "hello world\n" {
			t.Fatalf("Got bad msg: %q", msg.Data)
		}
	})
	dc.OnClose(func() {
		fmt.Println("Client Data channel closed")
		done <- true
	})
	signalPair(client, server.pc)
	<-done
}
func TestMultiLine(t *testing.T) {
	done := make(chan bool)
	server, err := NewWebRTCServer()
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("cat", nil)
	dc.OnOpen(func() {
		// dc.Send([]byte("123\n456\nEOF\n"))
		dc.Send([]byte("123\n"))
		dc.Send([]byte("456\n"))
		time.Sleep(1 * time.Second)
		dc.Close()
		fmt.Println("Finished sending")
	})
	var mockedMsgs []string
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := string(msg.Data)
		mockedMsgs = append(mockedMsgs, data)
	})
	dc.OnClose(func() {
		fmt.Println("On closing the data channel")
		done <- true

	})
	signalPair(client, server.pc)
	<-done
	server.Shutdown()

	if len(mockedMsgs) == 1 && mockedMsgs[0] != "123\n456\n" {
		t.Fatalf("Got one, wrong message  %v", mockedMsgs)
	}
	if len(mockedMsgs) == 2 && (mockedMsgs[0] != "123\n" || mockedMsgs[1] != "456\n") {
		t.Fatalf("Got two messages at least one wrong %v", mockedMsgs)
	}
}

func TestTmuxConnect(t *testing.T) {
	to := test.TimeOut(time.Second * 5)
	defer to.Stop()
	done := make(chan bool)
	gotLayout := make(chan bool)
	server, err := NewWebRTCServer()
	if err != nil {
		t.Fatalf("Failed to start a new WebRTC server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new peer connection %v", err)
	}
	dc, err := client.CreateDataChannel("tmux -CC", nil)

	dc.OnOpen(func() {
		log.Print("Sending resize message")
		dc.Send([]byte(`
{
    "version": 1,
    "time": 1589355555.147976,
    "refresh-client": {
        "width": 80,
        "height": 24
    }
}`))
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var m Terminal7Message
		if !msg.IsString {
			t.Errorf("Got a message that's not a string: %q", msg.Data)
		}
		json.Unmarshal(msg.Data, &m)
		if m.Layout == nil {
			t.Errorf("Expected a layout and got: %q", msg.Data)
		}
		layout := m.Layout[0]
		if layout.zoomed {
			t.Errorf("Got a zoomed window")
		}
		gotLayout <- true
	})

	dc.OnClose(func() {
		fmt.Println("Client Data channel closed")
		done <- true
	})
	signalPair(client, server.pc)
	<-done
	<-gotLayout
	server.Shutdown()
}
