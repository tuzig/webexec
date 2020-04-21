package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/afittestide/webexec/signal"
	"github.com/pion/webrtc/v2"
	"github.com/rs/cors"
)

type ConnectAPI struct {
	Offer string
}

func startWebRTCServer(remote string) []byte {
	offer := webrtc.SessionDescription{}
	signal.Decode(remote, &offer)
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	pc, err := NewWebRTCServer(config)
	if err != nil {
		panic(err)
	}
	err = pc.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = pc.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}
	return []byte(signal.Encode(answer))
}

func HTTPGo(address string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			log.Printf("Got an http request with bad method %q\n", r.Method)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		decoder := json.NewDecoder(r.Body)
		var t ConnectAPI
		e := decoder.Decode(&t)
		log.Printf("Got a valid POST request with data: %v", t)
		if e != nil {
			panic(e)
		}
		answer := startWebRTCServer(t.Offer)
		// Output the answer in base64 so we can paste it in browser
		w.Write(answer)
	})
	handler := cors.Default().Handler(mux)
	log.Fatal(http.ListenAndServe(address, handler))
}
