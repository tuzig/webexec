// This vile ciontains utilitiles used during the tests
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sys/unix"
)

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
		Logger.Errorf("Failed to marshal the makr message: %v", err)
		return -1
	}
	cdc.Send(markMsg)
	return ref
}

// NewClient is used generate a new client return the client, it's fingerprint
// and an error
func NewClient(known bool) (*webrtc.PeerConnection, string, error) {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	certificate, err := webrtc.GenerateCertificate(secretKey)
	if err != nil {
		return nil, "", err
	}
	fp, err := certificate.GetFingerprints()
	if known {
		f, err := os.OpenFile(
			TokensFilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		line := fmt.Sprintf(
			"%s %s\r\n", fp[0].Algorithm, strings.ToUpper(fp[0].Value))
		_, err = f.WriteString(line)
		if err != nil {
			return nil, "", err
		}
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{
		Certificates: []webrtc.Certificate{*certificate}})
	r := fmt.Sprintf("%s %s", fp[0].Algorithm, strings.ToUpper(fp[0].Value))
	return client, r, nil
}

// SignalPair is used to start a connection between two peers
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
	if ptyMux == nil {
		ptyMux = ptyMuxType{}
	}
	Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())).Sugar()
	f, err := ioutil.TempFile("", "authorized_fingerprints")
	f.Close()
	TokensFilePath = f.Name()
	require.Nil(t, err, "Failed setting a temp tokens file: %s", err)
	err = parseConf(defaultConf)
	require.Nil(t, err, "NewPeer failed with: %s", err)
	Conf.insecure = true
	Conf.iceServers = nil
	f, err = ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	key = &KeyType{Name: f.Name()}
	cert, err := key.generate()
	require.Nil(t, err)
	key.save(cert)
}

// GetFreePort asks the kernel for a free open port that is ready to use.
// copied from https://github.com/phayes/freeport
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// SendRestore sends an restore message
func SendRestore(cdc *webrtc.DataChannel, ref int, marker int) error {
	time.Sleep(10 * time.Millisecond)
	msg := CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "restore",
		Ref:  ref,
		Args: RestoreArgs{marker},
	}
	restoreMsg, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("Failed to marshal the auth args: %v", err)
	}
	cdc.Send(restoreMsg)
	return nil
}

// TestMain runs before every tesm
func TestMain(m *testing.M) {
	code := m.Run()
	// If we've used a temporary file, remove it
	if TokensFilePath != ConfPath("authorized_fingerprints") {
		os.Remove(TokensFilePath)
	}
	os.Exit(code)
}
