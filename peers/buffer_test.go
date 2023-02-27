package peers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddMarkRestore(t *testing.T) {
	buf := NewBuffer(10)
	buf.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	buf.Mark(1)
	buf.Mark(2)
	buf.Add([]byte{9, 10, 11, 12})
	ret := buf.GetSinceMarker(1)
	require.Equal(t, len(ret), 4)
	require.Equal(t, ret[0], byte(9))
	require.Equal(t, ret[1], byte(10))
	require.Equal(t, ret[2], byte(11))
	require.Equal(t, ret[3], byte(12))
	buf.Add([]byte{13, 14, 15, 16, 17, 18, 19, 20})
	ret = buf.GetSinceMarker(2)
	require.Equal(t, len(ret), 10)
	require.Equal(t, ret[0], byte(11))
}
