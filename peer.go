// This file holds the struct and code to communicate with remote peers
// over webrtc data channels.
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
)

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 15 * time.Minute
const peerBufferSize = 5000

// Peers holds all the peers (connected and disconnected)
var Peers []Peer

// Payload holds the client's payload
var Payload []byte

// WebRTCAPI is the gateway to webrtc calls
var WebRTCAPI *webrtc.API

// type Peer is used to remember a client aka peer connection
type Peer struct {
	Id                int
	Authenticated     bool
	State             string
	Remote            string
	Token             string
	LastContact       *time.Time
	LastMsgId         int
	PC                *webrtc.PeerConnection
	Answer            string
	cdc               *webrtc.DataChannel
	PendingChannelReq chan *webrtc.DataChannel
}

// NewPeer funcions starts listening to incoming peer connection from a remote
func NewPeer(remote string) (*Peer, error) {
	Logger.Infof("New Peer from: %s", remote)
	if WebRTCAPI == nil {
		s := webrtc.SettingEngine{}
		s.SetConnectionTimeout(connectionTimeout, keepAliveInterval)
		WebRTCAPI = webrtc.NewAPI(webrtc.WithSettingEngine(s))
	}
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	pc, err := WebRTCAPI.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("NewPeerConnection failed")
	}
	// TODO: protected the next two commands from reentrancy
	peer := Peer{
		Id:                len(Peers),
		Token:             "",
		Authenticated:     false,
		State:             "connected",
		LastContact:       nil,
		LastMsgId:         0,
		PC:                pc,
		Answer:            "",
		cdc:               nil,
		PendingChannelReq: make(chan *webrtc.DataChannel, 5),
	}
	Peers = append(Peers, peer)
	if err != nil {
		return nil, fmt.Errorf("Failed to open peer connection: %q", err)
	}
	// Status changes happend when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		s := connectionState.String()
		Logger.Infof("ICE Connection State change: %s", s)
		if s == "connected" {
			// TODO add initialization code
		}
		if s == "failed" {
			// TODO remove the peer from Peers
		}
	})
	// testing uses special signaling, so there's no remote information
	if len(remote) > 0 {
		err := peer.Listen(remote)
		if err != nil {
			return nil, fmt.Errorf("#%d: Failed to listen: %s", peer.Id, err)
		}
	} else {
		Logger.Error("Got a connect request with empty an offer")
	}
	pc.OnDataChannel(peer.OnChannelReq)
	return &peer, nil
}

// peer.Listen get's a client offer, starts listens to it and return its offer
func (peer *Peer) Listen(remote string) error {
	offer := webrtc.SessionDescription{}
	err := DecodeOffer(remote, &offer)
	if err != nil {
		return fmt.Errorf("Failed to decode offer: %s", err)
	}
	Logger.Infof("Listening to: %q\n%v", remote, offer)
	err = peer.PC.SetRemoteDescription(offer)
	if err != nil {
		return fmt.Errorf("Failed to set remote description: %s", err)
	}
	answer, err := peer.PC.CreateAnswer(nil)
	if err != nil {
		return err
	}
	// Sets the LocalDescription, and starts listning for UDP packets
	err = peer.PC.SetLocalDescription(answer)
	if err != nil {
		return err
	}
	peer.Answer = EncodeOffer(answer)
	Logger.Infof("Got an answer %q into %q", answer, peer.Answer)
	return nil
}

// start a system command over a pty. If the command contains a dimension
// in the format of 24x80 the login shell is lunched
func (peer *Peer) OnChannelReq(d *webrtc.DataChannel) {
	// the singalig channel is used for test setup
	if d.Label() == "signaling" {
		return
	}
	authenticated := peer.Authenticated
	label := d.Label()
	Logger.Infof("Got a channel request: peer authenticate: %v, channel label %q",
		authenticated, label)

	// let the command channel through as without it we can't authenticate
	// TODO: check if we need to track pending pane requests
	if label != "%" && !authenticated {
		Logger.Infof(
			"Bufferinga a channel request from an unauthenticated peer: %q",
			label)
		peer.PendingChannelReq <- d
		return
	}

	d.OnOpen(func() {
		pane := peer.OnPaneReq(d)
		if pane != nil {
			d.OnMessage(pane.OnMessage)
			d.OnClose(pane.OnCloseDC)
		}
	})
}

// Peer.NewPane opens a new pane and start its command and pty

// OnPaneReqs gets a data channel request and creates the pane
// The function parses the label to figure out what it needs to exec:
//   the command to run and rows & cols of the pseudo tty.
// returns a nil when it fails to parse the channel name or when the name is
// '%' used for command & control channel.
//
// label examples:
//      simple form with no pty: echo,"Hello world"
//		to start bash: "24x80,bash"
//		to reconnect to pane id 123: "24x80,>123"
func (peer *Peer) OnPaneReq(d *webrtc.DataChannel) *Pane {
	var (
		err      error
		cmdIndex int
		pane     *Pane
		ws       *pty.Winsize
	)

	// If the message starts with a digit we assume it starts with a size
	l := d.Label()
	fields := strings.Split(l, ",")
	// "%" is the command & control channel - aka cdc
	if l[0] == '%' {
		//TODO: if there's an older cdc close it
		Logger.Info("Got a request to open for a new control channel")
		peer.cdc = d
		d.OnMessage(peer.OnCTRLMsg)
		return nil
	}
	// if the label starts witha digit, it needs a pty
	if unicode.IsDigit(rune(l[0])) {
		cmdIndex = 1
		// no command, use default shell
		if cmdIndex > len(fields)-1 {
			Logger.Errorf("Got an invalid channlel label: %q", l)
			return nil
		}
		ws, err = ParseWinsize(fields[0])
		if err != nil {
			Logger.Errorf("Failed to parse winsize: %v", err)
		}
	}
	// If it's a reconnect, parse the id and connnect to the pane
	if rune(fields[cmdIndex][0]) == '>' {
		id, err := strconv.Atoi(fields[cmdIndex][1:])
		if err != nil {
			Logger.Errorf("Got an error converting incoming reconnect channel : %q", fields[cmdIndex])
			return nil
		}
		Logger.Infof("New channel is a reconnect request to %d", id)
		if id > len(Panes) {
			Logger.Errorf("Got a bad pane id: %d", id)
			return nil
		}
		pane = &Panes[id-1]
		pane.Resize(ws)
		pane.dcs = append(pane.dcs, d)
		Logger.Infof("Added dc to pane %v for a total of %d", pane, len(pane.dcs))
		pane.SendId(d)
		return pane
	}
	if err != nil {
		Logger.Errorf("Got an error parsing window size: %q", err)
	}
	// TODO: get the default exec  the users shell or the command from the channel's name
	pane, err = NewPane(fields[cmdIndex:], d, ws)
	if pane != nil {
		// Send the pane id as the first message
		pane.SendId(d)
		go pane.ReadLoop()
		return pane
	} else {
		Logger.Error("Failed to create new pane: %q", err)
		return nil
	}
}

// Authenticate checks authorization args against system's user
// returns the user's token or nil if failed to authenticat
func (peer *Peer) Authenticate(args *AuthArgs) string {
	t := "atoken"
	peer.Token = args.Token
	return t

}

// SendAck sends an ack for a given control message
func (peer *Peer) SendAck(cm CTRLMessage, body []byte) {
	args := AckArgs{Ref: cm.MessageId, Body: body}

	// TODO: protect message counter against reentrancy
	msg := CTRLMessage{time.Now().UnixNano() / 1000000, peer.LastMsgId + 1,
		"ack", &args}
	peer.LastMsgId += 1
	msgJ, err := json.Marshal(msg)
	if err != nil {
		Logger.Errorf("Failed to marshal the ack msg: %e\n   msg == %q", err, msg)
		return
	}
	Logger.Infof("Sending ack: %q", msgJ)
	peer.cdc.Send(msgJ)
}

// OnCTRLMsg handles incoming control messages
func (peer *Peer) OnCTRLMsg(msg webrtc.DataChannelMessage) {
	var raw json.RawMessage
	m := CTRLMessage{
		Args: &raw,
	}
	Logger.Infof("Got a CTRLMessage: %q\n", string(msg.Data))
	err := json.Unmarshal(msg.Data, &m)
	if err != nil {
		Logger.Infof("Failed to parse incoming control message: %v", err)
		return
	}
	switch m.Type {
	case "resize":
		var resizeArgs ResizeArgs
		err = json.Unmarshal(raw, &resizeArgs)
		if err != nil {
			Logger.Infof("Failed to parse incoming control message: %v", err)
			return
		}
		cId := resizeArgs.PaneID
		pane := Panes[cId-1]
		var ws pty.Winsize
		ws.Cols = resizeArgs.Sx
		ws.Rows = resizeArgs.Sy
		pane.Resize(&ws)
		peer.SendAck(m, nil)
	case "auth":
		// TODO:
		// token := Authenticate(m.Auth)
		var authArgs AuthArgs
		token := "Always autehnticated"
		if token != "" {
			peer.Authenticated = true
			err = json.Unmarshal(raw, &authArgs)
			peer.Token = authArgs.Token
			peer.SendAck(m, Payload)
			// handle pending channel requests
			handlePendingChannelRequests := func() {
				for d := range peer.PendingChannelReq {
					Logger.Infof("Handling pennding channel Req: %q", d.Label())
					peer.OnChannelReq(d)
				}
			}
			go handlePendingChannelRequests()
		} else {
			Logger.Infof("Authentication failed for %v", peer)
		}
	case "get_payload":
		peer.SendAck(m, Payload)
	case "set_payload":
		var payloadArgs SetPayloadArgs
		err = json.Unmarshal(raw, &payloadArgs)
		Logger.Infof("Setting payload to: %q", payloadArgs.Payload)
		Payload = payloadArgs.Payload
		peer.SendAck(m, Payload)
	default:
		Logger.Errorf("Got a control message with unknown type: %q", m.Type)
	}
	// TODO: add more commands here: mouse, clipboard, etc.
}
