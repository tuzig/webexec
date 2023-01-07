package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dchest/uniuri"
	"github.com/pion/webrtc/v3"
)

type LiveOffer struct {
	// the http get request that's waiting for the next candidate
	w        *http.ResponseWriter
	m        sync.Mutex
	incoming chan webrtc.ICECandidateInit
	//TODO: refactor and remove '*'
	cs chan *webrtc.ICECandidate
	p  *Peer
	id string
}

var currentOffers map[string]*LiveOffer
var coMutex sync.Mutex

func (lo *LiveOffer) handleIncoming(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case can := <-lo.incoming:
			if lo.p.PC != nil {
				Logger.Infof("Adding ICE candidate: %v", can)
				err := lo.p.PC.AddICECandidate(can)
				if err != nil {
					Logger.Errorf("Failed to add ICE candidate: " + err.Error())
				}
			} else {
				Logger.Warnf("Ignoring candidate: %v", can)
			}
		}
	}
}

func (la *LiveOffer) OnCandidate(can *webrtc.ICECandidate) {
	if can != nil {
		Logger.Infof("appending a candidate to %q: %v", la.id, can)
		la.cs <- can
	}
}
func GetSockFP() string {
	return RunPath("webexec.sock")
}
func StartSock() (*http.Server, error) {
	currentOffers = make(map[string]*LiveOffer)
	fp := GetSockFP()
	_, err := os.Stat(fp)
	if err == nil {
		os.Remove(fp)
	} else if errors.Is(err, os.ErrNotExist) {
		// file does not exist, let's make sure the dir does
		dir := RunPath("")
		_, err := os.Stat(dir)
		if errors.Is(err, os.ErrNotExist) {
			err = os.Mkdir(dir, 0755)
			if err != nil {
				Logger.Errorf("Failed to make dir %q: %s", dir, err)
				return nil, err
			}
		} else if err != nil {
			Logger.Errorf("Failed to stat dir %q: %s", dir, err)
			return nil, err
		}
	}
	m := http.ServeMux{}
	m.Handle("/layout", http.HandlerFunc(handleLayout))
	m.Handle("/offer/", http.HandlerFunc(handleOffer))
	server := http.Server{Handler: &m}
	l, err := net.Listen("unix", fp)
	if err != nil {
		return nil, fmt.Errorf("Failed to listen to unix socket: %s", err)
	}
	go func() {
		server.Serve(l)
		// this happens after main calles server.Shutdown()
		os.Remove(fp)
	}()
	Logger.Infof("Listening for requests on %q", fp)
	return &server, nil
}

func handleLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write(Payload)
	} else if r.Method == "POST" {
		b, _ := ioutil.ReadAll(r.Body)
		Payload = b
	}
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
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
		select {
		case c := <-a.cs:
			m, err := json.Marshal(c.ToJSON())
			if err != nil {
				http.Error(w, "Failed to marshal candidate", http.StatusInternalServerError)
			} else {
				Logger.Infof("replying to GET with: %v", string(m))
				w.Write(m)
			}
			return
		case <-time.After(time.Second * 5):
			if a.p.PC == nil {
				http.Error(w, "Connection failed", http.StatusServiceUnavailable)
			} else if a.p.PC.ConnectionState() == webrtc.PeerConnectionStateConnected {
				http.Error(w, "Connection established", http.StatusNoContent)
			}
		}
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
			msg := fmt.Sprintf("Failed to get fingerprint from offer: %s", err)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		peer, err := NewPeer(fp)
		if err != nil {
			http.Error(w, "Failed to create a new peer", http.StatusInternalServerError)
			return
		}
		h := uniuri.New()
		// TODO: move the 5 to conf, to refactored ice section
		lo := &LiveOffer{p: peer, id: h,
			cs:       make(chan *webrtc.ICECandidate, 5),
			incoming: make(chan webrtc.ICECandidateInit, 5),
		}
		coMutex.Lock()
		currentOffers[h] = lo
		coMutex.Unlock()
		peer.PC.OnICECandidate(lo.OnCandidate)
		err = peer.PC.SetRemoteDescription(offer)
		if err != nil {
			msg := fmt.Sprintf("Peer failed to listen: %s", err)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		go lo.handleIncoming(ctx)
		answer, err := peer.PC.CreateAnswer(nil)
		if err != nil {
			http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		}
		err = peer.PC.SetLocalDescription(answer)
		if err != nil {
			http.Error(w, "Failed to set local description", http.StatusInternalServerError)
			return
		}

		m := map[string]string{"type": "answer", "sdp": answer.SDP, "id": h}
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
		// cleanup: 30 should be in the conf under the [ice] section
		time.AfterFunc(30*time.Second, func() {
			Logger.Info("After 30 secs")
			cancel()
			coMutex.Lock()
			delete(currentOffers, h)
			coMutex.Unlock()
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
		a.incoming <- webrtc.ICECandidateInit{Candidate: string(can)}
	}
}
