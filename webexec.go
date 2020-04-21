package main

import (
	"log"

	"github.com/afittestide/webexec/server"
)

func main() {
	log.Printf("Starting http server on port 8888")
	server.HTTPGo("0.0.0.0:8888")
}
