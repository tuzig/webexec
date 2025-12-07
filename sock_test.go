package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/fx/fxtest"
)

func TestOfferGetCandidate(t *testing.T) {
	var id string
	initTest(t)
	lifecycle := fxtest.NewLifecycle(t)
	k := KeyType{}
	certificate, err := k.generate()
	require.NoError(t, err, "Failed to generate a certificate", err)
	conf := peers.Conf{
		Certificate:       certificate,
		Logger:            Logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return nil, nil
		},
	}
	sockServer := NewSockServer(&conf)
	require.NotNil(t, sockServer, "Failed to create a new server")
	startParams := SocketStartParams{t.TempDir()}
	server, err := StartSocketServer(lifecycle, sockServer, startParams)
	require.NoError(t, err, "Failed to start a new server")
	require.NotNil(t, server, "Failed to start a new server")
	lifecycle.RequireStart()
	fp := GetSockFP()
	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", fp)
			},
		},
	}
	// start the webrtc
	client, _, err := NewClient(true)
	require.Nil(t, err, "Failed to start a new client", err)
	var candidatesMux sync.Mutex
	pendingCandidates := make([]*webrtc.ICECandidate, 0)
	client.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidatesMux.Lock()
		pendingCandidates = append(pendingCandidates, c)
	})
	_, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	offer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	err = client.SetLocalDescription(offer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	buf, err := json.Marshal(offer)
	require.Nil(t, err, "Failed decoding an offer: %v", offer)
	Logger.Info("Sendiong post")
	resp, err := httpc.Post("http://unix/offer/", "application/json", bytes.NewBuffer(buf))
	require.NoError(t, err, "Failed sending a post request: %q", err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// read server answer
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	id = body["id"]
	require.NotEmpty(t, id, "Servers response didn't include 'id': %v", body)
	require.NotNil(t, body["answer"], "Servers response didn't include 'answer': %v", body)
	err = resp.Body.Close()
	require.Nil(t, err, "Failed to parse offer+id url: %q", err)
	for {
		r, err := httpc.Get("http://unix/offer/" + id)
		require.Nil(t, err, "Failed to get candidate: %q", err)
		msg, _ := ioutil.ReadAll(r.Body)
		err = r.Body.Close()
		require.Nil(t, err, "Failed to close offer+body: %q", err)
		Logger.Infof("Got candidate: %q", string(msg))
		if r.StatusCode == http.StatusServiceUnavailable {
			break
		}
		require.Equal(t, http.StatusOK, r.StatusCode, string(msg))
	}
	lifecycle.RequireStop()
}
func TestOfferPutCandidates(t *testing.T) {
	var id string
	initTest(t)
	lifecycle := fxtest.NewLifecycle(t)
	k := KeyType{}
	certificate, err := k.generate()
	require.NoError(t, err, "Failed to generate a certificate", err)
	conf := peers.Conf{
		Certificate:       certificate,
		Logger:            Logger,
		DisconnectTimeout: time.Second,
		FailedTimeout:     time.Second,
		KeepAliveInterval: time.Second,
		GatheringTimeout:  time.Second,
		GetICEServers: func() ([]webrtc.ICEServer, error) {
			return nil, nil
		},
	}
	sockServer := NewSockServer(&conf)
	startParams := SocketStartParams{t.TempDir()}
	server, err := StartSocketServer(lifecycle, sockServer, startParams)
	lifecycle.RequireStart()
	require.NoError(t, err, "Failed to start a new server")
	require.NotNil(t, server, "Failed to start a new server")
	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", startParams.fp)
			},
		},
	}
	// start the webrtc
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	require.Nil(t, err, "Failed to start a new client", err)
	defer client.Close()
	pendingCandidates := make(chan *webrtc.ICECandidate, 5)
	client.OnICECandidate(func(c *webrtc.ICECandidate) {
		Logger.Info("Got can")
		if c == nil {
			return
		}
		pendingCandidates <- c
	})
	_, err = client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %q", err)
	offer, err := client.CreateOffer(nil)
	require.Nil(t, err, "Failed to create client offer: %q", err)
	err = client.SetLocalDescription(offer)
	require.Nil(t, err, "Failed to set client's local Description client offer: %q", err)
	buf, err := json.Marshal(offer)
	require.Nil(t, err, "Failed decoding an offer: %v", offer)
	Logger.Info("Sendiong post")
	resp, err := httpc.Post("http://unix/offer/", "application/json", bytes.NewBuffer(buf))
	require.NoError(t, err, "Failed sending a post request: %q", err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Nil(t, err, "Failed to close post request body: %q", err)
	// read server answer
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	id = body["id"]
	require.NotEmpty(t, id, "Servers response didn't include 'id': %v", body)
	require.NotNil(t, body["answer"], "Servers response didn't include 'answer': %v", body)
	err = resp.Body.Close()
	require.Nil(t, err, "Failed to parse offer+id url: %q", err)
	a := sockServer.currentOffers[id]
	// Close the server-side peer connection to prevent logging after test completion
	defer func() {
		if a != nil && a.p != nil && a.p.PC != nil {
			a.p.PC.Close()
		}
	}()
	var i int
	for i = 0; i < 3; i++ {
		select {
		case c := <-pendingCandidates:
			payload := []byte(c.ToJSON().Candidate)
			require.Nil(t, err, "Failed to marshal candidate: %q", err)
			req, err := http.NewRequest("PUT", "http://unix/offer/"+id, bytes.NewReader(payload))
			require.Nil(t, err, "Failed to create new PUT request: %q", err)
			req.Header.Set("Content-Type", "application/json")
			r, err := httpc.Do(req)
			require.Nil(t, err, "Failed to put candidate: %q", err)
			msg, _ := ioutil.ReadAll(r.Body)
			require.Equal(t, http.StatusOK, r.StatusCode, string(msg))
			err = r.Body.Close()
			require.Nil(t, err, "Failed to close put body: %q", err)
		case <-time.After(time.Second):
			if a.p.PC.ICEGatheringState() != webrtc.ICEGatheringStateGathering {
				break
			}
		}
	}
	require.Greater(t, i, 0)
	// For incoming handle to finish
	lifecycle.RequireStop()
}
