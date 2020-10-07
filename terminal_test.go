package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tuzig/webexec/config"
	"github.com/tuzig/webexec/terminal"
	"go.uber.org/zap/zaptest"
)

func TestTerminalDump(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	term := terminal.New(logger, &config.DefaultConfig)
	term.SetSize(80, 2)
	c := make(chan rune, 4096)
	c <- 'h'
	c <- 'e'
	c <- 'l'
	c <- 'l'

	go term.UpdateLoop(c)
	time.Sleep(time.Second / 10)
	buffer := term.ActiveBuffer()
	b := buffer.Dump()
	logger.Infof("got a dump: %v", b)
	require.Equal(t, 5, len(b))
	require.Equal(t, "hell\n", string(b))
}
