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
	conn := 0
start:
	c, err := dialWS()
	if err != nil {
		conn++
		if conn > 3 {
			Logger.Errorf("Failed to dial the signaling server: %q", err)
			return
		}
		time.AfterFunc(2*time.Second, signalingGo)
		return
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
	s := strings.Replace(fps[0].Value, ":", "", -1)
	return strings.ToUpper(s)
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

	schema := "wss"
	if Conf.insecure {
		schema = "ws"
	}
	url := url.URL{Scheme: schema, Host: Conf.peerbookHost, Path: "/ws",
		RawQuery: params.Encode()}
	conn, resp, err := cstDialer.Dial(url.String(), nil)
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
	fp, found := m["source_fp"].(string)
	if !found {
		return fmt.Errorf("Missing 'source_fp' paramater")
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
		//TODO: for some reason we need to get a candidate before we carry on
		//      of the pty return EOF on first read. strange days
		time.AfterFunc(time.Second/5, func() {
			wsWriteM.Lock()
			c.WriteMessage(websocket.TextMessage, j)
			wsWriteM.Unlock()
		})
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
			if peer.PC == nil {
				peer.pendingCandidates <- &can.Candidate
				return nil
			}
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
