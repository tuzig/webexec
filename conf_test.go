// This files contains testing suites that test the conf
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfEnv(t *testing.T) {
	initTest(t)
	require.EqualValues(t, Conf.env["TERM"], "xterm")
	require.EqualValues(t, Conf.env["COLORTERM"], "truecolor")
}
