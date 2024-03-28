package peers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func newClient(known bool) (*webrtc.PeerConnection, *webrtc.Certificate, error) {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	certificate, err := webrtc.GenerateCertificate(secretKey)
	certs := []webrtc.Certificate{*certificate}
	client, err := webrtc.NewPeerConnection(
		webrtc.Configuration{Certificates: certs})
	if err != nil {
		return nil, nil, err
	}
	return client, certificate, err
}
func newPeer(t *testing.T, fp string, certificate *webrtc.Certificate) *Peer {
	logger := zaptest.NewLogger(t).Sugar()
	conf := Conf{
		Certificate:       certificate,
		Logger:            logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return []webrtc.ICEServer{}, nil
		},
		OnCTRLMsg: func(*Peer, CTRLMessage, json.RawMessage) {
			return
		},
	}
	peer, err := NewPeer(fp, &conf)
	require.NoError(t, err)
	require.NotNil(t, peer)
	return peer
}
func TestActivePeer(t *testing.T) {
	client, certs, err := newClient(true)
	require.NoError(t, err, "failed to create a new client %v", err)
	defer client.Close()
	peer := newPeer(t, "a", certs)
	require.NotNil(t, peer)
	activePeer := GetActivePeer()
	require.Nil(t, activePeer)
	SetLastPeer(peer)
	activePeer = GetActivePeer()
	require.Equal(t, peer, activePeer)
	// go back half a keep alive
	lastReceived = time.Now().Add(time.Second / 2.0)
	activePeer = GetActivePeer()
	require.Equal(t, peer, activePeer)
	// go back a long while
	lastReceived = time.Now().Add(-5 * time.Second)
	activePeer = GetActivePeer()
	require.Nil(t, activePeer)
}
