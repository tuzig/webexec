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
	"os"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v3"
    "github.com/riywo/loginshell"
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
	FP                string
	Remote            string
	Token             string
	LastContact       *time.Time
	LastRef           int
	PC                *webrtc.PeerConnection
	cdc               *webrtc.DataChannel
	Marker            int
	pendingCandidates chan *webrtc.ICECandidateInit
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
	is, err := getICEServers(Conf.peerbookHost)
	if err != nil {
		return nil, fmt.Errorf("failed to get ice servers: %w", err)
	}
	config := webrtc.Configuration{
		PeerIdentity: "webexec",
		ICEServers:   is,
		Certificates: certs,
	}
	pc, err := WebRTCAPI.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("NewPeerConnection failed")
	}
	peer := Peer{
		FP:                fingerprint,
		Token:             "",
		LastContact:       nil,
		LastRef:           0,
		PC:                pc,
		Marker:            -1,
		pendingCandidates: make(chan *webrtc.ICECandidateInit, 8),
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
			return
		}
		if state == webrtc.PeerConnectionStateConnecting {
			for c := range peer.pendingCandidates {
				err := pc.AddICECandidate(*c)
				if err != nil {
					Logger.Errorf("Failed to add ice candidate: %s", err)
				}
			}
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

// OnChannelReq starts the cdc channel.
// Upon establishing the connection, the client opens this channel with the
// api version he uses
func (peer *Peer) OnChannelReq(d *webrtc.DataChannel) {
	// the singalig channel is used for test setup
	if d.Label() == "signaling" {
		return
	}
	label := d.Label()
	Logger.Infof("Got a channel request: channel label %q", label)
	if label != "%" {
		Logger.Errorf("Closing client with wrong version: %s", label)
	}
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
	pane, err = NewPane(fields[cmdIndex:], peer, ws, 0)
	if err != nil {
		return nil, fmt.Errorf("Failed to create new pane: %q", err)
	}
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
		c := cdb.Add(d, pane, peer)
		d.OnMessage(pane.OnMessage)
		d.OnClose(func() {
			cdb.Delete(c)
		})
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

// SendNack sends an nack for a given control message
func (peer *Peer) SendNack(cm CTRLMessage, desc string) error {
	args := NAckArgs{Ref: cm.Ref, Desc: desc}
	return SendCTRLMsg(peer, "nack", &args)
}

// OnCTRLMsg handles incoming control messages
func (peer *Peer) OnCTRLMsg(msg webrtc.DataChannelMessage) {
	var raw json.RawMessage
	m := CTRLMessage{
		Args: &raw,
	}
	t := true
	dcOpts := &webrtc.DataChannelInit{Ordered: &t}
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
		for _, client := range cdb.All4Peer(peer) {
			client.pane.Buffer.Mark(peer.Marker)
			client.dc.Close()
			// will be removed on close
		}
		err = peer.SendAck(m, []byte(fmt.Sprintf("%d", peer.Marker)))
	case "reconnect_pane":
		var a ReconnectPaneArgs
		err = json.Unmarshal(raw, &a)
		if err != nil {
			Logger.Infof("Failed to parse incoming control message: %v", err)
			return
		}
		Logger.Infof("@%d: got reconnect_pane", a.ID)

		l := fmt.Sprintf("%d:%d", m.Ref, a.ID)
		d, err := peer.PC.CreateDataChannel(l, dcOpts)
		if err != nil {
			Logger.Warnf("Failed to create data channel : %v", err)
			return
		}
		d.OnOpen(func() {
			Logger.Info("open is completed!!!")
			pane, err := peer.Reconnect(d, a.ID)
			if err != nil || pane == nil {
				Logger.Warnf("Failed to reconnect to pane  data channel : %v", err)
				peer.SendNack(m, fmt.Sprintf("Failed to reconnect to: %d", a.ID))
				return
			} else {
				peer.SendAck(m, []byte(fmt.Sprintf("%d", pane.ID)))
			}
		})

	case "add_pane":
		var a AddPaneArgs
		var ws *pty.Winsize
		err = json.Unmarshal(raw, &a)
		if err != nil {
			Logger.Infof("Failed to parse incoming control message: %v", err)
			return
		}
		Logger.Infof("got add_pane: %v", a)
		if a.Rows > 0 && a.Cols > 0 {
			ws = &pty.Winsize{Rows: a.Rows, Cols: a.Cols, X: a.X, Y: a.Y}
		} else {
			ws = &pty.Winsize{Rows: 24, Cols: 80}
			Logger.Warn("Got an add_pane commenad with no rows or cols")
		}

		if a.Command[0] == "*" {
			shell, err := loginshell.Shell()
			if err != nil {
				Logger.Warnf("Failed to determine user's shell: %v", err)
				a.Command[0] = "bash"
			} else {
				Logger.Infof("Using %s for shell", shell)
				a.Command[0] = shell
			}
		}
		dirname, err := os.UserHomeDir()
		if err != nil {
			Logger.Warnf("Failed to determine user's home directory: %v", err)
			dirname = "/"
		}
		cmd := append([]string{"env", fmt.Sprintf("HOME=%s", dirname)}, a.Command...)
		pane, err := NewPane(cmd, peer, ws, a.Parent)
		if err != nil {
			Logger.Warnf("Failed to add a new pane: %v", err)
			return
		}
		l := fmt.Sprintf("%d:%d", m.Ref, pane.ID)
		d, err := peer.PC.CreateDataChannel(l, dcOpts)
		if err != nil {
			msg := fmt.Sprintf("Failed to create data channel : %s", l)
			peer.SendNack(m, msg)
			Logger.Warnf(msg)
			return
		}
		d.OnOpen(func() {
			c := cdb.Add(d, pane, peer)
			Logger.Infof("opened data channel for pane %d", pane.ID)
			peer.SendAck(m, []byte(fmt.Sprintf("%d", pane.ID)))
			d.OnMessage(pane.OnMessage)
			d.OnClose(func() {
				cdb.Delete(c)
			})
		})

	default:
		Logger.Errorf("Got a control message with unknown type: %q", m.Type)
	}
	if err != nil {
		Logger.Errorf("#%d: Failed to send [n]ack: %v", peer.FP, err)
	}
	return
}
