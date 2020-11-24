// This vile ciontains utilitiles used during the tests
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// AValidTokenForTests token is stored in the file "./test_tokens"
// tests that use it should add `TokensFilePath = "./test_tokens"`
const AValidTokenForTests = "THEoneANDonlyTOKEN"

// TestAckRef is the ref to use in tests
const TestAckRef = 123

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
func SendAuth(cdc *webrtc.DataChannel, token string) {
	time.Sleep(10 * time.Millisecond)
	//TODO we need something like peer.LastMsgId++ below
	msg := CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "auth",
		Ref:  TestAckRef,
		Args: AuthArgs{token},
	}
	authMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal the auth args: %v", err)
	} else {
		log.Print("Test is sending an auth message")
		cdc.Send(authMsg)
	}
}

// SignalPair is used to start a connection between two peers
func SignalPair(pcOffer *webrtc.PeerConnection, pcAnswer *webrtc.PeerConnection) error {
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
