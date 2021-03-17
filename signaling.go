package main

import (
	"bytes"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"net/url"
	"os"
)

type Candidate struct {
	offer      string
	address    string
	answer     string
	created_on string
}

func signalingGo(done chan os.Signal) {
	var req ConnectRequest
	var offer webrtc.SessionDescription
	u := url.URL{Scheme: "ws", Host: Conf.signalingHost, Path: "/connect"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		Logger.Error(err)
	}
	defer c.Close()
	for {
		_, m, err := c.ReadMessage()
		if err != nil {
			Logger.Errorf("Signaling read error", err)
			continue
		}
		Logger.Info("Received message", m)
		r := bytes.NewReader(m)
		err = parsePeerReq(r, &req, &offer)
		if err != nil {
			Logger.Error(err)
			continue
		}
		peer, err := NewPeer(req.Fingerprint)
		if err != nil {
			Logger.Errorf("Failed to create a new peer: %w", err)
			continue
		}
		answer, err := peer.Listen(offer)
		if err != nil {
			Logger.Errorf("Peer failed to listen : %w", err)
			continue
		}
		// reply with server's key
		payload := make([]byte, 4096)
		l, err := EncodeOffer(payload, answer)
		if err != nil {
			Logger.Errorf("Failed to encode offer : %w", err)
			continue
		}
		c.WriteMessage(websocket.TextMessage, payload[:l])
	}
}
