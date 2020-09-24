package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/stretchr/testify/require"
)

const timeout = 3 * time.Second
const ACK_REF = 123

func parseAck(t *testing.T, msg webrtc.DataChannelMessage) AckArgs {
	var args json.RawMessage
	var ackArgs AckArgs
	env := CTRLMessage{
		Args: &args,
	}
	err := json.Unmarshal(msg.Data, &env)
	require.Nil(t, err, "failed to unmarshal cdc message: %q", err)
	require.Equal(t, env.Type, "ack",
		"Expected an ack message and got %q", env.Type)
	err = json.Unmarshal(args, &ackArgs)
	require.Nil(t, err, "failed to unmarshal ack args: %q", err)
	return ackArgs
}

func sendAuth(cdc *webrtc.DataChannel, token string) {
	time.Sleep(10 * time.Millisecond)
	//TODO we need something like peer.LastMsgId++ below
	msg := CTRLMessage{
		Time:      time.Now().UnixNano(),
		Type:      "auth",
		MessageId: ACK_REF,
		Args:      AuthArgs{token},
	}
	authMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal the auth args: %v", err)
	} else {
		log.Print("Test is sending an auth message")
		cdc.Send(authMsg)
	}
}

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

/* TODO: refactor as StartCommand is no longer a thing
func TestStartCommand(t *testing.T) {
	// to := test.TimeOut(time.Second * 3)
	// defer to.Stop()
	server, err := NewWebRTCServer()
	if err != nil {
		t.Errorf("Failed to start a new server %v", err)
	}
	var c []string
	c = append(c, "echo", " badwolf")
	cmd, err := server.StartCommand(c)
	if err != nil {
		t.Errorf("Failed to start a new server %v", err)

	}
	b := make([]byte, 1024)
	_, err = cmd.Tty.Read(b)
	if err != nil {
		t.Errorf("Failed to read tty: %v", err)
	}
	if string(b[1:8]) != "badwolf" {
		t.Errorf("Expected command output 'badwolf' got %q", b[1:8])
	}
}
*/
func TestSimpleEcho(t *testing.T) {
	InitDevLogger()
	done := make(chan bool)
	gotAuthAck := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new server", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	// count the incoming messages
	count := 0
	cdc.OnOpen(func() {
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message
			var cm CTRLMessage
			log.Printf("Got a ctrl msg: %s", msg.Data)
			err := json.Unmarshal(msg.Data, &cm)
			require.Nil(t, err, "Failed to marshal the server msg: %v", err)
			if cm.Type == "ack" {
				gotAuthAck <- true
			}
		})
		<-gotAuthAck
		dc, err := client.CreateDataChannel("echo,hello world", nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			log.Printf("Channel %q opened, state: %v", dc.Label(), peer.State)
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// first get a channel Id and then "hello world" text
			log.Printf("got message: #%d %q", count, string(msg.Data))
			if count == 0 {
				_, err = strconv.Atoi(string(msg.Data))
				require.Nil(t, err, "Failed to cover channel it to int: %v", err)
			} else if count == 1 {
				require.EqualValues(t, string(msg.Data)[:11], "hello world",
					"Got bad msg: %q", msg.Data)
				done <- true
			}
			count++
		})
		dc.OnClose(func() {
			fmt.Println("Client Data channel closed")
			done <- true
		})
	})
	signalPair(client, peer.PC)
	// TODO: add timeout
	<-done
	require.Equal(t, count, 2, "Expected to recieve 2 messages and got %d", count)
}

func TestUnauthincatedBlocked(t *testing.T) {
	InitDevLogger()
	done := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection: %q", err)
	signalPair(client, peer.PC)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %q", err)
	cdc.OnOpen(func() {
		// control channel is open let's open another one, so we'll have
		// what to resize
		dc, err := client.CreateDataChannel("bash", nil)
		require.Nil(t, err, "failed to create the a channel: %q", err)
		dc.OnClose(func() {
			require.Equal(t, dc.Label(), "bash",
				"Got a close request for channel: %q", dc.Label())
			// First message in is the server id for this channel
		})
	})

	time.AfterFunc(3*time.Second, func() {
		done <- true
	})
	<-done
	Shutdown()
}

func TestAuthCommand(t *testing.T) {
	InitDevLogger()
	gotAuthAck := make(chan bool)
	gotTokenAck := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection %q", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %q", err)

	cdc.OnOpen(func() {
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ackArgs := parseAck(t, msg)
			require.Equal(t, ackArgs.Ref, ACK_REF,
				"Expeted ack ref to equal %d and got: ", ACK_REF, ackArgs.Ref)
			gotAuthAck <- true
		})

	})
	signalPair(client, peer.PC)
	<-gotAuthAck
	log.Printf("Got Auth Ack")
	// got auth ack now close the channel and start over, this time using
	// the token
	// TODO: remove the next block of code as tokens are different
	client.Close()
	Shutdown()
	require.Nil(t, err, "Failed to start a new WebRTC server %v", err)
	peer2, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err = webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection %v", err)
	cdc, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var cm CTRLMessage
			log.Printf("Got a ctrl msg: %s", msg.Data)
			err := json.Unmarshal(msg.Data, &cm)
			require.Nil(t, err, "Failed to marshal the server msg: %v", err)
			require.Equal(t, cm.Type, "ack",
				"Expeted an Ack message and got: %v", msg.Data)
			gotTokenAck <- true
		})
	})
	signalPair(client, peer2.PC)
	<-gotTokenAck
}

func TestResizeCommand(t *testing.T) {
	InitDevLogger()
	gotAuthAck := make(chan bool)
	done := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection %v", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ack := parseAck(t, msg)
			if ack.Ref == ACK_REF {
				gotAuthAck <- true
			}
			if ack.Ref == 456 {
				done <- true
			}
		})
		<-gotAuthAck
		// control channel is open let's open another one, so we'll have
		// what to resize
		dc, err := client.CreateDataChannel("12x34,bash", nil)
		require.Nil(t, err, "failed to create the a channel: %v", err)
		// paneID hold the ID of the channel as recieved from the server
		paneID := -1
		dc.OnOpen(func() {
			log.Println("Data channel is open")
			// send something to get the channel going
			// dc.Send([]byte{'#'})
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("Got data channel message: %q", string(msg.Data))
				if paneID == -1 {
					paneID, err = strconv.Atoi(string(msg.Data))
					require.Nil(t, err,
						"Got a bad first message: %q", string(msg.Data))
					resizeArgs := ResizeArgs{paneID, 80, 24}
					m := CTRLMessage{time.Now().UnixNano(), 456, "resize",
						&resizeArgs}
					resizeMsg, err := json.Marshal(m)
					require.Nil(t, err, "failed marshilng ctrl msg: %v", msg)
					log.Println("Sending the resize message")
					cdc.Send(resizeMsg)
				}

			})
		})
	})
	signalPair(client, peer.PC)
	<-done
}

func TestChannelReconnect(t *testing.T) {
	InitDevLogger()
	var cId string
	var dc *webrtc.DataChannel
	done := make(chan bool)
	gotAuthAck := make(chan bool)
	gotId := make(chan bool)
	// start the server
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	// and the client
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new server %v", err)
	// create the command & control data channel
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	// count the incoming messages
	count := 0
	cdc.OnOpen(func() {
		log.Println("cdc is opened")
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message
			var cm CTRLMessage
			log.Printf("Got a ctrl msg: %s", msg.Data)
			err := json.Unmarshal(msg.Data, &cm)
			require.Nil(t, err, "Failed to marshal the server msg: %v", err)
			if cm.Type == "ack" {
				gotAuthAck <- true
			}
		})
		<-gotAuthAck
		dc, err = client.CreateDataChannel("24x80,bash,-c,sleep 1; echo 123456", nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			log.Printf("Channel %q opened, state: %v", dc.Label(), peer.State)
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("DC Got msg #%d: %s", count, msg.Data)
			if count == 0 {
				cId = string(msg.Data)
				log.Printf("Client got a channel id: %q", cId)
				dc.Close()
				gotId <- true
			}
			count++
		})
	})
	signalPair(client, peer.PC)
	<-gotId
	// Now that we have a channel open, let's close the channel and reconnect
	dc2, err := client.CreateDataChannel("24x80,>"+cId, nil)
	require.Nil(t, err, "Failed to create the second data channel: %q", err)
	dc2.OnOpen(func() {
		log.Printf("Second channel is open.  state: %q", peer.State)
	})
	count2 := 0
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("DC2 Got msg #%d: %s", count2, msg.Data)
		// first message is the pane id
		if count2 == 0 && string(msg.Data) != cId {
			t.Errorf("Got a bad pane ID on reconnect, expected %q got %q",
				cId, msg.Data)
		}
		// second message should be the echo output
		if count2 == 1 {
			if !strings.Contains(string(msg.Data), "123456") {
				t.Errorf("Got an unexpected reply: %s", msg.Data)
			}
			log.Print("I'm done")
			done <- true
		}
		count2++
	})
	log.Print("Waiting on done")
	time.AfterFunc(3*time.Second, func() {
		t.Error("Timeout waiting for dat ain reconnected pane")
		done <- true
	})
	<-done
	// dc.Close()
	// dc2.Close()
}
func TestPayloadOperations(t *testing.T) {
	InitDevLogger()
	done := make(chan bool)
	gotAuthAck := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new server", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	Payload = []byte("[\"This is a simple payload\"]")
	payload2 := []byte("[\"Better payload\"]")
	cdc.OnOpen(func() {
		// there's only one payload
		// TODO: support sessions and multi payloads
		go sendAuth(cdc, "thejargonfile")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message and the get payload
			log.Printf("Got a ctrl msg: %s", msg.Data)
			args := parseAck(t, msg)
			var payload []byte
			err = json.Unmarshal(args.Body, &payload)
			if args.Ref == ACK_REF {
				require.Equal(t, []byte(args.Body), Payload,
					"Got the wrong payload: %q", args.Body)
				gotAuthAck <- true
			}
			if args.Ref == 777 {
				require.Equal(t, []byte(args.Body), payload2,
					"Got the wrong payload: %q", args.Body)
				done <- true
			}
		})
	})
	time.AfterFunc(3*time.Second, func() {
		t.Error("Timeout waiting for an ack")
		done <- true
	})
	signalPair(client, peer.PC)
	<-gotAuthAck
	args := SetPayloadArgs{payload2}
	setPayload := CTRLMessage{time.Now().UnixNano(), 777,
		"set_payload", &args}
	getMsg, err := json.Marshal(setPayload)
	if err != nil {
		log.Printf("Failed to marshal the auth args: %v", err)
	} else {
		log.Print("Test is sending an auth message")
		cdc.Send(getMsg)
	}
	<-done
}
