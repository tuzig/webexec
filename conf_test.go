// This files contains testing suites that test the conf
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfEnv(t *testing.T) {
	initTest(t)
	require.EqualValues(t, Conf.peerConf.Env["TERM"], "xterm-256color")
	require.EqualValues(t, Conf.peerConf.Env["COLORTERM"], "truecolor")
}
