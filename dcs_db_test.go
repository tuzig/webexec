package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDCsDB(t *testing.T) {
	db := NewDCsDB()
	require.NotNil(t, db)
}
