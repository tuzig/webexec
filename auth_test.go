package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/stretchr/testify/require"
)

func TestUnauthincatedBlocked(t *testing.T) {
	/* MT: Add a test logger that won't spam stdout but will log
	to t.Logger. Logs will only with -v or when there's an
	error */
	InitDevLogger()
	done := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection: %q", err)
	SignalPair(client, peer.PC)
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

	// MT: Why do we do this code?
	time.AfterFunc(3*time.Second, func() {
		done <- true
	})
	<-done
	Shutdown()
}

func TestAuthorization(t *testing.T) {
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
		go SendAuth(cdc, AValidTokenForTests)
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ackArgs := ParseAck(t, msg)
			require.Equal(t, ackArgs.Ref, TEST_ACK_REF,
				"Expeted ack ref to equal %d and got: ", TEST_ACK_REF, ackArgs.Ref)
			gotAuthAck <- true
		})

	})
	SignalPair(client, peer.PC)
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
		go SendAuth(cdc, AValidTokenForTests)
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
	SignalPair(client, peer2.PC)
	<-gotTokenAck
}

func TestBadToken(t *testing.T) {
	InitDevLogger()
	gotNAck := make(chan bool)
	peer, err := NewPeer("")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err, "Failed to start a new peer connection %q", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %q", err)

	cdc.OnOpen(func() {
		go SendAuth(cdc, "BADWOLF")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			msgType := GetMsgType(t, msg)
			require.Equal(t, msgType, "nack",
				"Expected a nack and got a %q", msgType)
			gotNAck <- true
		})
	})
	SignalPair(client, peer.PC)
	<-gotNAck
}
func TestIsAuthorized(t *testing.T) {
	// create the token file and test good & bad tokens
	file, err := ioutil.TempFile("", "authorized_tokens")
	TokensFilePath = file.Name()
	require.Nil(t, err, "Failed to create a temp tokens file: %s", err)
	file.WriteString("GOODTOKEN\nANOTHERGOODTOKEN\n")
	file.Close()
	require.True(t, IsAuthorized("GOODTOKEN"))
	require.True(t, IsAuthorized("ANOTHERGOODTOKEN"))
	require.False(t, IsAuthorized("BADTOKEN"))
}
