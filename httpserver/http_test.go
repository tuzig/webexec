package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

var serverHost string

func TestConnect(t *testing.T) {
	initTest(t)
	done := make(chan bool)
	// start the webrtc client
	client, fp, err := NewClient(true)
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
		l, err := EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		p := ConnectRequest{fp, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		w := httptest.NewRecorder()
		a := NewMockAuthBackend(fp)
		h := NewConnectHandler(a)
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		// read server offer
		err = DecodeOffer(&sd, w.Body.Bytes())
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
	/*
			// There's t.Cleanup in go 1.15+
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := Shutdown(ctx)
			require.Nil(t, err, "Failed shutting the http server: %v", err)
		Shutdown()
		// TODO: be smarter, this is just a hack to get github action to pass
		time.Sleep(500 * time.Millisecond)
	*/
}
func TestConnectBadFP(t *testing.T) {
	initTest(t)
	client, cert, err := NewClient(true)
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
		l, err := EncodeOffer(buf, *client.LocalDescription())
		require.Nil(t, err, "Failed ending an offer: %v", clientOffer)
		p := ConnectRequest{cert, 1, string(buf[:l])}
		b, err := json.Marshal(p)
		require.Nil(t, err, "Failed to marshal the connect request: %s", err)

		req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewBuffer(b))
		req.RemoteAddr = "8.8.8.8"
		w := httptest.NewRecorder()
		a := NewMockAuthBackend("")
		h := NewConnectHandler(a)
		h.HandleConnect(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestEncodeDecodeStringArray(t *testing.T) {
	initTest(t)
	a := []string{"Hello", "World"}
	b := make([]byte, 4096)
	l, err := EncodeOffer(b, a)
	require.Nil(t, err, "Failed to encode offer: %s", err)
	Logger.Infof("encode offer to: %q", b[:l])

	c := make([]string, 2)
	err = DecodeOffer(&c, b[:l])
	require.Nil(t, err, "Failed to decode offer: %s", err)
	require.Equal(t, a, c)
}
