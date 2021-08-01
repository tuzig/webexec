package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var wsWriteM sync.Mutex

func signalingGo() {
	c, err := dialWS()
	if err != nil {
		Logger.Errorf("Failed to dial the signaling server: %q", err)
		goto retry
	}
	Logger.Infof("Connected to peerbook")
	defer c.Close()
	for {
		mType, m, err := c.ReadMessage()
		if err != nil {
			Logger.Errorf("Signaling read error", err)
			break
		}
		if mType == websocket.TextMessage {
			Logger.Info("Received text message", string(m))
			err = handleMessage(c, m)
			if err != nil {
				Logger.Errorf("Failed to handle message: %w", err)
			}
		}
	}
retry:
	time.AfterFunc(Conf.peerbookTimeout, signalingGo)
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
	s := strings.Replace(fps[0].Value, ":", "", -1)
	return strings.ToUpper(s)
}

func dialWS() (*websocket.Conn, error) {
	var cstDialer = websocket.Dialer{
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		HandshakeTimeout: 3 * time.Second,
	}
	fp := getFP()

	params := url.Values{}
	params.Add("fp", fp)
	params.Add("name", Conf.name)
	params.Add("kind", "webexec")
	params.Add("email", Conf.email)

	schema := "wss"
	if Conf.insecure {
		schema = "ws"
	}
	url := url.URL{Scheme: schema, Host: Conf.peerbookHost, Path: "/ws",
		RawQuery: params.Encode()}
	conn, resp, err := cstDialer.Dial(url.String(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf(string(bodyBytes))
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
	_, found = m["peers"]
	if found {
		return nil
	}
	fp, found := m["source_fp"].(string)
	if !found {
		return fmt.Errorf("Missing 'source_fp' paramater")
	}
	v, found := m["peer_update"]
	if found {
		pu := v.(map[string]interface{})
		peer, found := Peers[fp]
		if found && peer.PC != nil && !pu["verified"].(bool) {
			peer.PC.Close()
			peer.PC = nil
		}
	}
	o, found := m["offer"].(string)
	if found {
		var offer webrtc.SessionDescription
		err = DecodeOffer(&offer, []byte(o))
		if err != nil {
			return fmt.Errorf("Failed to get fingerprint from offer: %w", err)
		}

		offerFP, err := GetFingerprint(&offer)
		if err != nil {
			return fmt.Errorf("Failed to get offer's fingerprint: %w", err)
		}
		if offerFP != fp {
			Peers[fp].PC.Close()
			Peers[fp].PC = nil
			Logger.Warnf("Refusing connection because fp mismatch: %s", fp)
			return fmt.Errorf("Mismatched fingerprint: %s", fp)
		}
		Logger.Info("Authenticated!")
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
		err = c.WriteMessage(websocket.TextMessage, j)
		if err != nil {
			return fmt.Errorf("Failed to write answer: %w", err)
		}
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
			if peer.PC == nil {
				Logger.Infof("ICE Candidate pending: %v", can.Candidate)
				peer.pendingCandidates <- &can.Candidate
				return nil
			}
			Logger.Infof("Adding an ICE Candidate: %v", can.Candidate)
			err := peer.PC.AddICECandidate(can.Candidate)
			if err != nil {
				return fmt.Errorf("Failed to add ice candidate: %w", err)
			}
		} else {
			return fmt.Errorf("got a candidate from an unknown peer: %s", fp)
		}
	}
	return nil
}
