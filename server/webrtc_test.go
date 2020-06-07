package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"reflect"
	"strconv"
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
func TestTerminalChannelWrite(t *testing.T) {
	to := test.TimeOut(time.Second * 3)
	defer to.Stop()
}
func TestSimpleEcho(t *testing.T) {
	// to := test.TimeOut(time.Second * 3)
	// defer to.Stop()
	done := make(chan bool)
	server, err := NewWebRTCServer()
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	peer := server.Listen("")

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("echo hello world", nil)
	dc.OnOpen(func() {
		fmt.Println("Channel opened")
	})
	// count the incoming messages
	count := 0
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// first get a channel Id and then a the hello world text
		log.Printf("got message: #%d %q", count, string(msg.Data))
		if count == 0 {
			_, err = strconv.Atoi(string(msg.Data))
			if err != nil {
				t.Fatalf("Failed to cover channel it to int: %v", err)
				done <- true
			}
			count++
		} else if count == 1 {
			if !msg.IsString && string(msg.Data) != "hello world\r\n" {
				t.Fatalf("Got bad msg: %q", msg.Data)
				done <- true
			}
			count++
		}
	})
	dc.OnClose(func() {
		fmt.Println("Client Data channel closed")
		done <- true
	})
	signalPair(client, peer.pc)
	<-done
	if count != 2 {
		t.Fatalf("Expected to recieve 2 messages and got %d", count)
	}
}
func TestMultiLine(t *testing.T) {
	done := make(chan bool)
	server, err := NewWebRTCServer()
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	peer := server.Listen("")
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("cat <<EOF", nil)
	dc.OnOpen(func() {
		// dc.Send([]byte("123\n456\nEOF\n"))
		dc.Send([]byte("123\r\n"))
		dc.Send([]byte("456\r\n"))
		dc.Send([]byte("EOF\r\n"))
		fmt.Println("Finished sending")
	})
	var mockedMsgs []string
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Got dc msg: %q", string(msg.Data))
		data := string(msg.Data)
		mockedMsgs = append(mockedMsgs, data)

	})
	dc.OnClose(func() {
		fmt.Println("On closing the data channel")
		// time.sleep(1*time.Second)
		done <- true
	})
	signalPair(client, peer.pc)
	<-done
	server.Shutdown()

	if len(mockedMsgs) != 3 {
		t.Fatalf("Got wrong number of messages: %d", len(mockedMsgs))
	}
	if mockedMsgs[1] != "123\r\n" {
		t.Fatalf("Expected a diffrent msg and got: %v", mockedMsgs[1])
	}
	if mockedMsgs[2] != "456\r\n" {
		t.Fatalf("Expected a diffrent msg and got: %v", mockedMsgs[2])
	}
}

func TestControlChannel(t *testing.T) {
	to := test.TimeOut(time.Second * 3)
	defer to.Stop()
	done := make(chan bool)
	gotError := make(chan bool)
	gotChannelId := make(chan bool)
	server, err := NewWebRTCServer()
	if err != nil {
		t.Fatalf("Failed to start a new WebRTC server %v", err)
	}
	peer := server.Listen("")

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new peer connection %v", err)
	}
	signalPair(client, peer.pc)
	cdc, err := client.CreateDataChannel("%", nil)
	if err != nil {
		t.Fatalf("failed to create the control data channel: %v", err)
	}
	cdc.OnOpen(func() {
		// control channel is open let's open another one, so we'll have
		// what to resize
		dc, err := client.CreateDataChannel("12x34 bash", nil)
		if err != nil {
			t.Fatalf("failed to create the a channel: %v", err)
		}
		// channelId hold the ID of the channel as recieved from the server
		var channelId int
		count := 0
		dc.OnOpen(func() {
			// First message in is the server id for this channel
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				/*
					if !msg.IsString {
						t.Errorf("Got a message that's not a string: %q", msg.Data)
					}
				*/
				if count == 0 {
					channelId, err = strconv.Atoi(string(msg.Data))
					if err != nil {
						t.Errorf("Expected an int id as first message: %v", err)
					}
					gotChannelId <- true
					count++
				}

			})
		})
		<-gotChannelId
		cdc.Send([]byte(`
{
    "time": 1589355555.147976,
	"message_id": 123,
    "resize_pty": {
        "id": "kkk` + string(channelId) + `",
        "sx": 80,
        "sy": 24
    }
}`))

		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var m CTRLMessage
			if !msg.IsString {
				t.Errorf("Got a message that's not a string: %q", msg.Data)
			}
			json.Unmarshal(msg.Data, &m)
			if m.Error == nil {
				t.Errorf("Expected a layout and got: %q", msg.Data)
			}

			if m.Error.Ref != 123 {
				t.Errorf("Expected to get an error regrading 123 instead %v", m.Error.Ref)
			}
			dc.Send([]byte("exit\n"))
			gotError <- true
		})

		dc.OnClose(func() {
			fmt.Println("Client Data channel closed")
			done <- true
		})
	})
	<-done
	<-gotError
	server.Shutdown()
}
