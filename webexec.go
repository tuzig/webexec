package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/afittestide/webexec/server"
)

func attachKillHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		os.Exit(0)
	}()
}
func main() {
	attachKillHandler()
	log.Printf("Starting http server on port 8888")
	server.HTTPGo("0.0.0.0:8888")
	for {
	}
}
