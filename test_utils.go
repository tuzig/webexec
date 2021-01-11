// This vile ciontains utilitiles used during the tests
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sys/unix"
)

// AValidTokenForTests token is stored in the file "./test_tokens"
// tests that use it should add `TokensFilePath = "./test_tokens"`
const AValidTokenForTests = "THEoneANDonlyTOKEN"

// TestAckRef is the ref to use in tests
const TestAckRef = 123

var testSD = webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ""}

// used to keep track of sent control messages
var lastRef int

// GetMsgType is used get the type of a control message
func GetMsgType(t *testing.T, msg webrtc.DataChannelMessage) string {
	env := CTRLMessage{}
	err := json.Unmarshal(msg.Data, &env)
	require.Nil(t, err, "failed to unmarshal cdc message: %q", err)
	return env.Type
}

// ParseAck parses and ack message and returns its args
func ParseAck(t *testing.T, msg webrtc.DataChannelMessage) AckArgs {
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

// SendAuth sends an authorization message
func SendAuth(cdc *webrtc.DataChannel, token string, marker int) {
	time.Sleep(10 * time.Millisecond)
	//TODO we need something like peer.LastMsgId++ below
	msg := CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "auth",
		Ref:  TestAckRef,
		Args: AuthArgs{token, marker},
	}
	authMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal the auth args: %v", err)
	} else {
		log.Print("Test is sending an auth message")
		cdc.Send(authMsg)
	}
}

func getMarker(cdc *webrtc.DataChannel) int {
	lastRef++
	ref := lastRef
	// sleep to simulate latency
	time.Sleep(10 * time.Millisecond)
	//TODO we need something like peer.LastMsgId++ below
	msg := CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "mark",
		Ref:  ref,
		Args: nil,
	}
	markMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal the makr message: %v", err)
		return -1
	}
	log.Print("Test is sending a mark message")
	cdc.Send(markMsg)
	return ref
}

// signalPair is used to start a connection between two peers
func SignalPair(pcOffer *webrtc.PeerConnection, peer *Peer) error {
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
	gatherComplete := webrtc.GatheringCompletePromise(pcOffer)
	if err := pcOffer.SetLocalDescription(offer); err != nil {
		return err
	}
	select {
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed mockedMsgs waiting to finish gathering ICE candidates")
	case <-gatherComplete:
		answer, err := peer.Listen(*pcOffer.LocalDescription())
		if err != nil {
			return err
		}
		err = pcOffer.SetRemoteDescription(*answer)
		if err != nil {
			return err
		}
		return nil
	}
}
func isAlive(pid int) bool {
	return unix.Kill(pid, 0) == nil
}

// waitForChild waits for a give timeout for for a process to die
func waitForChild(pid int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) <= timeout {
		if !isAlive(pid) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("process %d still alive (timeout=%v)", pid, timeout)
}
func initTest(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()
	TokensFilePath = "./test_tokens"
	err := LoadConf(defaultConf)
	require.Nil(t, err, "NewPeer failed with: %s", err)
}
