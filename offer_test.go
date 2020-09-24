package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeStringArray(t *testing.T) {
	InitDevLogger()

	a := []string{"Hello", "World"}
	b := EncodeOffer(a)
	Logger.Infof("encode offer to: %q", b)

	var c []string
	DecodeOffer(b, &c)
	require.Equal(t, a, c)
}
