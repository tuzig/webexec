package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type TURNResponse struct {
	TTL     int                 `json:"ttl"`
	Servers []map[string]string `json:"ice_servers"`
}

var PBICEServer *webrtc.ICEServer
var wsWriteM sync.Mutex
var PeerbookConn *websocket.Conn

func verifyPeer(host string) (bool, error) {
	fp := getFP()
	msg := map[string]string{"fp": fp, "email": Conf.email,
		"kind": "webexec", "name": Conf.name}
	m, err := json.Marshal(msg)
	schema := "https"
	if Conf.insecure {
		schema = "http"
	}
	url := url.URL{Scheme: schema, Host: host, Path: "/verify"}
	resp, err := http.Post(url.String(), "application/json", bytes.NewBuffer(m))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		return false, fmt.Errorf(string(b))
	}
	var ret map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		return false, err
	}
	v, found := ret["verified"]
	if found {
		return v.(bool), nil
	}
	_, found = ret["peers"]
	if found {
		return true, nil
	}
	return false, nil
}
func getICEServers(host string) ([]webrtc.ICEServer, error) {
	if host == "" {
		return Conf.iceServers, nil
	}
	if PBICEServer == nil {
		schema := "https"
		if Conf.insecure {
			schema = "http"
		}
		url := url.URL{Scheme: schema, Host: host, Path: "/turn"}
		resp, err := http.Post(url.String(), "application/json", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			return nil, fmt.Errorf(string(b))
		}
		var d TURNResponse
		err = json.NewDecoder(resp.Body).Decode(&d)
		if err != nil {
			return nil, err
		}
		if len(d.Servers) == 0 {
			return nil, nil
		}
		s := d.Servers[0]

		PBICEServer = &webrtc.ICEServer{
			URLs:       []string{s["urls"]},
			Username:   s["username"],
			Credential: s["credential"],
		}

	}
	return append(Conf.iceServers, *PBICEServer), nil
}

func peerbookGo() {
	go func() {
		var err error
		for {
			if PeerbookConn == nil {
				PeerbookConn, err = dialWS()
				if err != nil {
					Logger.Errorf("Failed to dial the peerbook server: %q", err)
					PeerbookConn = nil
					time.Sleep(Conf.peerbookTimeout)
					continue
				}
				Logger.Infof("Connected to peerbook")
			}

			mType, m, err := PeerbookConn.ReadMessage()
			if err != nil {
				Logger.Warnf("Signaling read error: %w", err)
				time.Sleep(Conf.peerbookTimeout)
				PeerbookConn = nil
				continue
			}
			if mType == websocket.TextMessage {
				Logger.Info("Received text message", string(m))
				err = handleMessage(PeerbookConn, m)
				if err != nil {
					Logger.Errorf("Failed to handle message: %w", err)
				}
			}
		}
	}()
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
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
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
