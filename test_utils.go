// This vile ciontains utilitiles used during the tests
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io/ioutil"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sys/unix"
)

// used to keep track of sent control messages
var lastRef int

// NewClient is used generate a new client return the client, it's fingerprint
// and an error
func NewClient(known bool) (*webrtc.PeerConnection, *webrtc.Certificate, error) {
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

func isAlive(pid int) bool {
	return unix.Kill(pid, 0) == nil
}

func initTest(t *testing.T) {
	if peers.PtyMux == nil {
		peers.PtyMux = peers.PtyMuxType{}
	}
	Logger = zaptest.NewLogger(t).Sugar()
	conf, addr, err := parseConf(defaultConf)
	require.Nil(t, err, "parseConf failed with: %s", err)
	conf.Logger = Logger
	require.NotNil(t, conf)
	require.Equal(t, "0.0.0.0:7777", string(addr))
	Conf.insecure = true
	Conf.iceServers = nil
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	key = &KeyType{Name: f.Name()}
	cert, err := key.generate()
	require.Nil(t, err)
	key.save(cert)
}
func newPeer(t *testing.T, fp string, certificate *webrtc.Certificate) *peers.Peer {
	conf := peers.Conf{
		Certificate:       certificate,
		Logger:            Logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return []webrtc.ICEServer{}, nil
		},
		OnCTRLMsg: handleCTRLMsg,
	}
	peer, err := peers.NewPeer(fp, &conf)
	require.NoError(t, err)
	require.NotNil(t, peer)
	return peer
}
