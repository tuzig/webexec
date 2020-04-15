package webexec

import (
	"fmt"
	"github.com/pion/webrtc/v2"
	"testing"
)

// expectedLabel represents the label of the data channel we are trying to test.
// Some other channels may have been created during initialization (in the Wasm
// bindings this is a requirement).
const expectedLabel = "data"

func TestSimpleEcho(t *testing.T) {

	done := make(chan bool)
	server, err := NewServer("password")
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	fmt.Printf("%v", server)

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("webexec", nil)
	dc.OnOpen(func() {
		err := dc.SendText("password")
		if err != nil {
			t.Fatalf("Failed to send string on data channel")
		}
		err = dc.SendText("echo hello world")
		if err != nil {
			t.Fatalf("Failed to send string on data channel")
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString && string(msg.Data) != "hello world" {
			t.Fatalf("Got bad msg: %q", msg.Data)
		}
		done <- true
	})
	<-done
}
