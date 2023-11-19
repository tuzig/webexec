// This files contains testing suites that test webexec as a compoennt and
// using a test client
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"github.com/tuzig/webexec/peers"
)

// SendRestore sends an restore message
func SendRestore(cdc *webrtc.DataChannel, ref int, marker int) error {
	time.Sleep(10 * time.Millisecond)
	msg := peers.CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "restore",
		Ref:  ref,
		Args: peers.RestoreArgs{marker},
	}
	restoreMsg, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("Failed to marshal the auth args: %v", err)
	}
	cdc.Send(restoreMsg)
	return nil
}

// waitForChild waits for a give timeout for for a process to die
func waitForChild(pid int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) <= timeout {
		if !isAlive(pid) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("process %d still alive (timeout=%v)", pid, timeout)
}

// SignalPair is used to start a connection between a peer and a client
func SignalPair(client *webrtc.PeerConnection, peer *peers.Peer) error {
	// Note(albrow): We need to create a data channel in order to trigger ICE
	// candidate gathering in the background for the JavaScript/Wasm bindings. If
	// we don't do this, the complete offer including ICE candidates will never be
	// generated.
	if _, err := client.CreateDataChannel("signaling", nil); err != nil {
		return err
	}

	offer, err := client.CreateOffer(nil)
	if err != nil {
		return err
	}
	gatherComplete := webrtc.GatheringCompletePromise(client)
	if err := client.SetLocalDescription(offer); err != nil {
		return err
	}
	select {
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed mockedMsgs waiting to finish gathering ICE candidates")
	case <-gatherComplete:
		answer, err := peer.Listen(*client.LocalDescription())
		if err != nil {
			return err
		}
		err = client.SetRemoteDescription(*answer)
		if err != nil {
			return err
		}
		return nil
	}
}

func getMarker(cdc *webrtc.DataChannel) int {
	lastRef++
	ref := lastRef
	// sleep to simulate latency
	time.Sleep(10 * time.Millisecond)
	//TODO we need something like peer.LastMsgId++ below
	msg := peers.CTRLMessage{
		Time: time.Now().UnixNano(),
		Type: "mark",
		Ref:  ref,
		Args: nil,
	}
	markMsg, err := json.Marshal(msg)
	if err != nil {
		return -1
	}
	cdc.Send(markMsg)
	return ref
}

// ParseAck parses and ack message and returns its args
func ParseAck(t *testing.T, msg webrtc.DataChannelMessage) peers.AckArgs {
	var args json.RawMessage
	var ackArgs peers.AckArgs
	env := peers.CTRLMessage{
		Args: &args,
	}
	err := json.Unmarshal(msg.Data, &env)
	require.Nil(t, err, "failed to unmarshal cdc message: %q", err)
	require.Equal(t, env.Type, "ack",
		"Expected an ack message and got %q", env.Type)
	err = json.Unmarshal(args, &ackArgs)
	require.Nil(t, err, "failed to unmarshal ack args: %q", err)
	return ackArgs
}

func TestSimpleEcho(t *testing.T) {
	initTest(t)
	Logger.Infof("TestSimpleEcho")
	closed := make(chan bool)
	client, certs, err := NewClient(true)
	peer := newPeer(t, "A", certs)
	// count the incoming messages
	count := 0
	dc, err := client.CreateDataChannel("echo,hello world", nil)
	require.Nil(t, err, "Failed to create the echo data channel: %v", err)
	dc.OnOpen(func() {
		Logger.Infof("Channel %q opened", dc.Label())
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// first get a channel Id and then "hello world" text
		Logger.Infof("got message: #%d %q", count, string(msg.Data))
		if count == 0 {
			_, err = strconv.Atoi(string(msg.Data))
			require.Nil(t, err, "Failed to cover channel it to int: %v", err)
		} else if count == 1 {
			require.EqualValues(t, string(msg.Data)[:11], "hello world",
				"Got bad msg: %q", msg.Data)
		}
		count++
	})
	dc.OnClose(func() {
		Logger.Info("Client Data channel closed")
		closed <- true
	})
	err = SignalPair(client, peer)
	require.NoError(t, err, "Signaling failed: %v", err)
	// TODO: add timeout
	Logger.Infof("Waiting for the channel to close")
	<-closed
	Logger.Infof("TestSimpleEcho done")
	panes := peers.Panes.All()
	lp := panes[len(panes)-1]

	waitForChild(lp.C.Process.Pid, time.Second)
	lp.Lock()
	require.False(t, lp.IsRunning)
	lp.Unlock()
	// For some reason we sometimes get an empty message and count can be 3
	require.GreaterOrEqual(t, count, 2, "Expected to recieve 2 messages and got %d", count)
}

func TestResizeCommand(t *testing.T) {
	initTest(t)
	done := make(chan bool)
	client, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer := newPeer(t, "A", certs)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		// control channel is open let's open another one, so we'll have
		// what to resize
		// dc, err := client.CreateDataChannel("12x34,bash", nil)
		// require.Nil(t, err, "failed to create the a channel: %v", err)
		addPaneArgs := peers.AddPaneArgs{Rows: 12, Cols: 34,
			Command: []string{"bash"}}
		m := peers.CTRLMessage{time.Now().UnixNano(), 123, "add_pane",
			&addPaneArgs}
		addPaneMsg, err := json.Marshal(m)
		require.NoError(t, err, "failed marshilng ctrl msg: %s", err)
		Logger.Info("Sending the addPane message")
		cdc.Send(addPaneMsg)
	})
	cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
		ack := ParseAck(t, msg)
		if ack.Ref == 123 {
			// parse add_pane and send the resize command
			// extract paneID from the ack body whcih is raw json
			paneID, err := strconv.Atoi(string(ack.Body))
			require.NoError(t, err, "failed to unmarshal ack body: %s", err)
			require.NotEqual(t, -1, paneID, "Got a bad pane id: %d", paneID)
			resizeArgs := peers.ResizeArgs{paneID, 80, 24}
			m := peers.CTRLMessage{time.Now().UnixNano(), 456, "resize",
				&resizeArgs}
			resizeMsg, err := json.Marshal(m)
			require.Nil(t, err, "failed marshilng ctrl msg: %v", err)
			Logger.Info("Sending the resize message")
			cdc.Send(resizeMsg)

		} else if ack.Ref == 456 {
			done <- true
		}
	})
	// all set, let's start the signaling
	SignalPair(client, peer)
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for dat ain reconnected pane")
	case <-done:
	}
}

func TestPayloadOperations(t *testing.T) {
	initTest(t)
	done := make(chan bool)
	client, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer := newPeer(t, "A", certs)
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	payload := []byte("[\"Better payload\"]")
	cdc.OnOpen(func() {
		time.Sleep(10 * time.Millisecond)
		args := peers.SetPayloadArgs{payload}
		setPayload := peers.CTRLMessage{time.Now().UnixNano(), 777,
			"set_payload", &args}
		setMsg, err := json.Marshal(setPayload)
		require.Nil(t, err, "Failed to marshal the auth args: %v", err)
		Logger.Info("Sending set_payload msg")
		cdc.Send(setMsg)
	})
	cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// we should get an ack for the auth message and the get payload
		Logger.Infof("Got a ctrl msg: %s", msg.Data)
		args := ParseAck(t, msg)
		if args.Ref == 777 {
			require.Nil(t, err, "Failed to unmarshall the control data channel: %v", err)
			require.Equal(t, payload, []byte(args.Body),
				"Got the wrong payload: %q", args.Body)
			done <- true
		}
	})
	SignalPair(client, peer)
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for payload data")
	case <-done:
	}
	// TODO: now get_payload and make sure it's the same
}
func TestMarkerRestore(t *testing.T) {
	initTest(t)
	var (
		cID         string
		dc          *webrtc.DataChannel
		markerRef   int
		markerMutex sync.Mutex
		marker      int
	)
	gotSetMarkerAck := make(chan bool)
	gotFirst := make(chan bool)
	gotSecondAgain := make(chan bool)
	gotAck := make(chan bool)
	client, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	// start the server
	peer := newPeer(t, "A", certs)
	require.Nil(t, err, "Failed to start a new server %v", err)
	// create the command & control data channel
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	// count the incoming messages
	count := 0
	cdc.OnOpen(func() {
		Logger.Info("cdc is opened")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message
			var cm peers.CTRLMessage
			Logger.Infof("Got a ctrl msg: %s", msg.Data)
			err := json.Unmarshal(msg.Data, &cm)
			require.Nil(t, err, "Failed to marshal the server msg: %v", err)
			if cm.Type == "ack" {
				args := ParseAck(t, msg)
				markerMutex.Lock()
				defer markerMutex.Unlock()
				if args.Ref == markerRef {
					// convert the body to int
					marker, err = strconv.Atoi(string(args.Body))
					require.NoError(t, err)
					Logger.Infof("Got marker: %d", marker)
					gotSetMarkerAck <- true
				}
			}
		})
		dc, err = client.CreateDataChannel(
			"24x80,bash,-c,echo 123456 ; sleep 1; echo 789; sleep 9", nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			Logger.Infof("Channel %q opened", dc.Label())
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			Logger.Infof("DC Got msg #%d: %s", count, msg.Data)
			if len(msg.Data) > 0 {
				if count == 0 {
					cID = string(msg.Data)
					Logger.Infof("Client got a channel id: %q", cID)
				}
				if count == 1 {
					require.Contains(t, string(msg.Data), "123456")
					gotFirst <- true
				}
				count++
			}
		})
	})
	SignalPair(client, peer)
	select {
	case <-time.After(6 * time.Second):
		t.Error("Timeout waiting for first datfirst data data")
	case <-gotFirst:
	}
	markerMutex.Lock()
	markerRef = getMarker(cdc)
	markerMutex.Unlock()
	select {
	case <-time.After(6 * time.Second):
		t.Error("Timeout waiting for marker ack")
	case <-gotSetMarkerAck:
	}
	client2, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create the second client %v", err)
	peer2 := newPeer(t, "A", certs)
	require.Nil(t, err, "Failed to start a new server %v", err)
	// create the command & control data channel
	SignalPair(client2, peer2)
	cdc2, err := client2.CreateDataChannel("%", nil)
	require.Nil(t, err, "Failed to create the control data channel: %v", err)
	// count the incoming messages
	cdc2.OnOpen(func() {
		// send the restore message "marker"
		go SendRestore(cdc2, 345, marker)
		cdc2.OnMessage(func(msg webrtc.DataChannelMessage) {
			// we should get an ack for the auth message
			var cm peers.CTRLMessage
			err := json.Unmarshal(msg.Data, &cm)
			require.Nil(t, err, "Failed to marshal the server msg: %v", err)
			Logger.Info("client2 got msg: %v", cm)
			if cm.Type == "ack" {
				gotAck <- true
			}
		})
		<-gotAck
		time.Sleep(time.Second)
		dc, err = client2.CreateDataChannel(">"+cID, nil)
		require.Nil(t, err, "Failed to create the echo data channel: %v", err)
		dc.OnOpen(func() {
			Logger.Infof("TS> Channel %q re-opened", dc.Label())
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// ignore null messages
			if len(msg.Data) > 0 {
				require.Contains(t, string(msg.Data), "789")
				gotSecondAgain <- true
			}

		})
	})
	select {
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for restored data")
	case <-gotSecondAgain:
	}
}
func TestAddPaneMessage(t *testing.T) {
	initTest(t)
	var wg sync.WaitGroup
	// the trinity: a new datachannel, an ack and BADWOLF (aka command output)
	wg.Add(3)
	client, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer := newPeer(t, "A", certs)
	done := make(chan bool)
	client.OnDataChannel(func(d *webrtc.DataChannel) {
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			Logger.Infof("Got a new datachannel message: %s", string(msg.Data))
			require.Equal(t, "BADWOLF", string(msg.Data[:7]))
			Logger.Infof(string(msg.Data))
			wg.Done()
		})
		l := d.Label()
		Logger.Infof("Got a new datachannel: %s", l)
		require.Regexp(t, regexp.MustCompile("^456:[0-9]+"), l)
		wg.Done()
	})
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		Logger.Info("cdc opened")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ack := ParseAck(t, msg)
			if ack.Ref == 456 {
				Logger.Infof("Got the ACK")
				wg.Done()
			}
		})
		addPaneArgs := peers.AddPaneArgs{Rows: 12, Cols: 34,
			Command: []string{"echo", "BADWOLF"}} //, "&&", "sleep", "5"}}
		m := peers.CTRLMessage{time.Now().UnixNano(), 456, "add_pane",
			&addPaneArgs}
		msg, err := json.Marshal(m)
		require.Nil(t, err, "failed marshilng ctrl msg: %v", msg)
		time.Sleep(time.Second / 10)
		cdc.Send(msg)
	})
	go func() {
		wg.Wait()
		done <- true
	}()
	SignalPair(client, peer)
	select {
	case <-time.After(6 * time.Second):
		t.Error("Timeout waiting for server to open channel")
	case <-done:
	}
}
func TestReconnectPane(t *testing.T) {
	initTest(t)
	var (
		wg     sync.WaitGroup
		gotMsg sync.WaitGroup
		ci     int
	)
	client, certs, err := NewClient(true)
	require.Nil(t, err, "Failed to create a new client %v", err)
	peer := newPeer(t, "A", certs)
	client.OnDataChannel(func(d *webrtc.DataChannel) {
		l := d.Label()
		//fs := strings.Split(d.Label(), ",")
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			if len(msg.Data) > 0 {
				Logger.Infof("Got a message in %s: %s", l, string(msg.Data))
				if strings.Contains(string(msg.Data), "BADWOLF") {
					gotMsg.Done()
				}
			}
		})
		Logger.Infof("Got a new datachannel: %s", l)
		require.Regexp(t, regexp.MustCompile("^45[67]:[0-9]+"), l)
		wg.Done()
	})
	cdc, err := client.CreateDataChannel("%", nil)
	require.Nil(t, err, "failed to create the control data channel: %v", err)
	cdc.OnOpen(func() {
		Logger.Info("cdc opened")
		cdc.OnMessage(func(msg webrtc.DataChannelMessage) {
			Logger.Infof("cdc got an ack: %v", string(msg.Data))
			ack := ParseAck(t, msg)
			if ack.Ref == 456 {
				ci, err = strconv.Atoi(string(ack.Body))
				require.Nil(t, err)
				wg.Done()
			}
		})
		time.Sleep(time.Second / 100)
		addPaneArgs := peers.AddPaneArgs{Rows: 12, Cols: 34,
			Command: []string{"bash", "-c", "sleep 1; echo BADWOLF"}}
		m := peers.CTRLMessage{time.Now().UnixNano(), 456, "add_pane",
			&addPaneArgs}
		msg, err := json.Marshal(m)
		require.Nil(t, err, "failed marshilng ctrl msg: %v", msg)
		cdc.Send(msg)
	})
	wg.Add(2)
	gotMsg.Add(2)
	SignalPair(client, peer)
	wg.Wait()
	Logger.Infof("After first wait")
	wg.Add(1)
	a := peers.ReconnectPaneArgs{ID: ci}
	m := peers.CTRLMessage{time.Now().UnixNano(), 457, "reconnect_pane",
		&a}
	msg, err := json.Marshal(m)
	require.Nil(t, err, "failed marshilng ctrl msg: %v", msg)
	cdc.Send(msg)
	gotMsg.Wait()
}
func TestExecCommand(t *testing.T) {
	initTest(t)
	c := []string{"bash", "-c", "echo hello"}
	_, tty, err := peers.ExecCommand(c, nil, nil, 0, "")
	b := make([]byte, 64)
	l, err := tty.Read(b)
	require.Nil(t, err)
	require.Less(t, 6, l, "Expected at least 5 bytes %s", string(b))
	require.Equal(t, "hello", string(b[:5]))
}
func TestExecCommandWithParent(t *testing.T) {
	initTest(t)
	c := []string{"sh"}
	cmd, tty, err := peers.ExecCommand(c, nil, nil, 0, "")
	time.Sleep(time.Second / 100)
	_, err = tty.Write([]byte("cd /tmp\n"))
	require.Nil(t, err)
	_, err = tty.Write([]byte("pwd\n"))
	require.Nil(t, err)
	time.Sleep(time.Second / 10)
	_, tty2, err := peers.ExecCommand([]string{"pwd"}, nil, nil, cmd.Process.Pid, "")
	require.Nil(t, err)
	b := make([]byte, 64)
	l, err := tty2.Read(b)
	require.NoError(t, err)
	require.Less(t, 5, l, "Expected at least 5 bytes %s", string(b))
	// validate that the pwd ends with "/tmp"
	cwd := string(b[:l])
	require.True(t, strings.HasSuffix(cwd, "/tmp\r\n"), "Expected ouput to end with /tmp, got %s", cwd)
}
