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
	"go.uber.org/zap/zaptest"
)

const timeout = 3 * time.Second

func TestSimpleEcho(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
	closed := make(chan bool)
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
		go SendAuth(cdc, AValidTokenForTests)
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
			log.Printf("Channel %q opened", dc.Label())
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
			}
			count++
		})
		dc.OnClose(func() {
			fmt.Println("Client Data channel closed")
			closed <- true
		})
	})
	SignalPair(client, peer.PC)
	// TODO: add timeout
	<-closed
	panes := Panes.All()
	time.Sleep(100 * time.Millisecond)
	require.False(t, panes[len(panes)-1].IsRunning)
	require.Equal(t, count, 2, "Expected to recieve 2 messages and got %d", count)
}

func TestResizeCommand(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
	gotAuthAck := make(chan bool)
	done := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection %v", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		go SendAuth(cdc, AValidTokenForTests)
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ack := ParseAck(t, msg)
			if ack.Ref == TestAckRef {
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
	SignalPair(client, peer.PC)
	<-done
}

func TestChannelReconnect(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
	var cID string
	var dc *webrtc.DataChannel
	done := make(chan bool)
	gotAuthAck := make(chan bool)
	gotID := make(chan bool)
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
		go SendAuth(cdc, AValidTokenForTests)
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
		dc, err = client.CreateDataChannel("bash,-c,sleep 1; echo 123456", nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			log.Printf("Channel %q opened", dc.Label())
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("DC Got msg #%d: %s", count, msg.Data)
			if count == 0 {
				cID = string(msg.Data)
				log.Printf("Client got a channel id: %q", cID)
				dc.Close()
				gotID <- true
			}
			count++
		})
	})
	SignalPair(client, peer.PC)
	<-gotID
	// Now that we have a channel open, let's close the channel and reconnect
	dc2, err := client.CreateDataChannel(">"+cID, nil)
	require.Nil(t, err, "Failed to create the second data channel: %q", err)
	dc2.OnOpen(func() {
		log.Println("Second channel is open")
	})
	count2 := 0
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("DC2 Got msg #%d: %s", count2, msg.Data)
		// first message is the pane id
		if count2 == 0 && string(msg.Data) != cID {
			t.Errorf("Got a bad pane ID on reconnect, expected %q got %q",
				cID, msg.Data)
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
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for dat ain reconnected pane")
	case <-done:
	}
	// dc.Close()
	// dc2.Close()
}
func TestPayloadOperations(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
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
		go SendAuth(cdc, AValidTokenForTests)
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message and the get payload
			log.Printf("Got a ctrl msg: %s", msg.Data)
			args := ParseAck(t, msg)
			var payload []byte
			err = json.Unmarshal(args.Body, &payload)
			if args.Ref == TestAckRef {
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
	SignalPair(client, peer.PC)
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for an ack")
	case <-gotAuthAck:
	}
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
func TestChannelRestore(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
	var cID string
	var dc *webrtc.DataChannel
	done := make(chan bool)
	gotAuthAck := make(chan bool)
	gotOutput := make(chan bool)
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
		go SendAuth(cdc, AValidTokenForTests)
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
		dc, err = client.CreateDataChannel("24x80,bash,-c,echo 123456 ; sleep 1", nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			log.Printf("Channel %q opened", dc.Label())
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("DC Got msg #%d: %s", count, msg.Data)
			if count == 0 {
				cID = string(msg.Data)
				log.Printf("Client got a channel id: %q", cID)
			}
			if count == 1 {
				require.Contains(t, string(msg.Data), "123456")
				gotOutput <- true
			}
			count++
		})
	})
	SignalPair(client, peer.PC)
	<-gotOutput
	// Now that we have a channel open, let's close the channel and reconnect
	dc2, err := client.CreateDataChannel(">"+cID, nil)
	require.Nil(t, err, "Failed to create the second data channel: %q", err)
	dc2.OnOpen(func() {
		log.Println("Second channel is open")
	})
	count2 := 0
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		// first message is the pane id
		if count2 == 0 && string(msg.Data) != cID {
			t.Errorf("Got a bad pane ID on reconnect, expected %q got %q",
				cID, msg.Data)
		}
		// second message should be the echo output
		if count2 == 1 {
			require.Contains(t, string(msg.Data), "123456",
				"Got an unexpected reply: %s", msg.Data)
			done <- true
		}
		count2++
	})
	log.Print("Waiting on done")
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for dat ain reconnected pane")
	case <-done:
	}
}
