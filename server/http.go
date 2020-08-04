package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/rs/cors"
)

// ConnectHandler listens for POST requests on /connect.
// A valid request should have an encoded WebRTC offer as its body.
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
		offer := make([]byte, 4096)
		l, e := r.Body.Read(offer)
		if e != io.EOF {
			e = fmt.Errorf("Failed to read http request body: %q", e)
			return
		}
		log.Printf("Got a valid POST request with offer: %q", string(offer[:l]))
		peer := s.Listen(string(offer[:l]))
		// reply with server's key
		w.Write(peer.Answer)
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
		log.Fatal(e)
		return
	}
	log.Fatal(http.ListenAndServe(address, h))
	return
}
