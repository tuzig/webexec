package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"github.com/rs/cors"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const SessionTTL = time.Second * 30

type AddressType string

type AuthBackend interface {
	// IsAthorized checks if the fingerprint is authorized to connect
	IsAuthorized(tokens ...string) bool
}
type ConnectHandler struct {
	authBackend AuthBackend
	peerConf    *peers.Conf
	logger      *zap.SugaredLogger
	address     AddressType
	sessions    map[uuid.UUID]*peers.Peer
}

// ConnectRequest is the schema for the connect POST request
type ConnectRequest struct {
	Fingerprint string `json:"fingerprint"`
	APIVer      int    `json:"api_version"`
	Offer       string `json:"offer"`
}

func NewConnectHandler(
	backend AuthBackend, conf *peers.Conf, logger *zap.SugaredLogger) *ConnectHandler {

	adress := os.Getenv("WEBEXEC_SERVER_URL")
	if adress == "" {
		adress = "http://localhost:7777"
	}
	logger.Infof("Using %s as server address", adress)

	return &ConnectHandler{
		authBackend: backend,
		peerConf:    conf,
		logger:      logger,
		address:     AddressType(adress),
		sessions:    make(map[uuid.UUID]*peers.Peer),
	}
}

// StartHTTPServer starts a http server that listens to the given address
// and serves the connect endpoint.
func StartHTTPServer(lc fx.Lifecycle, c *ConnectHandler, address AddressType,
	logger *zap.SugaredLogger) *http.Server {

	c.peerConf.Logger = logger
	c.AddHandlers(http.DefaultServeMux)
	server := &http.Server{
		Addr:    string(address),
		Handler: c.GetHandler()}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("Starting HTTP server")
			go server.ListenAndServe()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// peers.Shutdown()
			logger.Info("Stopping HTTP server")
			return server.Shutdown(ctx)
		},
	})
	return server
}
func (h *ConnectHandler) GetHandler() http.Handler {
	handler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"POST", "PATCH", "DELETE"},
		AllowedHeaders: []string{"*"},
	}).Handler(http.DefaultServeMux)
	return handler
}
func (h *ConnectHandler) AddHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/connect", h.HandleConnect)
	mux.HandleFunc("/offer", h.HandleOffer)
	mux.HandleFunc("/candidates/", h.HandleCandidate)
}

func (h *ConnectHandler) IsAuthorized(r *http.Request, fp string) bool {
	// check for localhost
	a := r.RemoteAddr
	if (len(a) >= 9 && a[:9] == "127.0.0.1") ||
		(len(a) >= 5 && a[:5] == "[::1]") {
		return true
	}
	bearer := ""
	authorization := r.Header.Get("Authorization")
	// ensure token length is at least 8
	if authorization != "" {
		if len(authorization) < 8 {
			h.logger.Warnf("Token too short: %s", authorization)
			return false
		}
		bearer = authorization[7:]
	}
	h.logger.Debugf("Client %s with token %s trying to connect", fp, bearer)
	return h.authBackend.IsAuthorized(fp, bearer)
}

// HandleOffer is called when a client requests the whip endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func (h *ConnectHandler) HandleOffer(w http.ResponseWriter, r *http.Request) {
	h.logger.Debugf("Got request from %s", r.RemoteAddr)
	if r.Method != "POST" {
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}
	// read the sdp string from the request body
	sdp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(sdp),
	}
	fp, err := peers.GetFingerprint(&offer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get fingerprint from sdp: %s", err),
			http.StatusBadRequest)
		return
	}
	if !h.IsAuthorized(r, fp) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	peer, err := peers.NewPeer(fp, h.peerConf)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create a new peer: %s", err), http.StatusInternalServerError)
		return
	}
	answer, err := peer.Listen(offer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Peer failed to listen : %s", err), http.StatusInternalServerError)
		return
	}
	sessionID := uuid.New()
	h.sessions[sessionID] = peer
	go func() {
		time.Sleep(SessionTTL)
		_, found := h.sessions[sessionID]
		if !found {
			return
		}
		delete(h.sessions, sessionID)
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Location")

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("ETag", fmt.Sprintf("%x", time.Now().Unix()))
	url := fmt.Sprintf("%s/candidates/%s", h.address, sessionID)
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(answer.SDP))
}

// HandleCandidate is called when a client requests the candidate endpoint
// it should be a patch and the body webrtc's client candidate.
func (h *ConnectHandler) HandleCandidate(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(r.URL.Path[len("/candidates/"):])
	if err != nil {
		http.Error(w, "Invalid session id", http.StatusBadRequest)
		return
	}
	if r.Method == "DELETE" {
		// get the session id from the url
		delete(h.sessions, sessionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != "PATCH" {
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}
	// ensure uuid is valid
	peer, found := h.sessions[sessionID]
	if !found {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	candidateData, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	var candidate webrtc.ICECandidateInit
	err = json.Unmarshal(candidateData, &candidate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to decode candidate - %s", err),
			http.StatusBadRequest)
		return
	}
	err = peer.AddCandidate(candidate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleConnect is called when a client requests the connect endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func (h *ConnectHandler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	var req ConnectRequest

	if r.Method != "POST" {
		http.Error(w, "This endpoint accepts only POST requests",
			http.StatusMethodNotAllowed)
		return
	}
	err := parsePeerReq(r.Body, &req, &offer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fp, err := peers.GetFingerprint(&offer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get fingerprint from sdp: %s", err),
			http.StatusBadRequest)
		return
	}
	if !h.IsAuthorized(r, fp) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	peer, err := peers.NewPeer(fp, h.peerConf)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create a new peer: %s", err), http.StatusInternalServerError)
		return
	}
	answer, err := peer.Listen(offer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Peer failed to listen : %s", err), http.StatusInternalServerError)
		return
	}
	// reply with server's key
	payload := make([]byte, 4096)
	l, err := peers.EncodeOffer(payload, answer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode offer : %s", err), http.StatusBadRequest)
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
	err = peers.DecodeOffer(offer, []byte(cr.Offer))
	if err != nil {
		return fmt.Errorf("Failed to decode client's offer: %w", err)
	}
	// ensure it's the same fingerprint as the one signalling got
	return nil
}
