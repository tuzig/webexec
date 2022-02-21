package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/dchest/uniuri"
	"github.com/pion/webrtc/v3"
)

type LiveOffer struct {
	// the http get request that's waiting for the next candidate
	w  *http.ResponseWriter
	m  sync.Mutex
	cs chan *webrtc.ICECandidate
	p  *Peer
	id string
}

var currentOffers map[string]*LiveOffer

func (la *LiveOffer) OnCandidate(can *webrtc.ICECandidate) {
	if can != nil {
		Logger.Infof("appending a candidate to %q: %v", la.id, can)
		la.cs <- can
	}
}
func StartSock() error {
	currentOffers = make(map[string]*LiveOffer)
	user, err := user.Current()
	if err != nil {
		return fmt.Errorf("Failed to get current user: %s", err)
	}
	fp := fmt.Sprintf("/var/run/webexec.%s.sock", user.Username)

	os.Remove(fp)
	m := http.ServeMux{}
	m.Handle("/layout", http.HandlerFunc(handleLayout))
	m.Handle("/offer/", http.HandlerFunc(hadnleOffer))
	server := http.Server{Handler: &m}
	l, err := net.Listen("unix", fp)
	if err != nil {
		return fmt.Errorf("Failed to listen to unix socket: %s", err)
	}
	go server.Serve(l)
	Logger.Infof("Listening for request on %q", fp)
	return nil
}

func handleLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write(Payload)
	} else if r.Method == "POST" {
		b, _ := ioutil.ReadAll(r.Body)
		Payload = b
	}
}

func hadnleOffer(w http.ResponseWriter, r *http.Request) {
	Logger.Infof("Got request: %v", r)
	cs := strings.Split(r.URL.Path[1:], "/")
	if r.Method == "GET" {
		// store the w and use it to reply with new candidates when they're available
		if len(cs) == 1 || len(cs) > 2 {
			http.Error(w, "GET path should be in the form `/accept/[hash]` ",
				http.StatusBadRequest)
			return
		}
		h := cs[1]
		a := currentOffers[h]
		if a == nil {
			http.Error(w, "request hash is unknown",
				http.StatusBadRequest)
			return
		}
	replying:
		for i := 0; i < 20; i++ {
			select {
			case c := <-a.cs:
				m, err := json.Marshal(c)
				Logger.Infof("replying to GET with: %v", string(m))
				if err != nil {
					http.Error(w, "Failed to marshal candidate", http.StatusInternalServerError)
				} else {
					w.Write(m)
				}
				return
			case <-time.After(time.Second):
				if a.p.PC == nil || a.p.PC.ConnectionState() == webrtc.PeerConnectionStateConnected {
					break replying
				}
			}
		}
		delete(currentOffers, h)
		return
	} else if r.Method == "POST" {
		if len(cs) != 2 || cs[1] != "" {
			http.Error(w, r.URL.Path, http.StatusBadRequest)
			http.Error(w, "POST path should be `/offer` ", http.StatusBadRequest)
			return
		}
		var offer webrtc.SessionDescription
		err := json.NewDecoder(r.Body).Decode(&offer)
		if err != nil {
			http.Error(w, "Failed to decode offer", http.StatusBadRequest)
			return
		}

		fp, err := GetFingerprint(&offer)
		if err != nil {
			http.Error(w, "Failed to get fingerprint from offer", http.StatusBadRequest)
			return
		}
		peer, err := NewPeer(fp)
		if err != nil {
			http.Error(w, "Failed to get fingerprint from offer", http.StatusInternalServerError)
			return
		}
		h := uniuri.New()
		currentOffers[h] = &LiveOffer{p: peer, id: h,
			cs: make(chan *webrtc.ICECandidate, 5)}
		peer.PC.OnICECandidate(currentOffers[h].OnCandidate)
		err = peer.PC.SetRemoteDescription(offer)
		if err != nil {
			msg := fmt.Sprintf("Peer failed to listen: %s", err)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		answer, err := peer.PC.CreateAnswer(nil)
		if err != nil {
			http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		}
		err = peer.PC.SetLocalDescription(answer)
		payload := make([]byte, 4096)
		l, err := EncodeOffer(payload, answer)
		if err != nil {
			http.Error(w, "Failed to encode offer", http.StatusInternalServerError)
			return
		}
		m := map[string]string{"answer": string(payload[:l]), "id": h}
		Logger.Infof("Sending answer: %v", m)
		j, err := json.Marshal(m)
		if err != nil {
			http.Error(w, "Failed to encode offer", http.StatusInternalServerError)
			return
		}
		_, err = w.Write(j)
		if err != nil {
			http.Error(w, "Failed to write answer", http.StatusInternalServerError)
			return
		}
		// cleanup
		time.AfterFunc(30*time.Second, func() {
			delete(currentOffers, h)
		})
		return
	} else if r.Method == "PUT" {
		if len(cs) == 1 || len(cs) > 2 {
			http.Error(w, "PUT path should be in the form `/accept/[hash]` ",
				http.StatusBadRequest)
			return
		}
		h := cs[1]
		a := currentOffers[h]
		if a == nil {
			http.Error(w, "PUT hash is unknown", http.StatusBadRequest)
			return
		}
		can, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read candidate from request body", http.StatusBadRequest)
		}
		Logger.Infof("Adding ICE candidate: %q", string(can))
		err = a.p.PC.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(can)})
		if err != nil {
			http.Error(w, "Failed to add ICE candidate: "+err.Error(), http.StatusBadRequest)
		}
	}
}
