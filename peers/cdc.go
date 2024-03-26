// This files holds the structure and utility functions used by the
// Command & Control data channel (aka cdc

package peers

import (
	"encoding/json"
	"fmt"

	"github.com/creack/pty"
)

// NAckArgs is a type that holds the args for an error message
type NAckArgs struct {
	// Desc hold the textual desciption of the error
	Desc string `json:"desc"`
	// Ref holds the message id the error refers to or 0 for system errors
	Ref int `json:"ref"`
}

// RestoreArgs is a type that holds client's authentication arguments.
type RestoreArgs struct {
	Marker int `json:"marker"`
}

// AckArgs is a type to hold the args for an Ack message
type AckArgs struct {
	// Ref holds the message id the error refers to or 0 for system errors
	Ref  int    `json:"ref"`
	Body string `json:"body,omitempty"`
}

// SetPayloadArgs is a type to hold the args for a set_payload type of a message
type SetPayloadArgs struct {
	// Ref holds the message id the error refers to or 0 for system errors
	Payload json.RawMessage `json:"payload"`
}

// ResizeArgs is a type that holds the argumnet to the resize pty command
// ResizeArgs is a type that holds the argumnet to the resize pty command
type ResizeArgs struct {
	PaneID int    `json:"pane_id"`
	Sx     uint16 `json:"sx"`
	Sy     uint16 `json:"sy"`
}

type AddPaneArgs struct {
	Command []string `json:"command"`
	Rows    uint16   `json:"rows, omitempty"`
	Cols    uint16   `json:"cols, omitempty"`
	X       uint16   `json:"x, omitempty"`
	Y       uint16   `json:"y, omitempty"`
	Parent  int      `json:"parent,omitempty"`
}

type ReconnectPaneArgs struct {
	ID int `json:"id"`
}

// CTRLMessage type holds control messages passed over the control channel
type CTRLMessage struct {
	// Time is in msec since EPOCH
	Time int64       `json:"time"`
	Ref  int         `json:"message_id"`
	Type string      `json:"type"`
	Args interface{} `json:"args"` // TODO: add omitempty
}

// ParseWinsize gets a string in the format of "24x80" and returns a Winsize
func ParseWinsize(s string) (*pty.Winsize, error) {
	var sx, sy uint16
	if _, err := fmt.Sscanf(s, "%dx%d", &sy, &sx); err != nil {
		return nil, fmt.Errorf("Failed to parse terminal dimensions: %s", err)
	}
	return &pty.Winsize{Rows: uint16(sy), Cols: uint16(sx)}, nil
}
