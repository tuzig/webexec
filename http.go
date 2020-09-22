package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/rs/cors"
)

func HTTPGo(address string) (e error) {
	h, e := ConnectHandler()
	if e != nil {
		log.Fatal(e)
		return
	}

	return http.ListenAndServe(address, h)
}

// ConnectHandler listens for POST requests on /connect.
// A valid request should have an encoded WebRTC offer as its body.
func ConnectHandler() (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", handleConnect)
	return cors.Default().Handler(mux), nil
}

// handleConnect is called when a client requests the connect endpoint
// it should be a post and the body webrtc's client offer.
// In reponse the handlers send the server's webrtc's offer.
func handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		//TODO: log the error
		Logger.Infof("Got an http request with bad method %q\n", r.Method)
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	offer, e := ioutil.ReadAll(r.Body)
	if e != nil {
		Logger.Errorf("Failed to read http request body: %s", e)
	}
	// Logger.Infof("Got a valid POST request with offer of len: %d", l)
	peer, err := NewPeer(string(offer))
	if err != nil {
		msg := fmt.Sprintf("NewPeer failed with: %s", err)
		Logger.Error(msg)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(msg))
	}
	// reply with server's key
	payload := []byte(peer.Answer)
	w.Write(payload)
}
