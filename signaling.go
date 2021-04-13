package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var wsWriteM sync.Mutex

func signalingGo() {
	conn := 0
start:
	c, err := dialWS()
	if err != nil {
		time.Sleep(3 * time.Second)
		conn++
		if conn > 3 {
			Logger.Errorf("Failed to dial the signaling server: %q", err)
			return
		}
		goto start
	}
	Logger.Infof("Connected to peerbook")
	conn = 0
	defer c.Close()
	for {
		mType, m, err := c.ReadMessage()
		if err != nil {
			Logger.Errorf("Signaling read error", err)
			c.Close()
			goto start
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
func getFP() string {
	certs, err := GetCerts()
	if err != nil {
		Logger.Error(err)
	}
	// TODO: is 0 the right choice?
	fps, err := certs[0].GetFingerprints()
	if err != nil {
		Logger.Error("Failed to get fingerprints: %w", err)
	}
	return fmt.Sprintf("%s %s", fps[0].Algorithm, fps[0].Value)
}

func dialWS() (*websocket.Conn, error) {
	var cstDialer = websocket.Dialer{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 30 * time.Second,
	}
	fp := getFP()

	params := url.Values{}
	params.Add("fp", fp)
	params.Add("name", Conf.name)
	params.Add("kind", "webexec")
	params.Add("email", Conf.email)
	url := url.URL{Scheme: "wss", Host: Conf.signalingHost, Path: "/ws",
		RawQuery: params.Encode()}
	conn, resp, err := cstDialer.Dial(url.String(), nil)
	if resp.StatusCode == 400 {
		return nil, fmt.Errorf(
			"code a bad request from the server: %s", resp.Status)
	}
	if resp.StatusCode == 401 {
		return nil, &errUnauthorized{}
	}
	return conn, err
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
	fp, found := m["source_fp"].(string)
	if !found {
		return fmt.Errorf("Missing 'source_fp' paramater")
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
				// TODO: get the hub to send all messages through a buffered
				//  channel and a single go func
				wsWriteM.Lock()
				c.WriteMessage(websocket.TextMessage, j)
				wsWriteM.Unlock()
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
		wsWriteM.Lock()
		c.WriteMessage(websocket.TextMessage, j)
		wsWriteM.Unlock()
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
func verifyPeer() (bool, error) {
	fmt.Println("Testing peerbook connectivity & authorization")
	fp := getFP()
	msg := map[string]string{"fp": fp, "email": Conf.email,
		"kind": "webexec", "name": Conf.name}
	m, err := json.Marshal(msg)
	u := fmt.Sprintf("https://%s/verify", Conf.signalingHost)
	resp, err := http.Post(u, "application/json", bytes.NewBuffer(m))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var ret map[string]bool
	err = json.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		return false, err
	}
	return ret["verified"], nil
}
