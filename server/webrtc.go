// Package server holds the code that runs a webrtc based service
// connecting commands with datachannels thru a pseudo tty.
package server

/*
#include <shadow.h>
#include <stddef.h>
#include <stdlib.h>
// MT: This should be after the comment
import "C"
*/

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

// type Command hold an executed command, it's pty and buffer
type Command struct {
	Id int
	// C holds the exectuted command
	C      *exec.Cmd
	Tty    *os.File
	Buffer [][]byte
	dcs    []*webrtc.DataChannel
}

// type WebRTCServer is the singelton we use to store server globals
type WebRTCServer struct {
	// Peers holds the connected and disconnected peers
	Peers []Peer
	// Cmds holds all the open channels we ever opened
	Cmds []*Command
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
	cdc               *webrtc.DataChannel
	Username          string
	PendingChannelReq chan *webrtc.DataChannel
}

func NewWebRTCServer() (server WebRTCServer, err error) {
	return WebRTCServer{nil, nil}, nil
	// Register data channel creation handling
}

// start a system command over a pty. If the command contains a dimension
// in the format of 24x80 the login shell is lunched
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
		log.Printf("Bufferinga a channel request from an unauthenticated peer: %q",
			d.Label())
		peer.PendingChannelReq <- d
		return
	}

	// MT: This code is "dense", need to move to smaller functions
	d.OnOpen(func() {
		var err error
		l := d.Label()
		// We get "terminal7" in c[0] as the first channel name
		// from a fresh client. This dc is used for as ctrl channel
		if l[0] == '%' {
			//TODO: if there's an older cdc close it
			log.Println("On open for a new control channel")
			peer.cdc = d
			d.OnMessage(peer.OnCTRLMsg)
			return
		} else {
			log.Printf("On open for a new Data channel %q\n", l)
		}
		cmd, err := peer.server.PipeCommand(l, d, peer.Username)
		if err != nil {
			log.Printf("Failed to pipe command: %v", err)
			return
		}
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			p := msg.Data
			log.Printf("< %v", p)
			l, err := cmd.Tty.Write(p)
			if err != nil {
				// MT: Don't panic
				log.Panicf("Stdin Write returned an error: %v", err)
			}
			if l != len(p) {
				log.Panicf("stdin write wrote %d instead of %d bytes", l, len(p))
			}
		})
		d.OnClose(func() {
			// TODO: ssupend a process and start a kill timer of a week or so
			log.Printf("Data channel closed : %q", d.Label())
		})
		// use copy to read command output and send it to the data channel

	})
}

// PipeCommand is a function that gets a data channel label, it's data channel
// and a user name.
// The function parses the label to figure out if it needs to start
// a new process or connect to existing process, i.e. "24x80 >123" to reconnect
// to command 123
func (server *WebRTCServer) PipeCommand(c string, d *webrtc.DataChannel,
	username string) (*Command, error) {
	/* MT: You can group vars in () as in
	var (
		cmd *exec.Cmd
		tty *os.File
		...
	)
	*/
	var cmd *exec.Cmd
	var tty *os.File
	var err error
	var reconnect bool
	var cId int
	var ret *Command
	// If the message starts with a digit we assume it starts with a size
	if unicode.IsDigit(rune(c[0])) {
		sep := strings.IndexRune(c, ' ')
		if sep == -1 {
			sep = len(c)
		} else if rune(c[sep+1]) == '>' {
			log.Printf("It's a reconnect!")
			// A reconnect request, i.e. ">123"
			cId, err = strconv.Atoi(c[sep+2:])
			if err != nil {
				return nil, fmt.Errorf("Got an error converting incoming reconnect channel id: %q", c[sep+1:])
			}
			if cId > len(server.Cmds) {
				return nil, fmt.Errorf("Got a bad channelId: %d", cId)
			}
			ret = server.Cmds[cId-1]
			reconnect = true
		}
		ws, err := parseWinsize(c[:sep])
		if err != nil {
			return nil, fmt.Errorf("Got an error parsing window size: %q", c[:sep])
		}
		if !reconnect {
			suArgs := []string{"-", username}
			cmd = exec.Command("su", suArgs...)
			tty, err = pty.StartWithSize(cmd, &ws)
			if err != nil {
				return nil, fmt.Errorf("Failed to start command: %v", err)
			}
		}
	} else {
		suArgs := []string{"-c", c, username}
		cmd = exec.Command("su", suArgs...)
		tty, err = pty.Start(cmd)
		if err != nil {
			return nil, fmt.Errorf("Got an error starting command: %v", err)
		}
	}
	if !reconnect {
		// Start a fresh command
		ret = &Command{len(server.Cmds), cmd, tty, nil,
			[]*webrtc.DataChannel{d}}
		server.Cmds = append(server.Cmds, ret)
		go func() {
			cmd.Wait()
			tty.Close()
		}()
		// NewCommand is up to here
		cId = len(server.Cmds)
		log.Printf("Added a command: id %d", cId)
		go ret.ReadLoop()
	} else {
		// Add the data channel to the command
		ret.dcs = append(ret.dcs, d)
	}

	// send the channel id as the first message
	s := strconv.Itoa(cId)
	bs := []byte(s)
	d.Send(bs)
	return ret, nil

}

func (cmd *Command) ReadLoop() {
	var i int
	// MT: io.Copy & https://golang.org/pkg/io/#MultiWriter
	// You can use SIGCHILD to know if a child process died
	b := make([]byte, 4096)
	for cmd.C.ProcessState.String() != "killed" {
		l, err := cmd.Tty.Read(b)
		if err != nil && err != io.EOF {
			log.Printf("Failed reading command output: %v", err)
			break
		}
		for i = 0; i < len(cmd.dcs); i++ {
			dc := cmd.dcs[i]
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
	for i = 0; i < len(cmd.dcs); i++ {
		dc := cmd.dcs[i]
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			fmt.Printf("Closing data channel: %q", dc.Label())
			dc.Close()
		}
	}
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
		"connected", remote, nil, 0, nil, nil, nil, "",
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
		cmd := peer.server.Cmds[cId-1]
		ws.Cols = m.ResizePTY.Sx
		ws.Rows = m.ResizePTY.Sy
		log.Printf("Changing pty size for channel %v: %v", cmd, ws)
		pty.Setsize(cmd.Tty, &ws)
		peer.SendAck(m, "")
	} else if m.Auth != nil {
		token := Authenticate(m.Auth)
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
