package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v3"
	"github.com/rs/cors"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type AddressType string

type AuthBackend interface {
	// IsAthorized checks if the fingerprint is authorized to connect
	IsAuthorized(tokens ...string) bool
}
type ConnectHandler struct {
	authBackend AuthBackend
	peerConf    *peers.Conf
	logger      *zap.SugaredLogger
}

// ConnectRequest is the schema for the connect POST request
type ConnectRequest struct {
	Fingerprint string `json:"fingerprint"`
	APIVer      int    `json:"api_version"`
	Offer       string `json:"offer"`
}

func NewConnectHandler(
	backend AuthBackend, conf *peers.Conf, logger *zap.SugaredLogger) *ConnectHandler {

	return &ConnectHandler{
		authBackend: backend,
		peerConf:    conf,
		logger:      logger,
	}
}

// StartHTTPServer starts a http server that listens to the given address
// and serves the connect endpoint.
func StartHTTPServer(lc fx.Lifecycle, address AddressType,
	c *ConnectHandler, logger *zap.SugaredLogger) *http.Server {

	c.peerConf.Logger = logger
	http.HandleFunc("/connect", c.HandleConnect)
	h := cors.Default().Handler(http.DefaultServeMux)
	server := &http.Server{Addr: string(address), Handler: h}
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	err := parsePeerReq(r.Body, &req, &offer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a := r.RemoteAddr
	localhost := (len(a) >= 9 && a[:9] == "127.0.0.1") ||
		(len(a) >= 5 && a[:5] == "[::1]")
	fp, err := peers.GetFingerprint(&offer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get fingerprint from sdp: %s", err),
			http.StatusBadRequest)
		return
	}
	bearer := r.Header.Get("Bearer")
	authorized := localhost || h.authBackend.IsAuthorized(fp, bearer)
	h.logger.Debugf("Client %s is %b authorized", fp, authorized)
	if !authorized {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	peer, err := peers.NewPeer(h.peerConf)
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
