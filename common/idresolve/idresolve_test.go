package idresolve

import (
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// TestResolveErrorKinds verifies that the sentinels are marked client-caused and
// that marking stayed transparent: identity via errors.Is and the exact message
// text must both survive, because callers match on them.
func TestResolveErrorKinds(t *testing.T) {
	t.Parallel()

	require.Equal(t, "invalid resource reference", ErrInvalidRef.Error())
	require.Equal(t, "resource reference not found", ErrNotFound.Error())
	require.Equal(t, errkind.InvalidRequest, errkind.Of(ErrInvalidRef))
	require.Equal(t, errkind.NotFound, errkind.Of(ErrNotFound))

	_, err := Resolve(nil, "abc")
	require.ErrorIs(t, err, ErrInvalidRef)
	require.Equal(t, errkind.InvalidRequest, errkind.Of(err))

	// Wrapped by Resolve itself: the kind must survive the errors.Wrap layer.
	_, err = Resolve(nil, "018f0000-0000-7000-8000-000000000003")
	require.ErrorIs(t, err, ErrInvalidRef)
	require.Equal(t, errkind.InvalidRequest, errkind.Of(err))

	_, err = Resolve(func(uuid string) (int, error) {
		return 0, gorm.ErrRecordNotFound
	}, "018f0000-0000-7000-8000-000000000004")
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, errkind.NotFound, errkind.Of(err))

	// A genuine lookup failure stays unclassified: it may be a database outage,
	// which must keep reaching ERROR.
	_, err = Resolve(func(uuid string) (int, error) {
		return 0, errors.New("dial tcp: connection refused")
	}, "018f0000-0000-7000-8000-000000000005")
	require.Equal(t, errkind.Unknown, errkind.Of(err))
}

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
