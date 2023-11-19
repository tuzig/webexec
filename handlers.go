package main

import (
	"encoding/json"
	"fmt"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v3"
	"github.com/riywo/loginshell"
	"github.com/tuzig/webexec/peers"
)

// handleResize handles resize control messages.
func handleResize(peer *peers.Peer, m peers.CTRLMessage, rawArgs json.RawMessage) {
	var resizeArgs peers.ResizeArgs
	err := json.Unmarshal(rawArgs, &resizeArgs)
	if err != nil {
		Logger.Infof("Failed to parse incoming control message: %v", err)
		return
	}
	cID := resizeArgs.PaneID
	pane := peers.Panes.Get(cID)
	if pane == nil {
		Logger.Error("Failed to parse resize message pane_id out of range")
		return
	}
	if pane.TTY == nil {
		Logger.Warnf("Tried to resize a pane with no tty")
		peer.SendNack(m, "Tried to resize a pane with no tty")
		return
	}
	var ws pty.Winsize
	ws.Cols = resizeArgs.Sx
	ws.Rows = resizeArgs.Sy
	pane.Resize(&ws)
	err = peer.Broadcast("resize", resizeArgs)
	if err != nil {
		Logger.Errorf("Failed to broadcast resize message: %v", err)
		peer.SendNack(m, "Failed to broadcast resize message")
		return
	}
	err = peer.SendAck(m, "")
	if err != nil {
		Logger.Errorf("#%d: Failed to send a resize ack: %v", peer.FP, err)
	}
}

// handleRestore handles restore control messages.
func handleRestore(peer *peers.Peer, m peers.CTRLMessage, rawArgs json.RawMessage) {
	var args peers.RestoreArgs
	err := json.Unmarshal(rawArgs, &args)
	if err != nil {
		Logger.Infof("Failed to parse incoming control message: %v", err)
		return
	}
	peer.Marker = args.Marker
	err = peer.SendAck(m, string(peers.Payload))
	if err != nil {
		Logger.Errorf("#%d: Failed to send restore ack: %v", peer.FP, err)
	}
}

// handleGetPayload handles get_payload control messages.
func handleGetPayload(peer *peers.Peer, m peers.CTRLMessage) {
	err := peer.SendAck(m, string(peers.Payload))
	if err != nil {
		Logger.Errorf("#%d: Failed to send get_payload ack: %v", peer.FP, err)
	}
}

// handleSetPayload handles set_payload control messages.
func handleSetPayload(peer *peers.Peer, m peers.CTRLMessage, raw json.RawMessage) {
	var payloadArgs peers.SetPayloadArgs
	err := json.Unmarshal(raw, &payloadArgs)
	if err != nil {
		Logger.Infof("Failed to parse incoming control message: %v", err)
		return
	}
	peers.Payload = payloadArgs.Payload
	// send the set_payload message to all connected peers
	peer.Broadcast("set_payload", payloadArgs)
	err = peer.SendAck(m, string(peers.Payload))
	if err != nil {
		Logger.Errorf("#%d: Failed to send set_payload ack: %v", peer.FP, err)
	}
}

// handlemark handles mark control messages.
func handleMark(peer *peers.Peer, m peers.CTRLMessage) {
	markerM.Lock()
	lastMarker++
	peer.Marker = lastMarker
	markerM.Unlock()
	for _, pane := range peers.Panes.All() {
		pane.Buffer.Mark(peer.Marker)
	}
	err := peer.SendAck(m, fmt.Sprintf("%d", peer.Marker))
	if err != nil {
		Logger.Errorf("#%d: Failed to send mark ack: %v", peer.FP, err)
	}
}

// handleReconnectPane handles reconnect_pane control messages.
func handleReconnectPane(peer *peers.Peer, m peers.CTRLMessage, raw json.RawMessage) {
	var a peers.ReconnectPaneArgs
	t := true
	dcOpts := &webrtc.DataChannelInit{Ordered: &t}
	err := json.Unmarshal(raw, &a)
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
			peer.SendAck(m, fmt.Sprintf("%d", pane.ID))
		}
	})
}
func handleAddPane(peer *peers.Peer, m peers.CTRLMessage, raw json.RawMessage) {
	var a peers.AddPaneArgs
	var ws *pty.Winsize
	t := true
	dcOpts := &webrtc.DataChannelInit{Ordered: &t}
	err := json.Unmarshal(raw, &a)
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
			a.Command[0] = "/bin/bash"
		} else {
			Logger.Infof("Using %s for shell", shell)
			a.Command[0] = shell
		}
	}
	cmd := a.Command
	pane, err := peers.NewPane(peer, ws, a.Parent)
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
		if peer.Conf.GetWelcome != nil {
			msg := peer.Conf.GetWelcome()
			Logger.Infof("Sending welcome message: %s", msg)
			err := d.Send([]byte(msg))
			if err != nil {
				Logger.Warnf("Failed to send welcome message: %v", err)
			}
		}
		pane.Run(cmd)
		c := peers.CDB.Add(d, pane, peer)
		Logger.Infof("opened data channel for pane %d", pane.ID)
		peer.SendAck(m, fmt.Sprintf("%d", pane.ID))
		d.OnMessage(pane.OnMessage)
		d.OnClose(func() {
			peers.CDB.Delete(c)
		})
	})
}
