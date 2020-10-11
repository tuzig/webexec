package main

// #include <stdlib.h>
// #include "st.h"
import "C"
import "log"

func STNew(col int, row int) {
	term := C.tnew(C.int(col), C.int(row))
	log.Printf("term %p", term)
}

func STResize(col int, row int) {
	C.tresize(C.int(col), C.int(row))
}

func STDump() []byte {
	buf := C.malloc(16536)
	defer C.free(buf)

	l := C.tdump2buf((*C.char)(buf))
	log.Printf("%d: %v", l, buf)
	return C.GoBytes(buf, l)
}

func STPutc(r rune) {
	C.tputc(C.uint(r))
}
func STWrite(s string) {
	for _, r := range s {
		STPutc(r)
	}
}
