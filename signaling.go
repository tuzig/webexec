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
dial:
	c, _, err := cstDialer.Dial(u.String(), nil)
	if err != nil {
		Logger.Warnf("Failed to dial the peerbook server %q: %w", u, err)
		return
	}
	Logger.Infof("Connected to peerbook")
	defer c.Close()
	for {
		mType, m, err := c.ReadMessage()
		if err != nil {
			Logger.Errorf("Signaling read error", err)
			c.Close()
			goto dial
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
	fp, found := m["source_fp"].(string)
	if !found {
		return fmt.Errorf("Missing 'source_fp' paramater")
	}
	code, found := m["code"]
	if found {
		Logger.Infof("Got a status message: %v", code)
		return nil
	}
	o, found := m["offer"].(string)
	if found {
		var offer webrtc.SessionDescription
		err = DecodeOffer(&offer, []byte(o))
		offerFP, err := GetFingerprint(&offer)
		if err != nil {
			return fmt.Errorf("Failed to get fingerprint from offer: %w", err)
		}
		if offerFP != fp {
			Peers[fp].PC.Close()
			Peers[fp].PC = nil
			return fmt.Errorf("Mismatching fingerprints: %q != %q", offerFP, fp)
		} else {
			Logger.Info("Authenticated!")
		}
		peer, err := NewPeer(fp)
		if err != nil {
			return fmt.Errorf("Failed to create a new peer: %w", err)
		}
		peer.PC.OnICECandidate(func(can *webrtc.ICECandidate) {
			if can != nil {
				m := map[string]interface{}{
					"target": fp, "candidate": can.ToJSON()}
				Logger.Infof("Sending candidate: %v", m)
				j, err := json.Marshal(m)
				if err != nil {
					Logger.Errorf("Failed to encode offer : %w", err)
					return
				}
				c.WriteMessage(websocket.TextMessage, j)
			}
		})
		err = peer.PC.SetRemoteDescription(offer)
		if err != nil {
			return fmt.Errorf("Peer failed to listen : %w", err)
		}
		answer, err := peer.PC.CreateAnswer(nil)
		if err != nil {
			return fmt.Errorf("Failed to create an answer: %w", err)
		}
		err = peer.PC.SetLocalDescription(answer)
		payload := make([]byte, 4096)
		l, err := EncodeOffer(payload, answer)
		if err != nil {
			return fmt.Errorf("Failed to encode offer : %w", err)
		}
		m := map[string]string{"answer": string(payload[:l]), "target": fp}
		Logger.Infof("Sending answer: %v", m)
		j, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("Failed to encode offer : %w", err)
		}
		c.WriteMessage(websocket.TextMessage, j)
		return nil
	}
	_, found = m["candidate"]
	if found {
		var can struct {
			sourceFP   string                  `json:"source_fp"`
			sourceName string                  `json:"source_name"`
			Candidate  webrtc.ICECandidateInit `json:"candidate"`
		}
		r.Seek(0, 0)
		err = dec.Decode(&can)
		peer, found := Peers[fp]
		if found {
			Logger.Infof("Adding an ICE Candidate: %v", can.Candidate)
			err := peer.PC.AddICECandidate(can.Candidate)
			if err != nil {
				return fmt.Errorf("Failed to set remote description: %w", err)
			}
		} else {
			return fmt.Errorf("got a candidate from an unknown peer: %s", fp)
		}
	}
	return nil
}
