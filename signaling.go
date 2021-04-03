package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type Candidate struct {
	offer      string
	address    string
	answer     string
	created_on string
}

func signalingGo() {
	var cstDialer = websocket.Dialer{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 30 * time.Second,
	}
	certs, err := key.GetCerts()
	if err != nil {
		Logger.Error(err)
	}
	params := url.Values{}
	// TODO: is 0 the right choice?
	fps, err := certs[0].GetFingerprints()
	if err != nil {
		Logger.Error("Failed to get fingerprints: %w", err)
	}
	fp := fmt.Sprintf("%s %s", fps[0].Algorithm, fps[0].Value)

	hostname, err := os.Hostname()
	if err != nil {
		Logger.Warnf("Failed to get hostname, using 'unknown'")
		hostname = "unknown"
	}
	params.Add("fp", fp)
	params.Add("name", hostname)
	params.Add("kind", "webexec")
	params.Add("email", Conf.email)
	u := url.URL{Scheme: "ws", Host: Conf.signalingHost, Path: "/ws",
		RawQuery: params.Encode()}
	c, _, err := cstDialer.Dial(u.String(), nil)
	if err != nil {
		Logger.Warnf("Failed to dial the signaling server %q: %w", u, err)
		return
	}
	defer c.Close()
	for {
		mType, m, err := c.ReadMessage()
		if err != nil {
			Logger.Errorf("Signaling read error", err)
			return
		}
		if mType == websocket.TextMessage {
			Logger.Info("Received text message", string(m))
			err = handleMessage(c, m)
			if err != nil {
				Logger.Errorf("Failed to handle message: %w", err)
			}
		}
	}
}
func handleMessage(c *websocket.Conn, message []byte) error {
	var m map[string]interface{}
	r := bytes.NewReader(message)
	dec := json.NewDecoder(r)
	err := dec.Decode(&m)
	if err != nil {
		return fmt.Errorf("Failed to decode message: %w", err)
	}
	code, found := m["code"]
	if found {
		Logger.Infof("Got a status message: %v", code)
		return nil
	}
	var offer webrtc.SessionDescription
	o, found := m["offer"]
	if found {
		err = DecodeOffer(offer, []byte(o.(string)))
		if err != nil {
			return fmt.Errorf("Failed to decode client's offer: %w", err)
		}
		fp, found := m["fp"]
		if !found {
			return fmt.Errorf("Missing 'fp' paramater")
		}
		peer, err := NewPeer(fp.(string))
		if err != nil {
			return fmt.Errorf("Failed to create a new peer: %w", err)
		}
		answer, err := peer.Listen(offer)
		if err != nil {
			return fmt.Errorf("Peer failed to listen : %w", err)
		}
		// reply with server's key
		payload := make([]byte, 4096)
		l, err := EncodeOffer(payload, answer)
		if err != nil {
			return fmt.Errorf("Failed to encode offer : %w", err)
		}
		c.WriteMessage(websocket.TextMessage, payload[:l])
	}
	return nil
}
