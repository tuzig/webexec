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

func TestActivePeer(t *testing.T) {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certs, err := webrtc.GenerateCertificate(secretKey)
	require.NoError(t, err)
	logger := zaptest.NewLogger(t).Sugar()
	conf := Conf{
		Certificate:       certs,
		Logger:            logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return []webrtc.ICEServer{}, nil
		},
		OnCTRLMsg: func(*Peer, *CTRLMessage, json.RawMessage) {
			return
		},
	}
	peer, err := NewPeer("fingerprint", &conf)
	require.NoError(t, err)
	require.NotNil(t, peer)
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
