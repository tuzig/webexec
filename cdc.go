// This files holds the structure and utility functions used by the
// Command & Control data channel (aka cdc

package main

import (
	"fmt"
	"github.com/creack/pty"
	"strconv"
	"strings"
)

// ErrorArgs is a type that holds the args for an error message
type ErrorArgs struct {
	// Desc hold the textual desciption of the error
	Desc string `json:"description"`
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
	Ref  int    `json:"ref"`
	Body string `json:"body"`
}

// ResizePTYArgs is a type that holds the argumnet to the resize pty command
type ResizePTYArgs struct {
	// The ChannelID is a sequence number that starts with 1
	ChannelId int    `json:"channel_id"`
	Sx        uint16 `json:"sx"`
	Sy        uint16 `json:"sy"`
}

// CTRLMessage type holds control messages passed over the control channel
type CTRLMessage struct {
	Time      int64          `json:"time"`
	MessageId int            `json:"message_id"`
	Ack       *AckArgs       `json:"ack"`
	ResizePTY *ResizePTYArgs `json:"resize_pty"`
	Auth      *AuthArgs      `json:"auth"`
	Error     *ErrorArgs     `json:"error"`
}

// parseWinsize gets a string in the format of "24x80" and returns a Winsize
func ParseWinsize(s string) (*pty.Winsize, error) {
	var sy int64
	var sx int64
	var err error
	Logger.Infof("Parsing window size: %q", s)
	dim := strings.Split(s, "x")
	sx, err = strconv.ParseInt(dim[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse number of rows: %v", err)

	}
	sy, err = strconv.ParseInt(dim[0], 0, 16)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse number of cols: %v", err)
	}
	return &pty.Winsize{uint16(sy), uint16(sx), 0, 0}, nil
}
