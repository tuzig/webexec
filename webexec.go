package main

import (

	"github.com/afittestide/webexec/server"
)

func main() {
	err := server.NewHTTPServer("0.0.0.0:8888")
	if err != nil {
		panic(err)
	}
	// Block forever
	select {}
}
