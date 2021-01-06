package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClientsDB(t *testing.T) {
	db := NewClientsDB()
	require.NotNil(t, db)
}
