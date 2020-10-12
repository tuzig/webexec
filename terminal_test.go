package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewTerminal(t *testing.T) {
	term := STNew(80, 24)
	require.NotNil(t, term)
}
func TestTerminalDump(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	logger.Info("Helllo")
	term := STNew(10, 2)

	STWrite(term, "a\n\rb\n\rc")
	/*
		for i := 65; i < 127 ; i++ {
			STPutc(i)
		}
	*/

	ret, l := STDump(term)
	require.Equal(t, 3, l)
	require.Equal(t, "b\nc", string(ret))
}
