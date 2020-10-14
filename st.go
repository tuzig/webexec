package main

// #include <stdlib.h>
// #include "st.h"
import "C"

import (
	"unsafe"
)

// STDumpContext is used to pass context between C & go
type STDumpContext struct {
	PaneID int
	dcIdx  int
}

// STNew allocates a new simple terminal and returns it.
// caller should C.free the returned pointer
func STNew(col uint16, row uint16) *C.Term {
	return C.tnew(C.int(col), C.int(row))
}

// STResize resizes a simple terminal
func STResize(t *C.Term, col uint16, row uint16) {
	C.tresize(t, C.int(col), C.int(row))
}

//export goSTDumpCB
func goSTDumpCB(buf *C.char, l C.int, context unsafe.Pointer) {
	// This is the function called from the C world by our expensive
	// C.somelib_get_files() function. The userdata value contains an instance
	// of *progressRequest, We unpack it and use it's values to call the
	// actual function that our user supplied.
	c := (*STDumpContext)(context)
	Logger.Infof("Sending dump buf len %d with context %v\n", l, c)
	Panes[c.PaneID-1].dcs[c.dcIdx].Send(C.GoBytes((unsafe.Pointer)(buf), l))
}

// STDump dumps a terminal buffer returning a byte slice and a len
func STDump(t *C.Term, c *STDumpContext) int {
	p := unsafe.Pointer(c)
	r := C.tdump2cb(t, p)
	return int(r)
}

// STPutc output a rune on the terminal
func STPutc(t *C.Term, r rune) {
	C.tputc(t, C.uint(r))
}

// STWrite writes a string to the simple terminal
func STWrite(t *C.Term, s string) {
	for _, r := range s {
		STPutc(t, r)
	}
}
