package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

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
	if r.Method != "POST" {
		Logger.Infof("Got an http request with bad method %q\n", r.Method)
		http.Error(w, "This endpoint accepts only POST requests",
			http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	offer, e := ioutil.ReadAll(io.LimitReader(r.Body, 4096))
	if e != nil {
		Logger.Errorf("Failed to read http request body: %s", e)
	}
	// Logger.Infof("Got a valid POST request with offer of len: %d", l)
	peer, err := NewPeer(string(offer))
	if err != nil {
		msg := fmt.Sprintf("Failed to create a new peer: %s", err)
		Logger.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	// reply with server's key
	payload := []byte(peer.Answer)
	w.Write(payload)
}
