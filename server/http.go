package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/rs/cors"
)

type ConnectAPI struct {
	Offer string
}

//
// ConnectHandler starts the webrtcServer and
func ConnectHandler() (h http.Handler, e error) {
	s, e := NewWebRTCServer()
	if e != nil {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			e = fmt.Errorf("Got an http request with bad method %q\n", r.Method)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		decoder := json.NewDecoder(r.Body)
		var t ConnectAPI
		e := decoder.Decode(&t)
		log.Printf("Got a valid POST request with data: %v", t)
		if e != nil {
			e = fmt.Errorf("Failed to decode client's key: %v", e)
			return
		}
		k := s.Listen(t.Offer)
		// reply with server's key
		w.Write(k)
	})
	h = cors.Default().Handler(mux)
	return
}

func NewHTTPListner() (l net.Listener, p int, e error) {
	l, e = net.Listen("tcp", ":0")
	if e != nil {
		return
	}
	p = l.Addr().(*net.TCPAddr).Port
	return
}

func HTTPGo(address string) (e error) {
	h, e := ConnectHandler()
	if e != nil {
		return
	}
	log.Fatal(http.ListenAndServe(address, h))

	return
}
