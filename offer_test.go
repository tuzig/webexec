package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEncodeDecodeStringArray(t *testing.T) {
	Logger = zaptest.NewLogger(t).Sugar()

	a := []string{"Hello", "World"}
	b := EncodeOffer(a)
	Logger.Infof("encode offer to: %q", b)

	var c []string
	DecodeOffer(b, &c)
	require.Equal(t, a, c)
}
