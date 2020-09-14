// Package server holds the code that runs a webrtc based service
// connecting commands with datachannels thru a pseudo tty.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
	"github.com/tuzig/webexec/signal"
)

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 15 * time.Minute
const peerBufferSize = 5000

// Command type hold an executed command, it's pty and buffer
type Pane struct {
	Id int
	// C holds the exectuted command
	C      *exec.Cmd
	Tty    *os.File
	Buffer [][]byte
	dcs    []*webrtc.DataChannel
}

// type Peer is used to remember a client aka peer connection
type Peer struct {
	Id                int
	Authenticated     bool
	State             string
	Remote            string
	LastContact       *time.Time
	LastMsgId         int
	pc                *webrtc.PeerConnection
	Answer            []byte
	cdc               *webrtc.DataChannel
	PendingChannelReq chan *webrtc.DataChannel
}

var Peers []Peer
var Panes []Pane

// start a system command over a pty. If the command contains a dimension
// in the format of 24x80 the login shell is lunched
func (peer *Peer) OnChannelReq(d *webrtc.DataChannel) {
	// the singalig channel is used for test setup
	if d.Label() == "signaling" {
		return
	}
	peer.log.Infof("Got a channel request: peer authenticate: %v, channel label %q",
		peer.Authenticated, d.Label())
	// let the command channel through as without it we can't authenticate
	if d.Label() != "%" && !peer.Authenticated {
		peer.log.Infof(
			"Bufferinga a channel request from an unauthenticated peer: %q",
			d.Label())
		peer.PendingChannelReq <- d
		return
	}

	d.OnOpen(func() {
		pane := peer.OnPaneReq(d)
		if pane != nil {
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				p := msg.Data
				Logger.Infof("> %q", p)
				l, err := pane.Tty.Write(p)
				if err != nil {
					Logger.Warnf("pty of %d write failed: %v",
						pane.Id, err)
				}
				if l != len(p) {
					Logger.Warnf("pty of %d wrote %d instead of %d bytes",
						pane.Id, l, len(p))
				}
			})
			d.OnClose(func() {
				pane.Kill()
				// TODO: do I need to free the pane memory?
				Logger.Infof("Data channel closed : %q", label)
			})
		}
	})
}

// Peer.NewPane opens a new pane and start its command and pty
func (peer *Peer) NewPane(command string, ws *pty.Winsize) (*Pane, error) {
	var err error
	var tty *os.File
	var pane *Pane
	pId := len(Panes)
	cmd := exec.Command(command)
	if ws.Rows != 0 {
		tty, err = pty.StartWithSize(cmd, ws)
	} else {
		err = cmd.Start()
	}
	if err != nil {
		return nil, fmt.Errorf("Failed launching %q: %q", command, err)
	}
	// TODO: protect from reentrancy
	pane = &Pane{pId, cmd, tty, nil, []*webrtc.DataChannel{}}
	Panes = append(Panes, *pane)
	go func() {
		cmd.Wait()
		tty.Close()
	}()
	// NewCommand is up to here
	Logger.Infof("Added a command: id %d tty - %q", pId, tty.Name())
	go pane.ReadLoop()
	s := strconv.Itoa(pId)
	bs := []byte(s)
	Logger.Infof("Added a command: id %d tty - %q id - %q", pId, tty.Name(), bs)
	// TODO: send the channel in a control message
	// d.Send(bs)
	return pane, nil
}

// OnPaneReqs gets a data channel request and creates the pane
// The function parses the label to figure out what it needs to exec:
//   the command to run and rows & cols of the pseudo tty.
// returns a nil when it fails to parse the channel name or when the name is
// '%' used for command & control channel.
//
// label examples:
//      simple form with no pty: "cat filename.txt"
//		to start bash: "24x80 bash"
//		to reconnect to pane id 123: "24x80 >123"
func (peer *Peer) OnPaneReq(d *webrtc.DataChannel) *Pane {
	var err error
	var id int
	var sep int
	var pane *Pane
	var ws pty.Winsize
	// If the message starts with a digit we assume it starts with a size
	l := d.Label()
	// "%" is the command & control channel - aka cdc
	if l[0] == '%' {
		//TODO: if there's an older cdc close it
		Logger.Info("Got a request to open for a new control channel")
		peer.cdc = d
		d.OnMessage(peer.OnCTRLMsg)
		return nil
	}
	Logger.Infof("Got a new data channel: %q\n", l)
	// if the label starts witha digit, it needs a pty
	if unicode.IsDigit(rune(l[0])) {
		sep = strings.IndexRune(l, ' ')
		// no command, use default shell
		if sep == -1 {
			Logger.Errorf("Got an invalid channlel label: %q", l)
			return nil
		}
		ws, err = parseWinsize(l[:sep])
	}
	// If it's a reconnect, parse the id and connnect to the pane
	if rune(l[sep+1]) == '>' {
		id, err = strconv.Atoi(l[sep+2:])
		if err != nil {
			Logger.Errorf("Got an error converting incoming reconnect channel : %q", l[sep+1:])
			return nil
		}
		if id > len(Panes) {
			Logger.Errorf("Got a bad channelId: %d", id)
			return nil
		}
		pane = &Panes[id-1]
		pane.dcs = append(pane.dcs, d)
		return pane
	}
	if err != nil {
		Logger.Errorf("Got an error parsing window size: %q", l)
	}
	// TODO: get the default exec  the users shell or the command from the channel's name
	pane, err = peer.NewPane("zsh", &ws)
	return pane
}

func (pane *Pane) ReadLoop() {
	var i int
	b := make([]byte, 4096)
	for pane.C.ProcessState.String() != "killed" {
		l, err := pane.Tty.Read(b)
		if err != nil && err != io.EOF {
			log.Printf("Failed reading command output: %v", err)
			break
		}
		for i = 0; i < len(pane.dcs); i++ {
			dc := pane.dcs[i]
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				log.Printf("> %d: %s", l, b[:l])
				err = dc.Send(b[:l])
				if err != nil {
					log.Printf("got an error when sending message: %v", err)
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
	for i = 0; i < len(pane.dcs); i++ {
		dc := pane.dcs[i]
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			fmt.Printf("Closing data channel: %q", dc.Label())
			dc.Close()
		}
	}
}
func (pane *Pane) Kill() {
	if pane.C.ProcessState.String() != "killed" {
		err := pane.C.Process.Kill()
		if err != nil {
			log.Printf("Failed to kill process: %v %v",
				err, pane.C.ProcessState.String())
		}
	}
}

// Listen opens a peer connection and starts listening for the offer
func Listen(remote string) *Peer {
	// TODO: protected the next two line from re entrancy
	peer := Peer{
		Id:                len(Peers),
		Authenticated:     false,
		State:             "connected",
		Remote:            remote,
		LastContact:       nil,
		LastMsgId:         0,
		pc:                nil,
		Answer:            nil,
		cdc:               nil,
		PendingChannelReq: make(chan *webrtc.DataChannel, 5),
	}
	Peers = append(Peers, peer)

	// Create a new webrtc API with a custom Logger
	// This SettingEngine allows non-standard WebRTC behavior
	s := webrtc.SettingEngine{}
	s.SetConnectionTimeout(connectionTimeout, keepAliveInterval)
	//TODO: call func (e *SettingEngine) SetEphemeralUDPPortRange(portMin, portMax uint16)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		err = fmt.Errorf("Failed to open peer connection: %q", err)
		return nil
	}
	// Handling status changes will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		s := connectionState.String()
		// Logger.Infof("ICE Connection State change: %s", s)
		if s == "connected" {
			// TODO add initialization code
		}
	})
	// testing uses special signaling, so there's no remote information
	if len(remote) > 0 {
		offer := webrtc.SessionDescription{}
		signal.Decode(remote, &offer)
		err = pc.SetRemoteDescription(offer)
		if err != nil {
			panic(err)
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			panic(err)
		}
		// Sets the LocalDescription, and starts listning for UDP packets
		err = pc.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}
		peer.Answer = []byte(signal.Encode(answer))
	}
	pc.OnDataChannel(peer.OnChannelReq)
	peer.pc = pc
	return &peer
}

// Shutdown is called when it's time to go.
// Sweet dreams.
func Shutdown() {
	for _, peer := range Peers {
		if peer.pc != nil {
			peer.pc.Close()
		}
	}
	for _, p := range Panes {
		p.C.Process.Kill()
	}
}

// Authenticate checks authorization args against system's user
// returns the user's token or nil if failed to authenticat
func (peer *Peer) Authenticate(args *AuthArgs) string {

	t := "atoken"
	goto HappyEnd
HappyEnd:
	peer.Username = args.Username
	return t

}

// SendAck sends an ack for a given control message
func (peer *Peer) SendAck(cm CTRLMessage, body string) {
	args := AckArgs{Ref: cm.MessageId, Body: body}
	// TODO: protect message counter against reentrancy
	msg := CTRLMessage{time.Now().UnixNano() / 1000000, peer.LastMsgId + 1, &args,
		nil, nil, nil}
	peer.LastMsgId += 1
	msgJ, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal the ack msg: %e", err)
	}
	log.Printf("Sending ack: %q", msgJ)
	peer.cdc.Send(msgJ)
}

// OnCTRLMsg handles incoming control messages
func (peer *Peer) OnCTRLMsg(msg webrtc.DataChannelMessage) {
	var m CTRLMessage
	log.Printf("Got a CTRLMessage: %q\n", string(msg.Data))
	err := json.Unmarshal(msg.Data, &m)
	if err != nil {
		log.Printf("Failed to parse incoming control message: %v", err)
		return
	}
	if m.ResizePTY != nil {
		var ws pty.Winsize
		cId := m.ResizePTY.ChannelId
		if cId == 0 {
			log.Printf("Error: Got a resize message with no channel Id")
			return
		}
		cmd := Panes[cId-1]
		ws.Cols = m.ResizePTY.Sx
		ws.Rows = m.ResizePTY.Sy
		log.Printf("Changing pty size for channel %v: %v", cmd, ws)
		pty.Setsize(cmd.Tty, &ws)
		peer.SendAck(m, "")
	} else if m.Auth != nil {
		// TODO:
		// token := Authenticate(m.Auth)
		token := "Always autehnticated"
		if token != "" {
			peer.Authenticated = true
			peer.Username = m.Auth.Username
			peer.SendAck(m, token)
			// handle pending channel requests
			handlePendingChannelRequests := func() {
				for d := range peer.PendingChannelReq {
					log.Printf("Handling pennding channel Req: %q", d.Label())
					peer.OnChannelReq(d)
				}
			}
			go handlePendingChannelRequests()
		} else {
			log.Printf("Authentication failed for %v", peer)
		}
	}
	// TODO: add more commands here: mouse, clipboard, etc.
}

// ErrorArgs is a type that holds the args for an error message
type ErrorArgs struct {
	// Desc hold the textual desciption of the error
	Desc string `json:"description"`
	// Ref holds the message id the error refers to or 0 for system errors
	Ref int `json:"ref"`
}

// AuthArgs is a type that holds client's authentication arguments.
type AuthArgs struct {
	Username string `json:"username"`
	Secret   string `json:"secret"`
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
func parseWinsize(s string) (ws pty.Winsize, err error) {
	dim := strings.Split(s, "x")
	sx, err := strconv.ParseInt(dim[1], 0, 16)
	ws = pty.Winsize{0, 0, 0, 0}
	if err != nil {
		return ws, fmt.Errorf("Failed to parse number of cols: %v", err)
	}
	sy, err := strconv.ParseInt(dim[0], 0, 16)
	if err != nil {
		return ws, fmt.Errorf("Failed to parse number of rows: %v", err)
	}
	ws = pty.Winsize{uint16(sy), uint16(sx), 0, 0}
	return
}
