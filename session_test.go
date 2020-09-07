// +build ignore

package main

import (
	"io"
	"log"
	"os/exec"
	"reflect"
	"testing"
)

func TestSessionStart(t *testing.T) {
	cmd := exec.Command("./webexec", "session", "start")
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Panicf("failed to open cmd stdin: %v", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Panicf("failed to open cmd stdout: %v", err)
	}
	cmd.Start()
	in.Write([]byte("e 1 80x24 cat <<EOF\n"))
	in.Write([]byte("i 1 World\nEOF\n"))
	b := make([]byte, 1024)
	var r []byte
	for {
		l, e := out.Read(b)
		if e == io.EOF {
			break
		}
		if l > 0 {
			if string(b[:4]) == "o 1 " {
				r = append(r, b[4:l]...)
			} else {
				t.Fatalf("got wrong message suffix: %q", string(b[:l]))
			}
		}
	}
	if reflect.DeepEqual(r, []byte("Hello\nWorld")) {
		t.Fatalf("got wrong stdout: %v", r)
	}
}

func TestResize(t *testing.T) {
	out := bytes.NewBufferString("")
	s := SessionServer{123, out}

	s.Process("e 123 34x56 echo $(tput cols)x$(tput lines)")
	if out.String() != "o 123 34x56" {
		t.Fail()
	}
	out.Reset()
	// now let's resize
	s.Process("r 123 12x78")
	s.Process("i 123 echo $(tput cols)x$(tput lines)")
	if out.String() != "o 123 12x78" {
		t.Fail()
	}
}
func TestKill(t *testing.T) {
	out := bytes.NewBufferString("")
	s := SessionServer{123, out}

	// start a shell
	s.Process("e 123 34x56")
	s.Process("k 123")
	// How do we test the process is killed?
}
