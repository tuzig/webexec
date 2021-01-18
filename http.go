package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v3"
	"github.com/rs/cors"
)

// HTTPGo starts to listen and serve http requests
func HTTPGo(address string) error {
	h, e := ConnectHandler()
	if e != nil {
		return e
	}

	return http.ListenAndServe(address, h)
}

// ConnectHandler listens for POST requests on /connect.
// A valid request should have an encoded WebRTC offer as its body.
func ConnectHandler() (http.Handler, error) {
	http.HandleFunc("/connect", handleConnect)
	return cors.Default().Handler(http.DefaultServeMux), nil
}

// handleConnect is called when a client requests the connect endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func handleConnect(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	if r.Method != "POST" {
		Logger.Infof("Got an http request with bad method %q\n", r.Method)
		http.Error(w, "This endpoint accepts only POST requests",
			http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	remote := make([]byte, 4096)
	l, err := r.Body.Read(remote)
	if err != nil && err != io.EOF {
		Logger.Errorf("Failed to read http request body: %s", err)
	}
	// Logger.Infof("Got a valid POST request with offer of len: %d", l)
	err = DecodeOffer(&offer, remote[:l])
	if err != nil {
		msg := fmt.Sprintf("Failed to decode offer: %s\n%q", err, remote)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	Logger.Info("Got New Peer")
	peer, err := NewPeer()
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
	l, err = EncodeOffer(payload, answer)
	if err != nil {
		msg := fmt.Sprintf("Failed to encode offer : %s", err)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	w.Write(payload[:l])
}
