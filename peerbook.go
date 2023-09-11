package main

import (
	"bytes"
	"context"
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
	"github.com/tuzig/webexec/peers"
	"go.uber.org/fx"
)

const writeWait = time.Second * 10

var PBICEServers []webrtc.ICEServer
var wsWriteM sync.Mutex

// outChan is used to send messages to peerbook
type PeerbookClient struct {
	outChan  chan []byte
	ws       *websocket.Conn
	peerConf *peers.Conf
	host     string
}

func NewPeerbookClient(peerConf *peers.Conf) *PeerbookClient {
	return &PeerbookClient{
		outChan:  make(chan []byte),
		peerConf: peerConf,
	}
}
func StartPeerbookClient(lc fx.Lifecycle, client *PeerbookClient) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if Conf.peerbookUID == "" {
				Logger.Info("No peerbook user ID configured, skipping peerbook connection")
				return nil
			}

			verified, err := verifyPeer(Conf.peerbookHost)
			if err != nil {
				Logger.Warnf("Got an error verifying peer: %s", err)
			}
			if verified {
				Logger.Infof("Verified by %s as %s", Conf.peerbookHost, Conf.peerbookUID)
			} else {
				fp := getFP()
				Logger.Infof("Unverified, please use Terminal7 to verify fingerprint: %s", fp)
			}
			go client.Go()
			Logger.Info("Started peerbook client")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			Logger.Info("TODO: stop peerbook client")
			return nil
		},
	})
}
func verifyPeer(host string) (bool, error) {
	fp := getFP()
	msg := map[string]string{
		"fp":   fp,
		"uid":  Conf.peerbookUID,
		"kind": "webexec",
		"name": Conf.name}
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
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
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
	} else {
		return false, fmt.Errorf("No verified field in response")
	}
}
func GetICEServers() ([]webrtc.ICEServer, error) {
	host := Conf.peerbookHost
	if host == "" {
		return Conf.iceServers, nil
	}
	if len(PBICEServers) == 0 {
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
		err = json.NewDecoder(resp.Body).Decode(&PBICEServers)
		if err != nil {
			return nil, err
		}
	}
	Logger.Infof("Got %d ICE servers from peerbook", len(PBICEServers))
	return append(Conf.iceServers, PBICEServers...), nil
}

func (pb *PeerbookClient) Go() error {
	done := make(chan struct{})
	// reader
	go func() {
		defer close(done)
		for {
			if pb.ws == nil {
				Logger.Infof("Dialing PeerBook in %s", time.Now())
				err := pb.Dial()
				if err != nil {
					Logger.Errorf("Failed to dial the peerbook server: %q", err)
					pb.ws = nil
					time.Sleep(Conf.peerbookTimeout)
					continue
				}
			}

			mType, m, err := pb.ws.ReadMessage()
			if err != nil {
				Logger.Warnf("Signaling read error: %w", err)
				time.Sleep(Conf.peerbookTimeout)
				pb.ws = nil
				continue
			}
			if mType == websocket.TextMessage {
				Logger.Infof("Received text message %s", string(m))
				err = pb.handleMessage(m)
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
		for {
			Logger.Info("in sender for")
			select {
			case <-done:
				return
			case msg, ok := <-pb.outChan:
				if !ok {
					Logger.Errorf("Got a bad message to send")
					continue
				}
				Logger.Infof("sending to pb: %s", msg)
				pb.ws.SetWriteDeadline(time.Now().Add(writeWait))
				err := pb.ws.WriteMessage(websocket.TextMessage, msg)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err,
						websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						Logger.Warnf("Failed to send websocket message: %s", err)
						return
					}
					continue
				}
			}
		}
	}()
	return nil
}
func getFP() string {
	certs, err := GetCerts()
	if err != nil {
		Logger.Errorf("Failed to get certs: %w", err)
		return ""
	}
	// TODO: is 0 the right choice?
	fps, err := certs[0].GetFingerprints()
	if err != nil {
		Logger.Error("Failed to get fingerprints: %w", err)
		return ""
	}
	s := strings.Replace(fps[0].Value, ":", "", -1)
	return strings.ToUpper(s)
}

func (pb *PeerbookClient) Dial() error {
	var cstDialer = websocket.Dialer{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
	fp := getFP()

	params := url.Values{}
	params.Add("fp", fp)
	params.Add("name", Conf.name)
	params.Add("kind", "webexec")
	params.Add("uid", Conf.peerbookUID)

	schema := "wss"
	if Conf.insecure {
		schema = "ws"
	}
	url := url.URL{Scheme: schema, Host: Conf.peerbookHost, Path: "/ws",
		RawQuery: params.Encode()}
	conn, resp, err := cstDialer.Dial(url.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf(string(bodyBytes))
	}
	if resp.StatusCode == 401 {
		return &errUnauthorized{}
	}
	pb.ws = conn
	return nil
}
func (pb *PeerbookClient) handleMessage(message []byte) error {
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
		peer, found := peers.Peers[fp]
		if found && peer.PC != nil && !pu["verified"].(bool) {
			peer.PC.Close()
			peer.PC = nil
		}
	}
	o, found := m["offer"].(string)
	if found {
		var offer webrtc.SessionDescription
		err = peers.DecodeOffer(&offer, []byte(o))
		if err != nil {
			return fmt.Errorf("Failed to get fingerprint from offer: %w", err)
		}

		offerFP, err := peers.GetFingerprint(&offer)
		if err != nil {
			return fmt.Errorf("Failed to get offer's fingerprint: %w", err)
		}
		if offerFP != fp {
			peers.Peers[fp].PC.Close()
			peers.Peers[fp].PC = nil
			Logger.Warnf("Refusing connection because fp mismatch: %s", fp)
			return fmt.Errorf("Mismatched fingerprint: %s", fp)
		}
		Logger.Info("Authenticated!")
		peer, err := peers.NewPeer(offerFP, pb.peerConf)
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
				pb.outChan <- j
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
		l, err := peers.EncodeOffer(payload, answer)
		if err != nil {
			return fmt.Errorf("Failed to encode offer : %w", err)
		}
		m := map[string]string{"answer": string(payload[:l]), "target": fp}
		Logger.Infof("Sending answer: %v", m)
		j, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("Failed to encode offer : %w", err)
		}
		pb.outChan <- j
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
		peer, found := peers.Peers[fp]
		if found {
			err := peer.AddCandidate(can.Candidate)
			if err != nil {
				return fmt.Errorf("Failed to add ice candidate: %w", err)
			}
		} else {
			return fmt.Errorf("got a candidate from an unknown peer: %s", fp)
		}
	}
	return nil
}
