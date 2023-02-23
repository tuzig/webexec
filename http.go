package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v3"
	"github.com/rs/cors"
)

// AuthBackend is the interface that wraps the basic authentication methods
type AuthBackend interface {
	// IsAthorized checks if the fingerprint is authorized to connect
	IsAuthorized(token string) bool
}

type ConnectHandler struct {
	authBackend AuthBackend
}

// ConnectRequest is the schema for the connect POST request
type ConnectRequest struct {
	Fingerprint string `json:"fingerprint"`
	APIVer      int    `json:"api_version"`
	Offer       string `json:"offer"`
}

func NewConnectHandler(backend AuthBackend) *ConnectHandler {
	return &ConnectHandler{backend}
}

// HTTPGo starts to listen and serve http requests
func HTTPGo(address string, authBackend AuthBackend) *http.Server {
	connectHandler := NewConnectHandler(authBackend)
	http.HandleFunc("/connect", connectHandler.HandleConnect)
	h := cors.Default().Handler(http.DefaultServeMux)
	server := &http.Server{Addr: address, Handler: h}
	go server.ListenAndServe()
	return server
}

// HandleConnect is called when a client requests the connect endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func (h *ConnectHandler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	var req ConnectRequest

	if r.Method != "POST" {
		Logger.Infof("Got an http request with bad method %q\n", r.Method)
		http.Error(w, "This endpoint accepts only POST requests",
			http.StatusMethodNotAllowed)
		return
	}
	Logger.Info("Got a new post request")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	err := parsePeerReq(r.Body, &req, &offer)
	if err != nil {
		Logger.Errorf(err.Error())
	}
	a := r.RemoteAddr
	localhost := (len(a) >= 9 && a[:9] == "127.0.0.1") ||
		(len(a) >= 5 && a[:5] == "[::1]")
	fp, err := GetFingerprint(&offer)
	if err != nil {
		Logger.Warnf("Failed to get fingerprint from sdp: %w", err)
	}
	if !localhost && h.authBackend.IsAuthorized(fp) {
		// check for Bearer token
		auth := r.Header.Get("Bearer")
		if auth == "" || !h.authBackend.IsAuthorized(auth) {
			Logger.Warnf("Unauthorized access from %s", a)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	peer, err := NewPeer(req.Fingerprint)
	if err != nil {
		msg := fmt.Sprintf("Failed to create a new peer: %s", err)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	answer, err := peer.Listen(offer)
	if err != nil {
		msg := fmt.Sprintf("Peer failed to listen : %s", err)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	// reply with server's key
	payload := make([]byte, 4096)
	l, err := EncodeOffer(payload, answer)
	if err != nil {
		msg := fmt.Sprintf("Failed to encode offer : %s", err)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	w.Write(payload[:l])
}

func parsePeerReq(message io.Reader, cr *ConnectRequest,
	offer *webrtc.SessionDescription) error {

	dec := json.NewDecoder(message)
	err := dec.Decode(cr)
	if err != nil {
		return fmt.Errorf("Failed to read connection request: %w", err)
	}
	err = DecodeOffer(offer, []byte(cr.Offer))
	if err != nil {
		return fmt.Errorf("Failed to decode client's offer: %w", err)
	}
	// ensure it's the same fingerprint as the one signalling got
	return nil
}

// GetFingerprint extract the fingerprints from a client's offer and returns
// a compressed fingerprint
func GetFingerprint(offer *webrtc.SessionDescription) (string, error) {
	s, err := offer.Unmarshal()
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal sdp: %w", err)
	}
	var f string
	if fingerprint, haveFingerprint := s.Attribute("fingerprint"); haveFingerprint {
		f = fingerprint
	} else {
		for _, m := range s.MediaDescriptions {
			if fingerprint, found := m.Attribute("fingerprint"); found {
				f = fingerprint
				break
			}
		}
	}
	if f == "" {
		return "", fmt.Errorf("Offer has no fingerprint: %v", offer)
	}
	return compressFP(f), nil
}
