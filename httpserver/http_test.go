package httpserver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/zap/zaptest"
)

var serverHost string

// MockAuthBackend is used to mock the auth backend
type MockAuthBackend struct {
	authorized string
}

func (a *MockAuthBackend) IsAuthorized(tokens ...string) bool {
	fmt.Println("authorized", a.authorized)
	if a.authorized == "" {
		return false
	}
	for _, t := range tokens {
		fmt.Println("token", t)
		if t == a.authorized {
			return true
		}
	}
	return false
}

func generateCert() (*webrtc.Certificate, error) {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate key: %w", err)
	}
	origin := make([]byte, 16)
	/* #nosec */
	if _, err := rand.Read(origin); err != nil {
		return nil, err
	}

	// Max random value, a 130-bits integer, i.e 2^130 - 1
	maxBigInt := new(big.Int)
	/* #nosec */
	maxBigInt.Exp(big.NewInt(2), big.NewInt(130), nil).Sub(maxBigInt, big.NewInt(1))
	/* #nosec */
	serialNumber, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return nil, err
	}

	return webrtc.NewCertificate(secretKey, x509.Certificate{
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		NotBefore:             time.Now(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SerialNumber:          serialNumber,
		Version:               2,
		Subject:               pkix.Name{CommonName: hex.EncodeToString(origin)},
		IsCA:                  true,
	})
}
func newClient(t *testing.T) (*webrtc.PeerConnection, *webrtc.Certificate, error) {
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
func newCert(t *testing.T) *webrtc.Certificate {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "Failed to generate a secret key: %q", err)
	certificate, err := webrtc.GenerateCertificate(secretKey)
	require.NoError(t, err, "Failed to generate a certificate: %q", err)
	return certificate
}

func TestConnectGoodFP(t *testing.T) {
	done := make(chan bool)
	// start the webrtc client
	client, cert, err := newClient(t)
	require.NoError(t, err, "Failed to create a client: %q", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	clientOffer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	gatherComplete := webrtc.GatheringCompletePromise(client)
	err = client.SetLocalDescription(clientOffer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("timed out waiting to ice gathering to complete")
	case <-gatherComplete:
		var sd webrtc.SessionDescription
		buf := make([]byte, 4096)
		l, err := peers.EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		fp, err := peers.ExtractFP(cert)
		require.NoError(t, err, "Failed to extract the fingerprint: %q", err)
		fmt.Println("fp", fp)
		a := &MockAuthBackend{authorized: fp}
		p := ConnectRequest{fp, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		w := httptest.NewRecorder()
		logger := zaptest.NewLogger(t).Sugar()
		certificate, err := generateCert()
		require.NoError(t, err, "Failed to create a certificate: %q", err)
		conf := &peers.Conf{
			Certificate:       certificate,
			Logger:            logger,
			DisconnectTimeout: time.Second,
			FailedTimeout:     time.Second,
			KeepAliveInterval: time.Second,
			GatheringTimeout:  time.Second,
			GetICEServers: func() ([]webrtc.ICEServer, error) {
				return nil, nil
			},
		}
		h := &ConnectHandler{
			authBackend: a,
			peerConf:    conf,
			logger:      logger,
		}
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		// read server offer
		err = peers.DecodeOffer(&sd, w.Body.Bytes())
		require.Nil(t, err, "Failed decoding an offer: %v", clientOffer)
		client.SetRemoteDescription(sd)
		// count the incoming messages
		cdc.OnOpen(func() {
			done <- true
		})
	}
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("Timeouton cdc open")
	case <-done:
	}
}
func TestConnectBadFP(t *testing.T) {
	client, cert, err := newClient(t)
	_, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	clientOffer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	gatherComplete := webrtc.GatheringCompletePromise(client)
	err = client.SetLocalDescription(clientOffer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("timed out waiting to ice gathering to complete")
	case <-gatherComplete:
		buf := make([]byte, 4096)
		l, err := peers.EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		fp, err := peers.ExtractFP(cert)
		require.NoError(t, err, "Failed to extract the fingerprint: %q", err)
		p := ConnectRequest{fp, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		require.Nil(t, err, "Failed to marshal the connect request: %s", err)

		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		w := httptest.NewRecorder()
		a := &MockAuthBackend{authorized: ""}
		logger := zaptest.NewLogger(t).Sugar()
		conf := &peers.Conf{
			// Certificate:       certificate,
			Logger:            logger,
			DisconnectTimeout: time.Second,
			FailedTimeout:     time.Second,
			KeepAliveInterval: time.Second,
			GatheringTimeout:  time.Second,
			GetICEServers: func() ([]webrtc.ICEServer, error) {
				return nil, nil
			},
		}
		h := NewConnectHandler(a, conf, logger)
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}
func TestConnectWithBearer(t *testing.T) {
	client, cert, err := newClient(t)
	_, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	clientOffer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	gatherComplete := webrtc.GatheringCompletePromise(client)
	err = client.SetLocalDescription(clientOffer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("timed out waiting to ice gathering to complete")
	case <-gatherComplete:
		buf := make([]byte, 4096)
		l, err := peers.EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		fp, err := peers.ExtractFP(cert)
		require.NoError(t, err, "Failed to extract the fingerprint: %q", err)
		p := ConnectRequest{fp, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		require.Nil(t, err, "Failed to marshal the connect request: %s", err)

		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		req.Header.Set("content-type", "application/json")
		req.Header.Set("Authorization", "Bearer LetMePass")
		w := httptest.NewRecorder()
		a := &MockAuthBackend{authorized: "LetMePass"}
		logger := zaptest.NewLogger(t).Sugar()
		certificate, err := generateCert()
		require.NoError(t, err, "Failed to generate a certificate", err)
		conf := &peers.Conf{
			Logger:            logger,
			DisconnectTimeout: time.Second,
			FailedTimeout:     time.Second,
			KeepAliveInterval: time.Second,
			GatheringTimeout:  time.Second,
			Certificate:       certificate,
			GetICEServers: func() ([]webrtc.ICEServer, error) {
				return nil, nil
			},
		}
		h := NewConnectHandler(a, conf, logger)
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}
}
func TestConnectWithBadBearer(t *testing.T) {
	client, cert, err := newClient(t)
	_, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	clientOffer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	gatherComplete := webrtc.GatheringCompletePromise(client)
	err = client.SetLocalDescription(clientOffer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("timed out waiting to ice gathering to complete")
	case <-gatherComplete:
		buf := make([]byte, 4096)
		l, err := peers.EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		fp, err := peers.ExtractFP(cert)
		require.NoError(t, err, "Failed to extract the fingerprint: %q", err)
		p := ConnectRequest{fp, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		require.Nil(t, err, "Failed to marshal the connect request: %s", err)

		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		req.Header.Set("content-type", "application/json")
		req.Header.Set("Authorization", "Bearer LetMePass")
		w := httptest.NewRecorder()
		a := &MockAuthBackend{authorized: ""}
		logger := zaptest.NewLogger(t).Sugar()
		certificate, err := generateCert()
		require.NoError(t, err, "Failed to generate a certificate", err)
		conf := &peers.Conf{
			// Certificate:       certificate,
			Logger:            logger,
			DisconnectTimeout: time.Second,
			FailedTimeout:     time.Second,
			KeepAliveInterval: time.Second,
			GatheringTimeout:  time.Second,
			Certificate:       certificate,
			GetICEServers: func() ([]webrtc.ICEServer, error) {
				return nil, nil
			},
		}
		h := NewConnectHandler(a, conf, logger)
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestEncodeDecodeStringArray(t *testing.T) {
	a := []string{"Hello", "World"}
	b := make([]byte, 4096)
	l, err := peers.EncodeOffer(b, a)
	require.Nil(t, err, "Failed to encode offer: %s", err)
	c := make([]string, 2)
	err = peers.DecodeOffer(&c, b[:l])
	require.Nil(t, err, "Failed to decode offer: %s", err)
	require.Equal(t, a, c)
}
func FuncTestOffer(t *testing.T) {
	done := make(chan bool)
	// start the webrtc client
	client, cert, err := newClient(t)
	require.NoError(t, err, "Failed to create a client: %q", err)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	clientOffer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	err = client.SetLocalDescription(clientOffer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)

	var sd webrtc.SessionDescription
	fp, err := peers.ExtractFP(cert)
	require.NoError(t, err, "Failed to extract the fingerprint: %q", err)
	a := &MockAuthBackend{authorized: fp}
	b, err := json.Marshal(clientOffer)
	require.NoError(t, err, "Failed to marshal the client offer: %s", err)
	req := httptest.NewRequest(http.MethodPost, "/offer", bytes.NewBuffer(b))
	req.RemoteAddr = "8.8.8.8"
	w := httptest.NewRecorder()
	logger := zaptest.NewLogger(t).Sugar()
	certificate, err := generateCert()
	require.NoError(t, err, "Failed to create a certificate: %q", err)
	conf := &peers.Conf{
		Certificate:       certificate,
		Logger:            logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return nil, nil
		},
	}
	h := &ConnectHandler{
		authBackend: a,
		peerConf:    conf,
		logger:      logger,
		address:     AddressType("127.0.0.1:7777"),
	}
	h.HandleOffer(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	// get the Location header
	cansURL := w.Header().Get("Location")
	require.NotEmpty(t, cansURL, "Location header is empty")
	require.Contains(t, "/candidates/", cansURL, "Location header is not a connect url")
	client.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		can := c.ToJSON()
		client.AddICECandidate(can)
		// send the candidate to cansURL using a PATCH request
		b, err := json.Marshal(can)
		require.Nil(t, err, "Failed to marshal the candidate: %s", err)
		req := httptest.NewRequest(http.MethodPatch, cansURL, bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		w := httptest.NewRecorder()
		h.HandleCandidate(w, req)
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	// read server offer
	dec := json.NewDecoder(w.Body)
	err = dec.Decode(&sd)
	require.Nil(t, err, "Failed decoding an offer: %v", clientOffer)
	client.SetRemoteDescription(sd)
	// count the incoming messages
	cdc.OnOpen(func() {
		done <- true
	})
	select {
	case <-time.After(3 * time.Second):
		t.Errorf("Timeouton cdc open")
	case <-done:
	}
}
