package main

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

/* it doesn't seem like SignalPair works when we need to test at this level.

func TestWrongFingerprint(t *testing.T) {
	initTest(t)
	failed := make(chan bool)
	// create an unknown client
	client, _, err := NewClient(false)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer, err := NewPeer("BAD CERT")
	require.Nil(t, err, "NewPeer failed with: %s", err)
	err = SignalPair(client, peer)
	dc, err := client.CreateDataChannel("echo,Failed", nil)
	require.Nil(t, err, "failed to create the a channel: %q", err)
	dc.OnMessage(func(_ webrtc.DataChannelMessage) { failed <- true })
	require.Nil(t, err, "failed to signal pair: %q", err)
	select {
	case <-time.After(3 * time.Second):
	case <-failed:
		t.Error("Data channel is opened even though no authentication")
	}
	Shutdown()
}
*/

func TestIsAuthorized(t *testing.T) {
	// create the token file and test good & bad tokens
	initTest(t)
	file, err := ioutil.TempFile("", "authorized_fingerprints")
	TokensFilePath = file.Name()
	require.Nil(t, err, "Failed to create a temp tokens file: %s", err)
	file.WriteString("GOODTOKEN\nANOTHERGOODTOKEN\n")
	file.Close()
	require.True(t, IsAuthorized("GOODTOKEN"))
	require.True(t, IsAuthorized("ANOTHERGOODTOKEN"))
	require.False(t, IsAuthorized("BADTOKEN"))
}
