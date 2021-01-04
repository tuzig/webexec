package main

import (
	"fmt"
	"github.com/hinshun/vt10x"
	"github.com/pion/webrtc/v2"
)

// STNew allocates a new simple terminal and returns it.
// caller should C.free the returned pointer
func STNew(cols int, rows int) (vt10x.VT, error) {
	t := vt10x.New()
	t.Resize(cols, rows)
	return t, nil
}

// STResize resizes a simple terminal
func STResize(t vt10x.VT, cols int, rows int) {
	Logger.Infof("Resizing to: %dx%d", rows, cols)
	t.Resize(cols, rows)
}

// STDump dumps a terminal buffer returning a byte slice and a len
func STDump(t vt10x.VT, d *webrtc.DataChannel) {
	t.Lock()
	defer t.Unlock()

	var view []byte
	var c rune
	rows, cols := t.Size()
	Logger.Infof("dumping scree size %dx%d", rows, cols)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c, _, _ = t.Cell(x, y)
			view = append(view, byte(c))
		}
		if y < rows-1 {
			view = append(view, byte('\n'))
			view = append(view, byte('\r'))
		}
		d.Send(view)
		view = nil
	}
	// position the cursor
	x, y := t.Cursor()
	Logger.Infof("Got cursor at: %d, %d", x, y)
	ps := fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
	d.Send([]byte(ps))
}

// STWrite writes a string to the simple terminal
func STWrite(t vt10x.VT, s string) {
	t.Write([]byte(s))
}
