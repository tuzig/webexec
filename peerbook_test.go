package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetICEServers(t *testing.T) {
	initTest(t)
	servers, err := getICEServers("127.0.0.1:17777")
	require.Nil(t, err)
	require.Equal(t, 1, len(servers), fmt.Sprintf("%v", servers))
}
