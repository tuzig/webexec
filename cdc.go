// This files holds the structure and utility functions used by the
// Command & Control data channel (aka cdc

package main

import (
	"encoding/json"
	"fmt"
	"github.com/creack/pty"
)

// MT: "Regular" code should know about tests
// BD: I guess you mean "shouldn't know" and I agree.
//     But practicalicy beats purity and this is the most elegant solution
//     I found.
const AValidTokenForTests = "THEoneANDonlyTOKEN"

/* MT:
- Why JSON? There are many serialization formats out there
-- BD: Because webexec one & only front end is javascript.
- Maybe we can try and decouple the code from the serialization so we can
	swMyitch serialization without much trouble
-- Good idea, but i'd rather wait till when we non-js client
*/

// ErrorArgs is a type that holds the args for an error message
type NAckArgs struct {
	// Desc hold the textual desciption of the error
	Desc string `json:"desc"`
	// Ref holds the message id the error refers to or 0 for system errors
	Ref int `json:"ref"`
}

// AuthArgs is a type that holds client's authentication arguments.
type AuthArgs struct {
	Token string `json:"token"`
}

// AckArgs is a type to hold the args for an Ack message
type AckArgs struct {
	// Ref holds the message id the error refers to or 0 for system errors
	Ref  int             `json:"ref"`
	Body json.RawMessage `json:"body"`
}

// SetPayloadArgs is a type to hold the args for a set_payload type of a message
type SetPayloadArgs struct {
	// Ref holds the message id the error refers to or 0 for system errors
	Payload json.RawMessage `json:"payload"`
}

/* MT: You can use https://pkg.go.dev/github.com/mitchellh/mapstructure for
variadic messages.

func main() {
	r := strings.NewReader(`
		{"type": "login", "user": "joe"}
		{"type": "resize", "height": 220, "width": 400}
	`)
	dec := json.NewDecoder(r)

	for {
		var m map[string]interface{}
		err := dec.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("ERROR:", err)
			break
		}
		if err := handleMessage(m); err != nil {
			fmt.Println("ERROR:", err)
		}
	}
}

func handleMessage(m map[string]interface{}) error {
	typ, ok := m["type"].(string)
	if !ok {
		return fmt.Errorf("bad message: %v", m)
	}
	switch typ {
	case "login":
		var msg struct {
			User string
		}
		if err := mapstructure.Decode(m, &msg); err != nil {
			return err
		}
		fmt.Println("USER:", msg)
	case "resize":
		var msg struct {
			Height int
			Width  int
		}
		if err := mapstructure.Decode(m, &msg); err != nil {
			return err
		}
		fmt.Println("RESIZE:", msg)
	}

	return nil
}
*/

// ResizeArgs is a type that holds the argumnet to the resize pty command
type ResizeArgs struct {
	PaneID int    `json:"pane_id"`
	Sx     uint16 `json:"sx"`
	Sy     uint16 `json:"sy"`
}

// CTRLMessage type holds control messages passed over the control channel
type CTRLMessage struct {
	// MT: Why time.Time? It knows how to un/marshal from/to JSON
	// MT: Document is it msec or sec since epoch
	// Time is in msec since EPOCH
	Time      int64       `json:"time"`
	MessageId int         `json:"message_id"`
	Type      string      `json:"type"`
	Args      interface{} `json:"args"`
}

// IsAuthorized checks whether a client token is authorized
func IsAuthorized(token string) bool {
	tokens, err := ReadAuthorizedTokens()
	if err != nil {
		Logger.Error(err)
		return false
	}
	for _, at := range tokens {
		if token == at {
			return true
		}
	}
	return false
}

// parseWinsize gets a string in the format of "24x80" and returns a Winsize
func ParseWinsize(s string) (*pty.Winsize, error) {
	var sx, sy uint16
	Logger.Infof("Parsing window size: %q", s)
	if _, err := fmt.Sscanf(s, "%dx%d", &sy, &sx); err != nil {
		return nil, fmt.Errorf("Failed to parse terminal dimensions: %s", err)
	}
	return &pty.Winsize{Rows: uint16(sy), Cols: uint16(sx)}, nil
}
