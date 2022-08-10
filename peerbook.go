package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

const writeWait = time.Second * 10

type TURNResponse struct {
	TTL     int                 `json:"ttl"`
	Servers []map[string]string `json:"ice_servers"`
}

var PBICEServer *webrtc.ICEServer
var wsWriteM sync.Mutex

// outChan is used to send messages to peerbook
var outChan chan []byte

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
	var c *websocket.Conn
	outChan = make(chan []byte)
	done := make(chan struct{})
	// reader
	go func() {
		var err error
		defer close(done)
		for {
			if c == nil {
				Logger.Infof("Dialing PeerBook in %s", time.Now())
				c, err = dialWS()
				if err != nil {
					Logger.Errorf("Failed to dial the peerbook server: %q", err)
					c = nil
					time.Sleep(Conf.peerbookTimeout)
					continue
				}
				Logger.Infof("Connected to peerbook")
			}

			mType, m, err := c.ReadMessage()
			if err != nil {
				Logger.Warnf("Signaling read error: %w", err)
				time.Sleep(Conf.peerbookTimeout)
				c = nil
				continue
			}
			if mType == websocket.TextMessage {
				Logger.Infof("Received text message %s", string(m))
				err = handleMessage(c, m)
				if err != nil {
					Logger.Errorf("Failed to handle message: %w", err)
				}
			} else {
				Logger.Infof("Ignoring mssage of type %d", mType)
			}
		}
	}()
	// writer
	go func() {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		for {
			Logger.Info("in sender for")
			select {
			case <-done:
				return
			case msg, ok := <-outChan:
				if !ok {
					Logger.Errorf("Got a bad message to send")
					continue
				}
				Logger.Infof("sending to pb: %s", msg)
				c.SetWriteDeadline(time.Now().Add(writeWait))
				err := c.WriteMessage(websocket.TextMessage, msg)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err,
						websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						Logger.Warnf("Failed to send websocket message: %s", err)
						return
					}
					continue
				}
			case <-interrupt:
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				Logger.Info("Closing peerbook connection")
				err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				if err != nil {
					Logger.Errorf("Failed to close websocket", err)
					return
				}
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				return
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
		Logger.Infof("Got a status message: %v %s", code, m["text"].(string))
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
				outChan <- j
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
		outChan <- j
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
