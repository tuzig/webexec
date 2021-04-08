// This files contains testing suites that test the conf
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecCommand(t *testing.T) {
	initTest(t)
	c := []string{"bash", "-c", "echo $TERM $COLORTERM"}
	_, tty, err := execCommand(c, nil)
	b := make([]byte, 64)
	l, err := tty.Read(b)
	require.Nil(t, err)
	require.Less(t, 14, l)
	require.Equal(t, "xterm truecolor", string(b[:15]))
}
