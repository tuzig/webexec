package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetVersionNote(t *testing.T) {
	versionNote := getVersionNote()
	fmt.Println(versionNote)
	require.NotEmpty(t, versionNote)
}
