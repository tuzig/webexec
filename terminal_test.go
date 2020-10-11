package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestTerminalDump(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	logger.Info("Helllo")
	STNew(10, 2)

	STWrite("a\n\rb\n\rc")
	/*
		for i := 65; i < 127 ; i++ {
			STPutc(i)
		}
	*/

	ret := STDump()
	require.Equal(t, "b\nc", string(ret))
}
