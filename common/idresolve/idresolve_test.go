package idresolve

import (
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestResolve verifies UUID lookups, malformed inputs, legacy integer rejection, and not-found mapping.
func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("legacy integer rejected", func(t *testing.T) {
		t.Parallel()

		_, err := Resolve(func(uuid string) (int, error) {
			t.Fatalf("lookup should not be called for integer refs")
			return 0, nil
		}, "42")

		require.ErrorIs(t, err, ErrInvalidRef)
	})

	t.Run("uuid", func(t *testing.T) {
		t.Parallel()

		id, err := Resolve(func(uuid string) (int, error) {
			require.Equal(t, "018f0000-0000-7000-8000-000000000001", uuid)
			return 99, nil
		}, "018f0000-0000-7000-8000-000000000001")

		require.NoError(t, err)
		require.Equal(t, 99, id)
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		_, err := Resolve(nil, "abc")
		require.ErrorIs(t, err, ErrInvalidRef)

		_, err = Resolve(nil, "")
		require.ErrorIs(t, err, ErrInvalidRef)
	})

	t.Run("unknown uuid", func(t *testing.T) {
		t.Parallel()

		_, err := Resolve(func(uuid string) (int, error) {
			return 0, errors.Wrap(gorm.ErrRecordNotFound, "missing")
		}, "018f0000-0000-7000-8000-000000000002")

		require.ErrorIs(t, err, ErrNotFound)
	})
}
