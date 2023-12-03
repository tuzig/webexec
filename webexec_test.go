package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetVersionNote(t *testing.T) {
	initTest(t)
	version = "0.0.1"
	versionNote := getVersionNote()
	require.NotEmpty(t, versionNote)
	version = cachedVersion.version.String()
	versionNote2 := getVersionNote()
	require.Empty(t, versionNote2)
}
