package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTerminal(t *testing.T) {
	term, err := STNew(80, 24)
	require.Nil(t, err)
	require.NotNil(t, term)
}
