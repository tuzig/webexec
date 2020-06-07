// // Package server holds the code that runs a webrtc based service
// connecting commands with datachannels thru a pseudo tty.
package server

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
)

const connectionTimeout = 600 * time.Second
const keepAliveInterval = 15 * time.Minute
const peerBufferSize = 5000

// type WebRTCServer is the singelton we use to store server globals
type WebRTCServer struct {
	// peers holds the connected and disconnected peers
	peers []Peer
	// channels holds all the open channel we have with process ID as key
	// TODO: refactor to a slice
	channels map[int]*TerminalChannel
}

// type Peer is used to remember a client aka peer connection
type Peer struct {
	Id          int
	State       string
	Remote      string
	LastContact *time.Time
	LastMsgId   uint32
	Buffer      [][]byte
	pc          *webrtc.PeerConnection
	Answer      []byte
}

func NewWebRTCServer() (server WebRTCServer, err error) {
	// Register data channel creation handling
	return
}
func (server *WebRTCServer) OnChannelReq(d *webrtc.DataChannel) {
	var cmd *exec.Cmd
	if d.Label() == "signaling" {
		return
	}
	d.OnOpen(func() {
		var tty *os.File
		var err error
		l := d.Label()
		log.Printf("New Data channel %q\n", l)
		c := strings.Split(l, " ")
		// We get "terminal7" in c[0] as the first channel name
		// from a fresh client. This dc is used for as ctrl channel
		if c[0] == "%" {
			// TODO: register the control client so we can send notifications
			d.OnMessage(server.OnCTRLMsg)
			return
		}
		var firstRune rune = rune(c[0][0])
		// If the message starts with a digit we assume it starts with
		// a size
		if unicode.IsDigit(firstRune) {
			ws, err := parseWinsize(c[0])
			if err != nil {
				log.Printf("Failed to parse winsize: %q ", c[0])
			}
			cmd = exec.Command(c[1], c[2:]...)
			log.Printf("starting command with size: %v", ws)
			tty, err = pty.StartWithSize(cmd, &ws)
		} else {
			cmd = exec.Command(c[0], c[1:]...)
			log.Printf("starting command without size %v %v", cmd.Path, cmd.Args)
			tty, err = pty.Start(cmd)
		}
		if err != nil {
			log.Panicf("Failed to start command: %v", err)
		}
		defer func() { _ = tty.Close() }() // Best effort.
		// create the channel and add to the server's channels map
		channelId := cmd.Process.Pid
		finished := make(chan bool)
		channel := TerminalChannel{d, cmd, tty, finished}
		server.channels[channelId] = &channel
		d.OnMessage(channel.OnMessage)
		// send the channel id as the first message
		s := strconv.Itoa(channelId)
		bs := []byte(s)
		channel.Write(bs)
		// use copy to read command output and send it to the data channel
		io.Copy(&channel, tty)

		if err != nil {
			log.Printf("Failed to kill process: %v %v",
				err, cmd.ProcessState.String())
		}
		// <-channel.busy
		d.Close()
	})
	d.OnClose(func() {
		err := cmd.Process.Kill()
		if err != nil {
			log.Printf("Failed to kill process: %v %v", err, cmd.ProcessState.String())
		}
		log.Println("Data channel closed")
	})
}

// Listen opens a peer connection and starts listening for the offer
func (server *WebRTCServer) Listen(remote string) *Peer {
	// TODO: protected the next two line from re entrancy
	peer := Peer{len(server.peers), "init", remote, nil, 0, nil, nil, nil}
	server.peers = append(server.peers, peer)

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
	if len(remote) > 0 {
		offer := webrtc.SessionDescription{}
		signal.Decode(remote, &offer)
		err = pc.SetRemoteDescription(offer)
		if err != nil {
			panic(err)
		}
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
	pc.OnDataChannel(server.OnChannelReq)
	peer.pc = pc
	peer.Answer = []byte(signal.Encode(answer))

	return &peer
}

// Shutdown is called when it's time to go.
// Sweet dreams.
func (server *WebRTCServer) Shutdown() {
	for _, peer := range server.peers {
		peer.pc.Close()
	}
	for _, channel := range server.channels {
		if channel.cmd != nil {
			log.Print("Shutting down WebRTC server")
			channel.cmd.Process.Kill()
		}
	}
}

// OnCTRLMsg handles incoming control messages
func (server *WebRTCServer) OnCTRLMsg(msg webrtc.DataChannelMessage) {
	var m CTRLMessage
	fmt.Printf("Got a control message: %q\n", string(msg.Data))
	err := json.Unmarshal(msg.Data, &m)
	if err != nil {
		log.Printf("Failed to parse incoming control message: %v", err)
		return
	}
	if m.resizePTY != nil {
		var ws pty.Winsize
		ws.Cols = m.resizePTY.sx
		ws.Rows = m.resizePTY.sy
		pty.Setsize(server.channels[m.resizePTY.id].pty, &ws)
	}
	// TODO: add more commands here: mouse, clipboard, etc.
}

// ErrorArgs is a type that holds the args for an error message
type ErrorArgs struct {
	Description string
	// Ref holds the message id the error refers to or 0 for system errors
	Ref uint32
}

// ResizePTYArgs is a type that holds the argumnet to the resize pty command
type ResizePTYArgs struct {
	id int
	sx uint16
	sy uint16
}

// CTRLMessage type holds control messages passed over the control channel
type CTRLMessage struct {
	time      float64        `json:"time"`
	messageId int            `json:"message_id"`
	resizePTY *ResizePTYArgs `json:"resize_pty"`
	Error     *ErrorArgs
}

// Type TerminalChannel holds the holy trinity: a data channel, a command and
// a pseudo tty.
type TerminalChannel struct {
	dc   *webrtc.DataChannel
	cmd  *exec.Cmd
	pty  *os.File
	busy chan bool
}

// Write send a buffer of data over the data channel
// TODO: rename this function, we use Write because of io.Copy
func (channel *TerminalChannel) Write(p []byte) (int, error) {
	// TODO: logging...
	log.Printf("> %q", string(p))
	if false {
		text := string(p)
		for _, r := range strings.Split(text, "\r\n") {
			if len(r) > 0 {
				log.Printf("> %q\n", r)
			}
		}
	}
	// channel.busy <- true
	err := channel.dc.Send(p)
	// channel.busy <- false
	if err != nil {
		return 0, fmt.Errorf("Data channel send failed: %v", err)
	}
	//TODO: can we get a truer value than `len(p)`
	return len(p), nil
}

// OnMessage is called on incoming messages from the data channel.
// It simply write the recieved data to the pseudo tty
func (channel *TerminalChannel) OnMessage(msg webrtc.DataChannelMessage) {
	p := msg.Data
	log.Printf("< %v", p)
	l, err := channel.pty.Write(p)
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
