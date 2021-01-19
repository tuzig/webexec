package main

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestWrongFingerprint(t *testing.T) {
	initTest(t)
	failed := make(chan bool)
	closed := make(chan bool)
	// create an unknown client
	client, _, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer, err := NewPeer("BAD CERT")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	client.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		Logger.Infof("Client's Connection State change: %s", state.String())
		if state == webrtc.PeerConnectionStateFailed {
			closed <- true
		}
	})

	dc, err := client.CreateDataChannel("echo,Failed", nil)
	require.Nil(t, err, "failed to create the a channel: %q", err)
	dc.OnOpen(func() { failed <- true })
	err = SignalPair(client, peer)
	require.Nil(t, err, "failed to signal pair: %q", err)
	select {
	case <-time.After(3 * time.Second):
	case <-closed:
	case <-failed:
		t.Error("Data channel is opened even though no authentication")
	}
	Shutdown()
}

func TestIsAuthorized(t *testing.T) {
	// create the token file and test good & bad tokens
	initTest(t)
	file, err := ioutil.TempFile("", "authorized_tokens")
	TokensFilePath = file.Name()
	require.Nil(t, err, "Failed to create a temp tokens file: %s", err)
	file.WriteString("GOODTOKEN\nANOTHERGOODTOKEN\n")
	file.Close()
	require.True(t, IsAuthorized("GOODTOKEN"))
	require.True(t, IsAuthorized("ANOTHERGOODTOKEN"))
	require.False(t, IsAuthorized("BADTOKEN"))
}
