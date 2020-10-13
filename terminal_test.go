package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTerminal(t *testing.T) {
	term := STNew(80, 24)
	require.NotNil(t, term)
}
