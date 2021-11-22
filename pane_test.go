// This files contains testing suites that test the conf
package main

import (
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
)

func TestExecCommand(t *testing.T) {
	initTest(t)
	c := []string{"bash", "-c", "echo $TERM $COLORTERM"}
	_, tty, err := execCommand(c, nil, 0)
	b := make([]byte, 64)
	l, err := tty.Read(b)
	require.Nil(t, err)
	require.Less(t, 14, l)
	require.Equal(t, "xterm truecolor", string(b[:15]))
}
func TestExecCommandWithParent(t *testing.T) {
	initTest(t)
	c := []string{"sh"}
	cmd, tty, err := execCommand(c, nil, 0)
	time.Sleep(time.Second / 100)
	_, err = tty.Write([]byte("cd /tmp\n"))
	require.Nil(t, err)
	_, err = tty.Write([]byte("pwd\n"))
	require.Nil(t, err)
	time.Sleep(time.Second / 10)
	cmd2, _, err := execCommand(c, nil, cmd.Process.Pid)
	require.Nil(t, err)
	p, err := process.NewProcess(int32(cmd2.Process.Pid))
	require.Nil(t, err)
	cwd, err := p.Cwd()
	require.Nil(t, err)
	require.Equal(t, "/private/tmp", cwd)
}
