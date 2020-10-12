package main

// #include <stdlib.h>
// #include "st.h"
import "C"

import (
	"log"
	"unsafe"
)

func STFree(t *C.Term) {
	C.free(unsafe.Pointer(t))
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

// STDump dumps a terminal buffer returning a byte slice and a len
func STDump(t *C.Term) ([]byte, int) {
	buf := C.malloc(16536)
	defer C.free(buf)

	l := C.tdump2buf(t, (*C.char)(buf))
	log.Printf("%d: %v", l, buf)
	return C.GoBytes(buf, l), int(l)
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
