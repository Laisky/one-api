package logcursor

// Tests for the sealed pagination cursor (W2.4 step 3).
//
// The load-bearing properties are that the anchor row id never appears on the
// wire, that no mutation of a token is ever accepted, and that a cursor cannot
// be moved between authorization scopes.

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCursor returns a cursor valid at the given instant.
//
// Parameters:
//   - now: the issue time.
//   - rowID: the anchor row id.
//
// Return values:
//   - Cursor: the cursor.
//   - [32]byte: its filter digest.
func testCursor(now time.Time, rowID int64) (Cursor, [32]byte) {
	var digest [32]byte
	for i := range digest {
		digest[i] = byte(i + 1)
	}
	return Cursor{
		CreatedAt:    1757030400,
		RowID:        rowID,
		FilterDigest: digest,
		IssuedAt:     now.Unix(),
		ExpiresAt:    now.Add(30 * time.Minute).Unix(),
	}, digest
}

// TestCursorRoundTrip verifies a sealed cursor opens back to the same values.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 987654321)

	token, err := Seal(cursor, "self", 42, "log.self")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, tokenPrefix))

	got, err := Open(token, "self", 42, "log.self", digest, now)
	require.NoError(t, err)
	require.Equal(t, cursor.CreatedAt, got.CreatedAt)
	require.Equal(t, cursor.RowID, got.RowID)
	require.Equal(t, cursor.FilterDigest, got.FilterDigest)
}

// TestCursorDoesNotLeakRowID verifies the internal primary key is not
// recoverable from the token.
//
// dto.LogResponse deliberately exposes no integer id; a signed-but-readable
// cursor would put it back on the API boundary.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorDoesNotLeakRowID(t *testing.T) {
	now := time.Now().UTC()
	const rowID = int64(1234567890123)
	cursor, _ := testCursor(now, rowID)

	token, err := Seal(cursor, "all", 0, "log.all")
	require.NoError(t, err)

	require.NotContains(t, token, strconv.FormatInt(rowID, 10),
		"the decimal row id must not appear in the token")

	raw, err := base64.RawURLEncoding.DecodeString(token[len(tokenPrefix):])
	require.NoError(t, err)
	require.NotContains(t, string(raw), strconv.FormatInt(rowID, 10))

	// The plaintext row id big-endian must not appear in the ciphertext either.
	plain := []byte{0, 0, 1, 31, 113, 251, 4, 203}
	require.NotContains(t, string(raw), string(plain))
}

// TestCursorRejectsEveryMutation verifies no single-bit change to a token is
// ever accepted, and that no mutation panics.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRejectsEveryMutation(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 55)
	token, err := Seal(cursor, "all", 0, "log.all")
	require.NoError(t, err)

	body := token[len(tokenPrefix):]
	raw, err := base64.RawURLEncoding.DecodeString(body)
	require.NoError(t, err)

	for i := range raw {
		for bit := range 8 {
			mutated := make([]byte, len(raw))
			copy(mutated, raw)
			mutated[i] ^= 1 << bit

			candidate := tokenPrefix + base64.RawURLEncoding.EncodeToString(mutated)
			require.NotPanics(t, func() {
				_, openErr := Open(candidate, "all", 0, "log.all", digest, now)
				require.Error(t, openErr, "a mutated token at byte %d bit %d must be rejected", i, bit)
			})
		}
	}
}

// TestCursorRejectsMalformedInput verifies structural rejections happen before
// any cryptography, and that oversized input is refused on length.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRejectsMalformedInput(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 7)
	token, err := Seal(cursor, "all", 0, "log.all")
	require.NoError(t, err)

	cases := map[string]string{
		"empty":             "",
		"prefix only":       tokenPrefix,
		"wrong prefix":      "lc9." + token[len(tokenPrefix):],
		"no prefix":         token[len(tokenPrefix):],
		"standard alphabet": tokenPrefix + base64.StdEncoding.EncodeToString([]byte("0123456789")),
		"whitespace":        tokenPrefix + " " + token[len(tokenPrefix):],
		"one mebibyte":      tokenPrefix + strings.Repeat("A", 1<<20),
		"sixty four kib":    tokenPrefix + strings.Repeat("A", 64<<10),
		"short sealed body": tokenPrefix + base64.RawURLEncoding.EncodeToString([]byte("too short")),
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Open(candidate, "all", 0, "log.all", digest, now)
			require.Error(t, err)
			var reject *RejectError
			require.ErrorAs(t, err, &reject)
			require.Equal(t, ReasonMalformed, reject.Reason)
		})
	}

	// Every truncated prefix of a valid token is rejected.
	for i := 1; i < len(token); i++ {
		_, err := Open(token[:i], "all", 0, "log.all", digest, now)
		require.Error(t, err, "truncation at %d must be rejected", i)
	}
}

// TestCursorCannotCrossScope verifies a cursor is unusable outside the exact
// authorization scope it was issued under.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorCannotCrossScope(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 11)

	token, err := Seal(cursor, "self", 42, "log.self")
	require.NoError(t, err)

	for name, open := range map[string]func() (Cursor, error){
		"replayed by another user": func() (Cursor, error) {
			return Open(token, "self", 43, "log.self", digest, now)
		},
		"escalated to site scope": func() (Cursor, error) {
			return Open(token, "all", 0, "log.self", digest, now)
		},
		"moved to the admin route": func() (Cursor, error) {
			return Open(token, "self", 42, "log.all", digest, now)
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := open()
			require.Error(t, err, "a cursor must never be usable outside its issuing scope")
		})
	}
}

// TestCursorExpiry verifies lifetime enforcement, including a bounded tolerance
// for a peer clock that is slightly ahead.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorExpiry(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 3)
	token, err := Seal(cursor, "all", 0, "log.all")
	require.NoError(t, err)

	_, err = Open(token, "all", 0, "log.all", digest, now.Add(31*time.Minute))
	var reject *RejectError
	require.ErrorAs(t, err, &reject)
	require.Equal(t, ReasonExpired, reject.Reason)

	// Issued 30 seconds in the future: inside the skew allowance.
	future, futureDigest := testCursor(now.Add(30*time.Second), 4)
	futureToken, err := Seal(future, "all", 0, "log.all")
	require.NoError(t, err)
	_, err = Open(futureToken, "all", 0, "log.all", futureDigest, now)
	require.NoError(t, err)

	// Issued ten minutes in the future: outside it.
	far, farDigest := testCursor(now.Add(10*time.Minute), 5)
	farToken, err := Seal(far, "all", 0, "log.all")
	require.NoError(t, err)
	_, err = Open(farToken, "all", 0, "log.all", farDigest, now)
	require.ErrorAs(t, err, &reject)
	require.Equal(t, ReasonExpired, reject.Reason)
}

// TestCursorRejectsChangedQuery verifies a cursor stops working when the
// request's normalized query changes.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRejectsChangedQuery(t *testing.T) {
	now := time.Now().UTC()
	cursor, digest := testCursor(now, 9)
	token, err := Seal(cursor, "all", 0, "log.all")
	require.NoError(t, err)

	other := digest
	other[0] ^= 0xff

	_, err = Open(token, "all", 0, "log.all", other, now)
	var reject *RejectError
	require.ErrorAs(t, err, &reject)
	require.Equal(t, ReasonQueryChanged, reject.Reason)
}
