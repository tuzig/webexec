package peers

import (
	"io"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockTTY is a minimal io.ReadWriteCloser for testing OnMessage.
type mockTTY struct {
	written []byte
}

func (m *mockTTY) Read(p []byte) (int, error) { return 0, io.EOF }
func (m *mockTTY) Write(p []byte) (int, error) {
	m.written = append(m.written, p...)
	return len(p), nil
}
func (m *mockTTY) Close() error { return nil }

// resetActivePeer resets the package-level active-peer globals and registers
// a cleanup so subsequent tests see a clean slate.
func resetActivePeer(t *testing.T) {
	t.Helper()
	mostRecentPeer = nil
	lastReceived = time.Time{}
	t.Cleanup(func() {
		mostRecentPeer = nil
		lastReceived = time.Time{}
	})
}

func newTestPane(t *testing.T) *Pane {
	t.Helper()
	peer := &Peer{}
	peer.logger = zaptest.NewLogger(t).Sugar()
	return &Pane{
		peer:   peer,
		outbuf: make(chan []byte, OutBufSize),
	}
}

func TestInterceptQueryCSI6n(t *testing.T) {
	p := newTestPane(t)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b[6n"))
	require.True(t, handled)
	require.NotEmpty(t, resp)
	require.Equal(t, byte(0x1b), resp[0])
	require.Equal(t, byte('['), resp[1])
}

func TestInterceptQueryCSI5n(t *testing.T) {
	p := newTestPane(t)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b[5n"))
	require.True(t, handled)
	require.Equal(t, "\x1b[0n", string(resp))
}

func TestInterceptQueryCSIC(t *testing.T) {
	p := newTestPane(t)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b[c"))
	require.True(t, handled)
	require.Equal(t, "\x1b[?1;2c", string(resp))

	resp, handled = p.interceptQuery(p.peer, []byte("\x1b[0c"))
	require.True(t, handled)
	require.Equal(t, "\x1b[?1;2c", string(resp))
}

func TestInterceptQueryCSIGreaterC(t *testing.T) {
	p := newTestPane(t)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b[>c"))
	require.True(t, handled)
	require.Equal(t, "\x1b[>0;115;0c", string(resp))

	resp, handled = p.interceptQuery(p.peer, []byte("\x1b[>0c"))
	require.True(t, handled)
	require.Equal(t, "\x1b[>0;115;0c", string(resp))
}

func TestInterceptQueryPassThrough(t *testing.T) {
	p := newTestPane(t)

	_, handled := p.interceptQuery(p.peer, []byte("a"))
	require.False(t, handled)

	_, handled = p.interceptQuery(p.peer, []byte("\x1b[A"))
	require.False(t, handled)

	_, handled = p.interceptQuery(p.peer, []byte("\x1b[2J"))
	require.False(t, handled)

	_, handled = p.interceptQuery(p.peer, []byte("\x1b[4n"))
	require.False(t, handled)
}

func TestIsOSCColorQuery(t *testing.T) {
	require.True(t, isOSCColorQuery([]byte("\x1b]11;?\x07")))
	require.True(t, isOSCColorQuery([]byte("\x1b]10;?\x07")))
	require.True(t, isOSCColorQuery([]byte("\x1b]11;?\x1b\\")))
	require.False(t, isOSCColorQuery([]byte("\x1b]11")))
	require.False(t, isOSCColorQuery([]byte("\x1b]12;?\x07")))
	require.False(t, isOSCColorQuery([]byte("\x1b]11;a\x07")))
}

func TestInterceptQueryOSCColorFromActivePeer(t *testing.T) {
	resetActivePeer(t)
	p := newTestPane(t)

	SetLastPeer(p.peer)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b]11;?\x07"))
	require.False(t, handled, "active peer OSC should pass through to PTY")
	require.Nil(t, resp)
}

func TestInterceptQueryOSCColorFromInactivePeer(t *testing.T) {
	resetActivePeer(t)
	p := newTestPane(t)

	// No active peer — OSC query should be dropped
	resp, handled := p.interceptQuery(p.peer, []byte("\x1b]11;?\x07"))
	require.True(t, handled, "non-active peer OSC should be dropped")
	require.Empty(t, resp)
}

// TestInterceptQueryOSCColorFromReconnectingPeer verifies that when peerB
// reconnects to a pane originally created by peerA, the OSC color query
// is filtered against the *sender* (peerB), not pane.peer (peerA).
func TestInterceptQueryOSCColorFromReconnectingPeer(t *testing.T) {
	resetActivePeer(t)
	peerA := &Peer{}
	peerA.logger = zaptest.NewLogger(t).Sugar()
	peerB := &Peer{}
	peerB.logger = zaptest.NewLogger(t).Sugar()

	// Pane was created by peerA, but peerB is now connected
	p := &Pane{
		peer:   peerA,
		outbuf: make(chan []byte, OutBufSize),
	}

	// peerB is the active peer (from real typing)
	SetLastPeer(peerB)

	// peerB sends OSC 11 — active peer, should pass through
	resp, handled := p.interceptQuery(peerB, []byte("\x1b]11;?\x07"))
	require.False(t, handled, "active peer (peerB) OSC should pass through to PTY")
	require.Nil(t, resp)

	// peerA sends OSC 11 — NOT active peer, should be dropped
	resp, handled = p.interceptQuery(peerA, []byte("\x1b]11;?\x07"))
	require.True(t, handled, "inactive peer (peerA) OSC should be dropped")
	require.Empty(t, resp)
}

// TestOnMessagePassthroughSetsLastPeer verifies that real input (keystrokes)
// flowing through OnMessage updates the active peer via SetLastPeer.
func TestOnMessagePassthroughSetsLastPeer(t *testing.T) {
	resetActivePeer(t)
	peer := &Peer{}
	peer.logger = zaptest.NewLogger(t).Sugar()
	tty := &mockTTY{}
	p := &Pane{
		peer:   peer,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("echo hello\r")})

	require.Same(t, peer, mostRecentPeer, "pass-through input should set last peer")
	require.Equal(t, []byte("echo hello\r"), tty.written)
}

// TestOnMessageQueryDoesNotSetLastPeer verifies that intercepted query
// sequences do NOT update the active peer — this is the race condition fix.
func TestOnMessageQueryDoesNotSetLastPeer(t *testing.T) {
	resetActivePeer(t)
	peer := &Peer{}
	peer.logger = zaptest.NewLogger(t).Sugar()
	tty := &mockTTY{}
	p := &Pane{
		peer:   peer,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// DSR cursor position query — should be intercepted, not written to PTY
	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("\x1b[6n")})

	require.Nil(t, mostRecentPeer, "intercepted query should not set last peer")
	require.Empty(t, tty.written, "intercepted query should not reach PTY")

	// DSR device status query
	mostRecentPeer = nil
	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("\x1b[5n")})
	require.Nil(t, mostRecentPeer, "intercepted DSR should not set last peer")

	// Device attributes query
	mostRecentPeer = nil
	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("\x1b[c")})
	require.Nil(t, mostRecentPeer, "intercepted DA should not set last peer")
}

// TestOnMessageOSCColorDroppedDoesNotSetLastPeer verifies that an OSC color
// query from a non-active peer is dropped and does not update the active peer.
func TestOnMessageOSCColorDroppedDoesNotSetLastPeer(t *testing.T) {
	resetActivePeer(t)
	peer := &Peer{}
	peer.logger = zaptest.NewLogger(t).Sugar()
	tty := &mockTTY{}
	p := &Pane{
		peer:   peer,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// No active peer — OSC query should be dropped
	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("\x1b]11;?\x07")})

	require.Nil(t, mostRecentPeer, "dropped OSC query should not set last peer")
	require.Empty(t, tty.written, "dropped OSC query should not reach PTY")
}

// TestOnMessageOSCColorActivePeerPassesThrough verifies that an OSC color
// query from the active peer passes through to the PTY and updates the
// active peer (since it's real input heading to the PTY).
func TestOnMessageOSCColorActivePeerPassesThrough(t *testing.T) {
	resetActivePeer(t)
	peer := &Peer{}
	peer.logger = zaptest.NewLogger(t).Sugar()
	tty := &mockTTY{}
	p := &Pane{
		peer:   peer,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// Set this peer as active
	SetLastPeer(peer)

	p.OnMessage(peer, webrtc.DataChannelMessage{Data: []byte("\x1b]11;?\x07")})

	require.Same(t, peer, mostRecentPeer, "active peer OSC pass-through should update last peer")
	require.Equal(t, []byte("\x1b]11;?\x07"), tty.written, "active peer OSC should reach PTY")
}

// TestOnMessageMultiplePeersOSCNoAmplification verifies the core fix: when
// two different peers send OSC 11 queries, only the active peer's query
// reaches the PTY; the other is dropped.
func TestOnMessageMultiplePeersOSCNoAmplification(t *testing.T) {
	resetActivePeer(t)
	peerA := &Peer{}
	peerB := &Peer{}
	logger := zaptest.NewLogger(t).Sugar()
	peerA.logger = logger
	peerB.logger = logger
	tty := &mockTTY{}

	// pane belongs to peerA; peerB is a second client sharing the pane
	p := &Pane{
		peer:   peerA,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// Simulate: peerA typed something first (becoming active)
	p.OnMessage(peerA, webrtc.DataChannelMessage{Data: []byte("ls\r")})
	require.Equal(t, []byte("ls\r"), tty.written)
	require.Same(t, peerA, GetActivePeer())
	tty.written = nil

	// Now peerA sends OSC 11 — active peer, should pass through
	p.OnMessage(peerA, webrtc.DataChannelMessage{Data: []byte("\x1b]11;?\x07")})
	require.Equal(t, []byte("\x1b]11;?\x07"), tty.written, "active peer OSC should reach PTY")
	tty.written = nil

	// peerB sends OSC 11 — NOT active peer, should be dropped
	// (peerA is still active because only pass-through updates SetLastPeer)
	require.Same(t, peerA, GetActivePeer())
	p.OnMessage(peerB, webrtc.DataChannelMessage{Data: []byte("\x1b]11;?\x07")})
	require.Empty(t, tty.written, "inactive peer OSC should be dropped")
}

// TestOnMessageReconnectingPeerUsesSenderNotPanePeer verifies that when
// peerB reconnects to a pane created by peerA, the active-peer tracking
// uses the sender (peerB), not pane.peer (peerA).
func TestOnMessageReconnectingPeerUsesSenderNotPanePeer(t *testing.T) {
	resetActivePeer(t)
	peerA := &Peer{}
	peerB := &Peer{}
	logger := zaptest.NewLogger(t).Sugar()
	peerA.logger = logger
	peerB.logger = logger
	tty := &mockTTY{}

	// Pane was created by peerA, but peerB reconnects
	p := &Pane{
		peer:   peerA,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// peerB types — should become active (not peerA)
	p.OnMessage(peerB, webrtc.DataChannelMessage{Data: []byte("ls\r")})
	require.Same(t, peerB, mostRecentPeer, "sender peerB should be set as active, not pane.peer peerA")
	require.Equal(t, []byte("ls\r"), tty.written)
}

// --- Output-side OSC query stripping tests ---

// TestFilterPTYOutputStripsOSC11Query verifies that OSC 11 color queries in
// PTY output are stripped and a response is written back to the PTY.
func TestFilterPTYOutputStripsOSC11Query(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	// PTY output containing an OSC 11 query with BEL terminator
	input := []byte("hello\x1b]11;?\x07world")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, []byte("helloworld"), filtered, "OSC 11 query should be stripped")
	// PTY should have received the synthesized response
	require.Contains(t, string(tty.written), "\x1b]11;rgb:0000/0000/0000\x1b\\")
}

// TestFilterPTYOutputStripsOSC10Query verifies that OSC 10 color queries in
// PTY output are stripped and a response is written back to the PTY.
func TestFilterPTYOutputStripsOSC10Query(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	// OSC 10 query with ST terminator
	input := []byte("\x1b]10;?\x1b\\")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "OSC 10 query should be fully stripped")
	require.Contains(t, string(tty.written), "\x1b]10;rgb:ffff/ffff/ffff\x1b\\")
}

// TestFilterPTYOutputStripsMultipleQueries verifies multiple queries in one
// read are all stripped.
func TestFilterPTYOutputStripsMultipleQueries(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	input := []byte("\x1b]11;?\x07\x1b]10;?\x07")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "all OSC queries should be stripped")
}

// TestFilterPTYOutputNoQueryPassesThrough verifies normal output is unchanged.
func TestFilterPTYOutputNoQueryPassesThrough(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	input := []byte("hello world\x1b[2J")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, input, filtered, "non-OSC output should be unchanged")
	require.Empty(t, tty.written, "no response should be written to PTY")
}

// TestFilterPTYOutputQueryInMiddleOfData verifies that a query embedded in
// the middle of other data is stripped correctly while surrounding data
// is preserved.
func TestFilterPTYOutputQueryInMiddleOfData(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	input := []byte("before\x1b]11;?\x07after")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, []byte("beforeafter"), filtered)
}

// TestFilterPTYOutputIncompleteOSCNotStripped verifies that an incomplete
// OSC sequence (no terminator found) is left in the data.
func TestFilterPTYOutputIncompleteOSCNotStripped(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	// OSC 11 query without terminator — should be left alone
	input := []byte("hello\x1b]11;?")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, input, filtered, "incomplete OSC should not be stripped")
	require.Empty(t, tty.written, "no response for incomplete OSC")
}

// TestFilterPTYOutputIncompleteOSCNoQuestionMark verifies that a partial OSC
// sequence ending at the `;` with no `?` character does not cause a panic
// (out-of-bounds access).  This is a realistic scenario when the PTY read
// splits in the middle of an escape sequence.
func TestFilterPTYOutputIncompleteOSCNoQuestionMark(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	// OSC 11 query prefix without '?' — should not panic, should pass through
	input := []byte("hello\x1b]11;")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, input, filtered, "incomplete OSC without '?' should pass through")
	require.Empty(t, tty.written, "no response for incomplete OSC")
}

// TestFilterPTYOutputNonColorOSCPassesThrough verifies that OSC sequences
// that are not 10/11 color queries are not stripped.
func TestFilterPTYOutputNonColorOSCPassesThrough(t *testing.T) {
	tty := &mockTTY{}
	p := &Pane{
		TTY: tty,
	}

	// OSC 12 (cursor color) — should not be stripped
	input := []byte("\x1b]12;?\x07")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, input, filtered, "non-10/11 OSC should pass through")
}

// --- Output-side CSI query stripping tests ---

// TestFilterPTYOutputStripsCSI6n verifies that a CSI 6n (cursor position)
// query in PTY output is stripped and a CPR response is written back to the PTY.
func TestFilterPTYOutputStripsCSI6n(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("\x1b[6n")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "CSI 6n should be stripped")
	require.NotEmpty(t, tty.written, "CPR response should be written to PTY")
	require.Equal(t, byte(0x1b), tty.written[0])
	require.Equal(t, byte('['), tty.written[1])
}

// TestFilterPTYOutputStripsCSI5n verifies that a CSI 5n (device status)
// query in PTY output is stripped and \x1b[0n is written back to the PTY.
func TestFilterPTYOutputStripsCSI5n(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("\x1b[5n")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "CSI 5n should be stripped")
	require.Equal(t, []byte("\x1b[0n"), tty.written)
}

// TestFilterPTYOutputStripsCSIC verifies that a CSI c (primary DA) query
// in PTY output is stripped and the DA response is written back to the PTY.
func TestFilterPTYOutputStripsCSIC(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("\x1b[c")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "CSI c should be stripped")
	require.Equal(t, []byte("\x1b[?1;2c"), tty.written)
}

// TestFilterPTYOutputStripsCSIGreaterC verifies that a CSI >c (secondary DA)
// query in PTY output is stripped and the secondary DA response is written
// back to the PTY.
func TestFilterPTYOutputStripsCSIGreaterC(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("\x1b[>c")
	filtered := p.filterPTYOutput(input)
	require.Empty(t, filtered, "CSI >c should be stripped")
	require.Equal(t, []byte("\x1b[>0;115;0c"), tty.written)
}

// TestFilterPTYOutputCSIMidData verifies that a CSI query embedded in other
// output is stripped correctly while surrounding data is preserved.
func TestFilterPTYOutputCSIMidData(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("before\x1b[6nafter")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, []byte("beforeafter"), filtered, "surrounding data should be preserved")
	require.NotEmpty(t, tty.written, "CPR response should be written to PTY")
}

// TestFilterPTYOutputMixedOSCAndCSI verifies that both OSC and CSI queries
// in a single read are handled correctly.
func TestFilterPTYOutputMixedOSCAndCSI(t *testing.T) {
	tty := &mockTTY{}
	p := newTestPane(t)
	p.TTY = tty

	input := []byte("x\x1b]11;?\x07y\x1b[6nz")
	filtered := p.filterPTYOutput(input)
	require.Equal(t, []byte("xyz"), filtered, "both queries should be stripped, text preserved")
	// PTY should have received both responses
	require.Contains(t, string(tty.written), "\x1b]11;rgb:0000/0000/0000\x1b\\")
	// Also should contain a CPR response (starts with \x1b[ and ends with R)
	written := string(tty.written)
	cprIdx := -1
	for i := 0; i < len(written); i++ {
		if i+2 < len(written) && written[i] == 0x1b && written[i+1] == '[' {
			// Check it ends with 'R' (CPR) rather than 'n' or 'c'
			for j := i + 2; j < len(written); j++ {
				if written[j] == 'R' {
					cprIdx = i
					break
				}
				if written[j] == 0x1b {
					break
				}
			}
		}
		if cprIdx >= 0 {
			break
		}
	}
	require.GreaterOrEqual(t, cprIdx, 0, "PTY should receive a CPR response")
}

// --- Input-side OSC response filtering tests ---

func TestIsOSCColorResponse(t *testing.T) {
	// Valid responses
	require.True(t, isOSCColorResponse([]byte("\x1b]11;rgb:0000/0000/0000\x07")))
	require.True(t, isOSCColorResponse([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\")))
	require.True(t, isOSCColorResponse([]byte("\x1b]10;rgb:ffff/ffff/ffff\x07")))
	// Not a response (it's a query — has '?')
	require.False(t, isOSCColorResponse([]byte("\x1b]11;?\x07")))
	require.False(t, isOSCColorResponse([]byte("\x1b]10;?\x07")))
	// Wrong OSC number
	require.False(t, isOSCColorResponse([]byte("\x1b]12;rgb:0000/0000/0000\x07")))
	// Too short
	require.False(t, isOSCColorResponse([]byte("\x1b]11;")))
}

func TestInterceptQueryOSCColorResponseFromActivePeer(t *testing.T) {
	resetActivePeer(t)
	p := newTestPane(t)

	SetLastPeer(p.peer)

	resp, handled := p.interceptQuery(p.peer, []byte("\x1b]11;rgb:0000/0000/0000\x07"))
	require.False(t, handled, "active peer OSC response should pass through to PTY")
	require.Nil(t, resp)
}

func TestInterceptQueryOSCColorResponseFromInactivePeer(t *testing.T) {
	resetActivePeer(t)
	p := newTestPane(t)

	// No active peer — OSC response should be dropped
	resp, handled := p.interceptQuery(p.peer, []byte("\x1b]11;rgb:0000/0000/0000\x07"))
	require.True(t, handled, "non-active peer OSC response should be dropped")
	require.Empty(t, resp)
}

// TestOnMessageMultiplePeersOSCResponseNoAmplification verifies that when
// multiple clients send OSC 11 color responses, only the active peer's
// response reaches the PTY.
func TestOnMessageMultiplePeersOSCResponseNoAmplification(t *testing.T) {
	resetActivePeer(t)
	peerA := &Peer{}
	peerB := &Peer{}
	logger := zaptest.NewLogger(t).Sugar()
	peerA.logger = logger
	peerB.logger = logger
	tty := &mockTTY{}

	p := &Pane{
		peer:   peerA,
		outbuf: make(chan []byte, OutBufSize),
		TTY:    tty,
	}

	// peerA becomes active via real input
	p.OnMessage(peerA, webrtc.DataChannelMessage{Data: []byte("ls\r")})
	tty.written = nil

	// peerA sends OSC 11 response — active, should pass through
	resp := []byte("\x1b]11;rgb:0000/0000/0000\x07")
	p.OnMessage(peerA, webrtc.DataChannelMessage{Data: resp})
	require.Equal(t, resp, tty.written, "active peer response should reach PTY")
	tty.written = nil

	// peerB sends OSC 11 response — not active, should be dropped
	p.OnMessage(peerB, webrtc.DataChannelMessage{Data: resp})
	require.Empty(t, tty.written, "inactive peer response should be dropped")
}
