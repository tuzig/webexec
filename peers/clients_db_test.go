package peers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientsDB(t *testing.T) {
	db := NewClientsDB()
	require.NotNil(t, db)
}
