package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pion/webrtc/v3"
	"github.com/rs/cors"
)

// ConnectRequest is the schema for the connect POST request
type ConnectRequest struct {
	Fingerprint string `json:"fingerprint"`
	APIVer      int    `json:"api_version"`
	Offer       string `json:"offer"`
}

// HTTPGo starts to listen and serve http requests
func HTTPGo(address string) {
	http.HandleFunc("/connect", handleConnect)
	h := cors.Default().Handler(http.DefaultServeMux)
	err := http.ListenAndServe(address, h)
	Logger.Errorf("%s", err)
	done <- os.Interrupt
}

// handleConnect is called when a client requests the connect endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func handleConnect(w http.ResponseWriter, r *http.Request) {
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
	fp, err := GetFingerprint(&offer)
	if err != nil {
		Logger.Warnf("Failed to get fingerprint from sdp: %w", err)
	}
	if !IsAuthorized(fp) {
		msg := "Unknown client fingerprint"
		Logger.Info(msg)
		http.Error(w, msg, http.StatusUnauthorized)
		return
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
