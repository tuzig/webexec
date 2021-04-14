// This file holds the struct and code to communicate with remote peers
// over webrtc data channels.
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v3"
)

const keepAliveInterval = 2 * time.Second

var (
	// Peers holds all the peers (connected and disconnected)
	Peers map[string]*Peer
	// Payload holds the client's payload
	Payload []byte
	// WebRTCAPI is the gateway to webrtc calls
	WebRTCAPI *webrtc.API
	// the id of the last marker used
	lastMarker = 0
	markerM    sync.RWMutex
	webrtcAPIM sync.Mutex
	peersM     sync.Mutex
)

// Peer is a type used to remember a client.
type Peer struct {
	FP          string
	Remote      string
	Token       string
	LastContact *time.Time
	LastRef     int
	PC          *webrtc.PeerConnection
	cdc         *webrtc.DataChannel
	Marker      int
}

// NewPeer funcions starts listening to incoming peer connection from a remote
func NewPeer(fingerprint string) (*Peer, error) {
	webrtcAPIM.Lock()
	if WebRTCAPI == nil {
		var s webrtc.SettingEngine
		if pionLoggerFactory != nil {
			s = webrtc.SettingEngine{LoggerFactory: pionLoggerFactory}
			if Conf.portMax > 0 {
				s.SetEphemeralUDPPortRange(Conf.portMin, Conf.portMax)
			}
		} else {
			// for testing
			s = webrtc.SettingEngine{}
		}
		s.SetICETimeouts(
			Conf.disconnectTimeout, Conf.failedTimeout, Conf.keepAliveInterval)
		WebRTCAPI = webrtc.NewAPI(webrtc.WithSettingEngine(s))
	}
	webrtcAPIM.Unlock()
	certs, err := GetCerts()
	if err != nil {
		return nil, fmt.Errorf("failed to get certificates: %w", err)
	}
	config := webrtc.Configuration{
		PeerIdentity: "webexec",
		ICEServers:   []webrtc.ICEServer{{URLs: Conf.iceServers}},
		Certificates: certs,
	}
	pc, err := WebRTCAPI.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("NewPeerConnection failed")
	}
	peer := Peer{
		FP:          fingerprint,
		Token:       "",
		LastContact: nil,
		LastRef:     0,
		PC:          pc,
		Marker:      -1,
	}
	peersM.Lock()
	if Peers == nil {
		Peers = make(map[string]*Peer)
	}
	Peers[fingerprint] = &peer
	peersM.Unlock()
	// Status changes happend when the peer has connected/disconnected
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		Logger.Infof("WebRTC Connection State change: %s", state.String())
		if state == webrtc.PeerConnectionStateFailed {
			peer.PC.Close()
			peer.PC = nil
		}
	})
	pc.OnDataChannel(peer.OnChannelReq)
	return &peer, nil
}

// Listen get's a client offer, starts listens to it and returns an answear
func (peer *Peer) Listen(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	Logger.Infof("Listening to: %v", offer)
	err := peer.PC.SetRemoteDescription(offer)
	if err != nil {
		return nil, fmt.Errorf("Failed to set remote description: %s", err)
	}
	answer, err := peer.PC.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	// Sets the LocalDescription, and starts listning for UDP packets
	// Create channel that is blocked until ICE Gathering is complete
	// TODO: remove this and erplace with ICE trickle
	gatherComplete := webrtc.GatheringCompletePromise(peer.PC)
	err = peer.PC.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}
	select {
	case <-time.After(Conf.gatheringTimeout):
		return nil, fmt.Errorf("timed out waiting to finish gathering ICE candidates")
	case <-gatherComplete:
	}
	return peer.PC.LocalDescription(), nil
}

// OnChannelReq starts a system command over a pty.
// If the data channel label includes a dimention
// in the format of 24x80 the login shell is lunched
func (peer *Peer) OnChannelReq(d *webrtc.DataChannel) {
	// the singalig channel is used for test setup
	if d.Label() == "signaling" {
		return
	}
	label := d.Label()
	Logger.Infof("Got a channel request: channel label %q", label)

	d.OnOpen(func() {
		pane, err := peer.GetOrCreatePane(d)
		if err != nil {
			msg := fmt.Sprintf("Failed to get or create pane for dc %q: %s",
				d.Label(), err)
			d.Send([]byte(msg))
			Logger.Errorf(msg)
		}
		if pane != nil {
			c := cdb.Add(d, pane, peer)
			d.OnMessage(pane.OnMessage)
			d.OnClose(func() {
				cdb.Delete(c)
			})
		}
	})
}

// GetOrCreatePane gets a data channel and creates an associated pane
// The function parses the label to figure out what it needs to exec:
//   the command to run and rows & cols of the pseudo tty.
// returns a nil when it fails to parse the channel name or when the name is
// '%' used for command & control channel.
//
// label examples:
//      simple form with no pty: `echo,Hello world`
//		to start bash: `24x80,bash`
//		to reconnect to pane id 123: `>123`
func (peer *Peer) GetOrCreatePane(d *webrtc.DataChannel) (*Pane, error) {
	var (
		err      error
		cmdIndex int
		pane     *Pane
		ws       *pty.Winsize
	)

	// If the message starts with a digit we assume it starts with a size
	// i.e. "24x80,echo,Hello World"
	l := d.Label()
	fields := strings.Split(l, ",")
	// "%" is the command & control channel - aka cdc
	if l[0] == '%' {
		//TODO: if there's an older cdc close it
		Logger.Info("Got a request to open a control channel")
		peer.cdc = d
		d.OnMessage(peer.OnCTRLMsg)
		return nil, nil
	}
	// if the label starts witha digit, i.e. "80x24" it needs a pty
	if unicode.IsDigit(rune(l[0])) {
		cmdIndex = 1
		// no command, don't create the pane
		if cmdIndex > len(fields)-1 {
			return nil, fmt.Errorf("Got an invalid pane label: %q", l)
		}
		ws, err = ParseWinsize(fields[0])
		if err != nil {
			return nil, fmt.Errorf("Failed to parse winsize: %v", err)
		}
	}
	if len(fields[cmdIndex]) < 2 {
		return nil, fmt.Errorf("Command is too short")
	}

	// If it's a reconnect, parse the id and reconnnect to the pane
	if rune(fields[cmdIndex][0]) == '>' {
		id, err := strconv.Atoi(fields[cmdIndex][1:])
		if err != nil {
			return nil, fmt.Errorf("Got an error converting incoming reconnect id : %q",
				fields[cmdIndex])
		}
		Logger.Infof("Got a reconnect request to pane %d", id)
		return peer.Reconnect(d, id)
	}
	pane, err = NewPane(fields[cmdIndex:], d, peer, ws)
	if pane != nil {
		pane.sendFirstMessage(d)
		go pane.ReadLoop()
		return pane, nil
	}

	return nil, fmt.Errorf("Failed to create new pane: %q", err)
}

// Reconnect reconnects to a pane and restore the screen/buffer
// buffer from that marker if not we use our headless terminal emulator to
// send over the current screen.
func (peer *Peer) Reconnect(d *webrtc.DataChannel, id int) (*Pane, error) {
	pane := Panes.Get(id)
	if pane == nil {
		return nil, fmt.Errorf("Got a bad pane id: %d", id)
	}
	if pane.IsRunning {
		pane.sendFirstMessage(d)
		pane.Restore(d, peer.Marker)
		return pane, nil
	}
	d.Close()
	return nil, fmt.Errorf("Can not reconnect as pane is not running")
}

// SendAck sends an ack for a given control message
func (peer *Peer) SendAck(cm CTRLMessage, body []byte) error {
	args := AckArgs{Ref: cm.Ref, Body: body}
	return SendCTRLMsg(peer, "ack", &args)
}

// SendNAck sends an nack for a given control message
func (peer *Peer) SendNAck(cm CTRLMessage, desc string) error {
	args := NAckArgs{Ref: cm.Ref, Desc: desc}
	return SendCTRLMsg(peer, "nack", &args)
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
		cID := resizeArgs.PaneID
		pane := Panes.Get(cID)
		if pane == nil {
			Logger.Error("Failed to parse resize message pane_id out of range")
			return
		}
		var ws pty.Winsize
		ws.Cols = resizeArgs.Sx
		ws.Rows = resizeArgs.Sy
		pane.Resize(&ws)
		err = peer.SendAck(m, nil)
		if err != nil {
			Logger.Errorf("#%d: Failed to send a resize ack: %v", peer.FP, err)
			return
		}
	case "restore":
		var args RestoreArgs
		err = json.Unmarshal(raw, &args)
		peer.Marker = args.Marker
		err = peer.SendAck(m, Payload)
	case "get_payload":
		err = peer.SendAck(m, Payload)
	case "set_payload":
		var payloadArgs SetPayloadArgs
		err = json.Unmarshal(raw, &payloadArgs)
		Logger.Infof("Setting payload to: %s", payloadArgs.Payload)
		Payload = payloadArgs.Payload
		err = peer.SendAck(m, Payload)
	case "mark":
		// acdb a marker and store it in each pane
		markerM.Lock()
		lastMarker++
		peer.Marker = lastMarker
		markerM.Unlock()
		for _, dc := range cdb.All4Peer(peer) {
			// this will usually fail, but delete what's needed
			dc.pane.Buffer.Mark(peer.Marker)
			cdb.Delete(dc)
		}
		err = peer.SendAck(m, []byte(fmt.Sprintf("%d", peer.Marker)))
	default:
		Logger.Errorf("Got a control message with unknown type: %q", m.Type)
	}
	if err != nil {
		Logger.Errorf("#%d: Failed to send [n]ack: %v", peer.FP, err)
		return
	}
}
