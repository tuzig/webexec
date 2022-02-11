package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
)

func StartSock() error {
	// TODO: use pid for the socket name
	os.Remove("webexec.sock")
	m := http.ServeMux{}
	m.Handle("/layout", http.HandlerFunc(handleLayout))
	server := http.Server{Handler: &m}
	l, err := net.Listen("unix", "webexec.sock")
	if err != nil {
		return fmt.Errorf("Failed to listen to unix socket: %s", err)
	}
	go server.Serve(l)
	Logger.Infof("Listening for request on webexec.sock")
	return nil
}

func handleLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write(Payload)
	} else if r.Method == "POST" {
		b, _ := ioutil.ReadAll(r.Body)
		Payload = b
	}
}
