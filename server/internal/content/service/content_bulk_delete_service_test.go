package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanUniqueUUIDs(t *testing.T) {
	t.Run("dedupes while keeping order", func(t *testing.T) {
		ids, err := scanUniqueUUIDs([]string{
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222",
			"11111111-1111-1111-1111-111111111111",
		})
		require.NoError(t, err)
		require.Len(t, ids, 2)
		assert.True(t, ids[0].Valid)
		assert.True(t, ids[1].Valid)
	})

	t.Run("rejects a malformed id", func(t *testing.T) {
		_, err := scanUniqueUUIDs([]string{"not-a-uuid"})
		assert.Error(t, err)
	})
}
