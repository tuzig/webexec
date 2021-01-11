package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeStringArray(t *testing.T) {
	initTest(t)
	a := []string{"Hello", "World"}
	b := make([]byte, 4096)
	l, err := EncodeOffer(b, a)
	require.Nil(t, err, "Failed to encode offer: %s", err)
	Logger.Infof("encode offer to: %q", b[:l])

	c := make([]string, 2)
	err = DecodeOffer(&c, b[:l])
	require.Nil(t, err, "Failed to decode offer: %s", err)
	require.Equal(t, a, c)
}
