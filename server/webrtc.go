// Package server holds the code that runs a webrtc based service
// connecting commands with datachannels thru a pseudo tty.
package server

/*
#include <shadow.h>
#include <stddef.h>
#include <stdlib.h>
*/
import "C"

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

	"github.com/afittestide/webexec/signal"
	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
	"github.com/tredoe/osutil/user/crypt"
	"github.com/tredoe/osutil/user/crypt/sha512_crypt"
)

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 15 * time.Minute
const peerBufferSize = 5000

// type Command hold an executed command, it's pty and buffer
type Command struct {
	Id int
	// C holds the exectuted command
	C      *exec.Cmd
	Tty    *os.File
	Buffer [][]byte
}

// type WebRTCServer is the singelton we use to store server globals
type WebRTCServer struct {
	// peers holds the connected and disconnected peers
	Peers []Peer
	// channels holds all the open channel we have with process ID as key
	Cmds []Command
}

// Type Channel connects a data channel with a command
// a pseudo tty.
type Channel struct {
	dc  *webrtc.DataChannel
	Cmd *Command
}

// type Peer is used to remember a client aka peer connection
type Peer struct {
	server            *WebRTCServer
	Id                int
	Authenticated     bool
	State             string
	Remote            string
	LastContact       *time.Time
	LastMsgId         int
	pc                *webrtc.PeerConnection
	Answer            []byte
	Channels          []*Channel
	cdc               *webrtc.DataChannel
	Username          string
	PendingChannelReq chan *webrtc.DataChannel
}

func NewWebRTCServer() (server WebRTCServer, err error) {
	return WebRTCServer{nil, nil}, nil
	// Register data channel creation handling
}

// start a system command over a pty
func (peer *Peer) StartCommand(c []string) (*Command, error) {
	var cmd *exec.Cmd
	var tty *os.File
	var err error
	var firstRune rune = rune(c[0][0])
	// If the message starts with a digit we assume it starts with
	// a size
	suArgs := []string{peer.Username, "-c"}
	if unicode.IsDigit(firstRune) {
		ws, err := parseWinsize(c[0])
		if err != nil {
			return nil, fmt.Errorf("Failed to parse winsize: %q ", c[0])
		}

		cmd = exec.Command("su", append(suArgs, c[1:]...)...)
		log.Printf("starting command with size: %v", ws)
		tty, err = pty.StartWithSize(cmd, &ws)
	} else {
		cmd = exec.Command("su", append(suArgs, c[0:]...)...)
		log.Printf("starting command without size %v %v", cmd.Path, cmd.Args)
		tty, err = pty.Start(cmd)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to start command: %v", err)
	}
	// create the channel and add to the server's channels slice
	ret := Command{len(peer.server.Cmds), cmd, tty, nil}
	peer.server.Cmds = append(peer.server.Cmds, ret)
	go func() {
		cmd.Wait()
		tty.Close()
	}()
	return &ret, nil
}

func (peer *Peer) OnChannelReq(d *webrtc.DataChannel) {
	log.Printf("Got a channel request: peer authenticate: %v, channel label %q",
		peer.Authenticated, d.Label())
	// the singalig channel is used for test setup
	if d.Label() == "signaling" {
		return
	}
	// let the command channel through as without it we can't authenticate
	// if it's not that
	if d.Label() != "%" && !peer.Authenticated {
		peer.PendingChannelReq <- d
		/*
			log.Printf("Denying a channel request from an unauthenticated peer: %q",
				d.Label())
			d.Close()
		*/
		return
	}

	d.OnOpen(func() {
		var err error
		l := d.Label()
		log.Printf("New Data channel %q\n", l)
		c := strings.Split(l, " ")
		// We get "terminal7" in c[0] as the first channel name
		// from a fresh client. This dc is used for as ctrl channel
		if c[0] == "%" {
			//TODO: if there's an older cdc close it
			log.Println("Registering a control channel")
			peer.cdc = d
			d.OnMessage(peer.OnCTRLMsg)
			// handle pending channel requests
			for r := range peer.PendingChannelReq {
				peer.OnChannelReq(r)
			}
			return
		}
		cmd, err := peer.StartCommand(c)
		if err != nil {
			log.Printf("Failed to start command: %v", err)
			return
		}
		channel := Channel{d, cmd}
		// TODO: protect from reentrancy
		channelId := len(peer.Channels)
		peer.Channels = append(peer.Channels, &channel)
		log.Printf("Added a channel: id %d, num of channels: %d",
			channelId, len(peer.Channels))
		d.OnMessage(channel.OnMessage)
		// send the channel id as the first message
		s := strconv.Itoa(channelId)
		bs := []byte(s)
		channel.Write(bs)
		// use copy to read command output and send it to the data channel
		io.Copy(&channel, cmd.Tty)

		if err != nil {
			log.Printf("Failed to kill process: %v %v",
				err, cmd.C.ProcessState.String())
		}
		d.Close()
		d.OnClose(func() {
			cmd.Kill()
			log.Println("Data channel closed")
		})
	})
}

func (cmd *Command) Kill() {
	if cmd.C.ProcessState.String() != "killed" {
		err := cmd.C.Process.Kill()
		if err != nil {
			log.Printf("Failed to kill process: %v %v",
				err, cmd.C.ProcessState.String())
		}
	}
}

// Listen opens a peer connection and starts listening for the offer
func (server *WebRTCServer) Listen(remote string) *Peer {
	// TODO: protected the next two line from re entrancy
	peer := Peer{server, len(server.Peers), false,
		"connected", remote, nil, 0, nil, nil, nil, nil, "",
		make(chan *webrtc.DataChannel, 5)}
	server.Peers = append(server.Peers, peer)

	// Create a new webrtc API with a custom logger
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
		log.Printf("ICE Connection State change: %s\n", s)
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
func (server *WebRTCServer) Shutdown() {
	for _, peer := range server.Peers {
		if peer.pc != nil {
			peer.pc.Close()
		}
	}
	for _, c := range server.Cmds {
		c.C.Process.Kill()
	}
}

// Authenticate checks authorization args against system's user
func (peer *Peer) Authenticate(args *AuthArgs) bool {
	var hash string
	var c crypt.Crypter
	var err error

	sp := C.getspnam(C.CString(args.Username))
	if sp == nil {
		return false
	}
	pwdp := C.GoString(sp.sp_pwdp)
	i := 0
	salt := strings.IndexFunc(pwdp, func(r rune) bool {
		if r == '$' {
			i++
		}
		return i == 3
	})
	s := []byte(pwdp)[:salt]
	t := string(pwdp)[salt:]
	if t == args.Password {
		goto HappyEnd
	}
	c = sha512_crypt.New()
	hash, err = c.Generate([]byte(args.Password), s)
	if err != nil {
		log.Printf("Got an error generate the hash. salt: %q", pwdp[:salt])
	}
	log.Printf("shadow %q\ngenerated %q", pwdp, hash)
	if string(hash) != pwdp {
		return false
	}
HappyEnd:
	peer.Username = args.Username
	return true

}

// SendAck sends an ack for a given control message
func (peer *Peer) SendAck(cm CTRLMessage) {
	args := AckArgs{Ref: cm.MessageId}
	// TODO: protect message counter against reentrancy
	msg := CTRLMessage{time.Now().UnixNano(), peer.LastMsgId + 1, &args,
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
	fmt.Printf("Got a control message: %q\n", string(msg.Data))
	err := json.Unmarshal(msg.Data, &m)
	if err != nil {
		log.Printf("Failed to parse incoming control message: %v", err)
		return
	}
	if m.ResizePTY != nil {
		var ws pty.Winsize
		channel := peer.Channels[m.ResizePTY.ChannelId]
		ws.Cols = m.ResizePTY.Sx
		ws.Rows = m.ResizePTY.Sy
		log.Printf("Changing pty size for channel %v: %v", channel, ws)
		pty.Setsize(channel.Cmd.Tty, &ws)
		peer.SendAck(m)
	} else if m.Auth != nil {
		if peer.Authenticate(m.Auth) {
			peer.Authenticated = true
			peer.SendAck(m)
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
	Password string `json:"password"`
}

// AckArgs is a type to hold the args for an Ack message
type AckArgs struct {
	// Ref holds the message id the error refers to or 0 for system errors
	Ref int `json:"ref"`
}

// ResizePTYArgs is a type that holds the argumnet to the resize pty command
type ResizePTYArgs struct {
	ChannelId int
	Sx        uint16
	Sy        uint16
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

// OnMessage is called on incoming messages from the data channel.
// It simply write the recieved data to the pseudo tty
func (channel *Channel) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	log.Printf("< %v", p)
	l, err := channel.Cmd.Tty.Write(p)
	if err != nil {
		log.Panicf("Stdin Write returned an error: %v", err)
	}
	if l != len(p) {
		log.Panicf("stdin write wrote %d instead of %d bytes", l, len(p))
	}
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

// Write send a buffer of data over the data channel
// TODO: rename this function, we use Write because of io.Copy
func (channel *Channel) Write(p []byte) (int, error) {
	// TODO: logging...
	log.Printf("> %q", string(p))
	err := channel.dc.Send(p)
	if err != nil {
		return 0, fmt.Errorf("Data channel send failed: %v", err)
	}
	//TODO: can we get a truer value than `len(p)`
	return len(p), nil
}
