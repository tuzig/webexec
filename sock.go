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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dchest/uniuri"
	"github.com/pion/webrtc/v3"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/fx"
	"golang.design/x/clipboard"
)

type SocketStartParams struct {
	fp string
}
type sockServer struct {
	currentOffers map[string]*LiveOffer
	coMutex       sync.Mutex
	conf          *peers.Conf
}

type LiveOffer struct {
	// the http get request that's waiting for the next candidate
	w        *http.ResponseWriter
	m        sync.Mutex
	incoming chan webrtc.ICECandidateInit
	//TODO: refactor and remove '*'
	cs chan *webrtc.ICECandidate
	p  *peers.Peer
	id string
}

const socketFileName = "webexec.sock"

var socketFilePath string

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
func NewSockServer(conf *peers.Conf) *sockServer {
	return &sockServer{
		currentOffers: make(map[string]*LiveOffer),
		conf:          conf,
	}
}

// GetSockFP returns the path to the socket file
func GetSockFP() string {
	if socketFilePath == "" {
		socketFilePath = RunPath(socketFileName)
	}
	return socketFilePath
}

func (s *sockServer) handleClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		var reply []byte
		peer := peers.GetActivePeer()
		if peer != nil {
			Logger.Info("Reading the peers' clipboard")
			clip, err := peer.SendControlMessageAndWait("get_clipboard", nil)
			if err != nil {
				Logger.Errorf("Failed to send the paste message: %s", err)
				http.Error(w, "Failed to send the paste message", http.StatusInternalServerError)
				return
			}
			reply = []byte(clip)
		} else {
			// use the local clipboard as a fallback
			Logger.Info("Got clipboard GET, using local clipboard")
			reply = clipboard.Read(clipboard.FmtText)
		}
		w.Write(reply)
	} else if r.Method == "POST" {
		mimetype := r.Header.Get("Content-Type")
		b, _ := ioutil.ReadAll(r.Body)
		peer := peers.GetActivePeer()
		if peer != nil {
			// check the incoming mime type and send the appropriate message
			Logger.Infof("Setting peers' clipboard with mime type %q", mimetype)
			args := peers.SetClipboardArgs{
				MimeType: mimetype,
				Data:     string(b),
			}

			err := peer.SendControlMessage("set_clipboard", args)
			if err != nil {
				Logger.Errorf("Failed to send the paste message: %s", err)
				http.Error(w, "Failed to send the paste message", http.StatusInternalServerError)
				return
			}
		} else {
			Logger.Info("Got clipboard POST, using local clipboard")
			if strings.Contains(mimetype, "text/plain") {
				clipboard.Write(clipboard.FmtText, b)
			} else {
				clipboard.Write(clipboard.FmtImage, b)
			}
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func StartSocketServer(lc fx.Lifecycle, s *sockServer, params SocketStartParams) (*http.Server, error) {
	if params.fp == "" {
		socketFilePath = RunPath(socketFileName)
	} else {
		socketFilePath = params.fp
	}
	_, err := os.Stat(params.fp)
	if err == nil {
		Logger.Infof("Removing stale socket file %q", params.fp)
		err = os.Remove(params.fp)
		if err != nil {
			Logger.Errorf("Failed to remove stale socket file %q: %s", params.fp, err)
			return nil, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// file does not exist, extract the directory and create it
		dir := filepath.Dir(params.fp)
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
	m.Handle("/status", http.HandlerFunc(s.handleStatus))
	m.Handle("/layout", http.HandlerFunc(s.handleLayout))
	m.Handle("/offer/", http.HandlerFunc(s.handleOffer))
	m.Handle("/clipboard", http.HandlerFunc(s.handleClipboard))
	server := http.Server{Handler: &m}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			l, err := net.Listen("unix", params.fp)
			if err != nil {
				return fmt.Errorf("Failed to listen to unix socket: %s", err)
			}
			go server.Serve(l)
			Logger.Infof("Listening for requests on %q", params.fp)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			Logger.Info("Stopping socket server")
			err := server.Shutdown(ctx)
			os.Remove(params.fp)
			Logger.Info("Socket server down")
			return err
		},
	})
	return &server, nil
}

func (s *sockServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write([]byte("READY"))
	}
	/* TODO: return status of all connected peers
	if len(peers.Peers) == 0 {
		fmt.Println("No peers connected")
	} else {
		fmt.Println("Connected peers:")
		for _, peer := range peers.Peers {
			fmt.Printf("  %s", peer.FP)
		}
	}
	*/
}
func (s *sockServer) handleLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write(peers.Payload)
	} else if r.Method == "POST" {
		b, _ := ioutil.ReadAll(r.Body)
		peers.Payload = b
	}
}

func (s *sockServer) handleOffer(w http.ResponseWriter, r *http.Request) {
	cs := strings.Split(r.URL.Path[1:], "/")
	if r.Method == "GET" {
		// store the w and use it to reply with new candidates when they're available
		if len(cs) == 1 || len(cs) > 2 {
			http.Error(w, "GET path should be in the form `/accept/[hash]` ",
				http.StatusBadRequest)
			return
		}
		h := cs[1]
		a := s.currentOffers[h]
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
			a.p.Lock()
			defer a.p.Unlock()
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
		fp, err := peers.GetFingerprint(&offer)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get fingerprint from sdp: %s", err),
				http.StatusBadRequest)
			return
		}

		peer, err := peers.NewPeer(fp, s.conf)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create a new peer: %s", err),
				http.StatusInternalServerError)
			return
		}
		h := uniuri.New()
		// TODO: move the 5 to conf, to refactored ice section
		lo := &LiveOffer{p: peer, id: h,
			cs:       make(chan *webrtc.ICECandidate, 5),
			incoming: make(chan webrtc.ICECandidateInit, 5),
		}
		s.coMutex.Lock()
		s.currentOffers[h] = lo
		s.coMutex.Unlock()
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
			s.coMutex.Lock()
			delete(s.currentOffers, h)
			s.coMutex.Unlock()
		})
		return
	} else if r.Method == "PUT" {
		if len(cs) == 1 || len(cs) > 2 {
			http.Error(w, "PUT path should be in the form `/accept/[hash]` ",
				http.StatusBadRequest)
			return
		}
		h := cs[1]
		a := s.currentOffers[h]
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
