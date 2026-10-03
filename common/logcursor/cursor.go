package logcursor

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
)

// tokenPrefix versions the wire form. A future format bumps it so an old token
// is rejected on a cheap string compare rather than a failed decryption.
const tokenPrefix = "lc1."

// Payload layout constants. Every field is fixed width, so a malformed token is
// rejected on length before any parsing.
const (
	payloadVersion = byte(1)

	offVersion      = 0
	offIssuedAt     = 1
	offExpiresAt    = 9
	offCreatedAt    = 17
	offRowID        = 25
	offFilterDigest = 33
	payloadLen      = offFilterDigest + 32 // 65 bytes

	nonceLen = 12
	tagLen   = 16

	// maxTokenLen bounds decoding work. The sealed token is a fixed 129 bytes
	// of binary, so 256 characters of base64 is generous headroom for the
	// prefix while still rejecting a megabyte of input on a length check.
	maxTokenLen = 256
)

// Cursor is the state a page hands to its successor.
type Cursor struct {
	// CreatedAt is the anchor row's created_at, in Unix seconds.
	CreatedAt int64
	// RowID is the anchor row's primary key. It never leaves the process
	// unsealed.
	RowID int64
	// FilterDigest binds the cursor to one normalized query and scope.
	FilterDigest [32]byte
	// IssuedAt and ExpiresAt bound the cursor's lifetime, in Unix seconds.
	IssuedAt  int64
	ExpiresAt int64
}

// RejectReason classifies why a cursor was refused, so the handler can answer
// with a stable machine-readable code without leaking which check failed to an
// attacker probing the seal.
type RejectReason string

const (
	// ReasonMalformed covers every structural failure: length, prefix,
	// encoding, version and seal.
	ReasonMalformed RejectReason = "cursor_invalid"
	// ReasonExpired means the cursor aged out.
	ReasonExpired RejectReason = "cursor_expired"
	// ReasonQueryChanged means the request no longer matches the query or scope
	// the cursor was issued for.
	ReasonQueryChanged RejectReason = "cursor_query_changed"
)

// RejectError carries a RejectReason to the handler.
type RejectError struct {
	// Reason is the machine-readable classification.
	Reason RejectReason
}

// Error implements the error interface.
//
// Parameters: none.
//
// Return values:
//   - string: the reason code.
func (e *RejectError) Error() string { return string(e.Reason) }

// associatedData binds a sealed cursor to an authorization scope.
//
// The scope is NOT stored in the cursor. It is supplied by the caller from the
// current request on both seal and open, so a cursor issued under one scope
// cannot be opened under another: the GCM tag check fails and the caller gets a
// restart response. This is what prevents a cursor from ever widening access.
//
// Parameters:
//   - scopeKind: "self" or "all".
//   - scopeUser: the subject user id; 0 for site-wide.
//   - endpoint: the route identifier.
//
// Return values:
//   - []byte: the associated data.
func associatedData(scopeKind string, scopeUser int, endpoint string) []byte {
	return []byte("logcursor.v1|" + scopeKind + "|" + strconv.Itoa(scopeUser) + "|" + endpoint)
}

// Seal encodes a cursor into its opaque wire form.
//
// Parameters:
//   - cursor: the cursor to seal; IssuedAt and ExpiresAt must already be set.
//   - scopeKind: the caller's scope kind.
//   - scopeUser: the caller's subject user id.
//   - endpoint: the route identifier.
//
// Return values:
//   - string: the opaque token.
//   - error: wrapped failure when the cipher or entropy source is unavailable.
func Seal(cursor Cursor, scopeKind string, scopeUser int, endpoint string) (string, error) {
	gcm, err := aead()
	if err != nil {
		return "", err
	}

	plaintext := make([]byte, payloadLen)
	plaintext[offVersion] = payloadVersion
	binary.BigEndian.PutUint64(plaintext[offIssuedAt:], uint64(cursor.IssuedAt))
	binary.BigEndian.PutUint64(plaintext[offExpiresAt:], uint64(cursor.ExpiresAt))
	binary.BigEndian.PutUint64(plaintext[offCreatedAt:], uint64(cursor.CreatedAt))
	binary.BigEndian.PutUint64(plaintext[offRowID:], uint64(cursor.RowID))
	copy(plaintext[offFilterDigest:], cursor.FilterDigest[:])

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.Wrap(err, "logcursor: generate nonce")
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, associatedData(scopeKind, scopeUser, endpoint))
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open decodes and validates a cursor.
//
// Checks run cheapest first, so a hostile caller cannot make the server perform
// decryption or allocation proportional to its input: length, then prefix, then
// base64, then the sealed length, then the AEAD open (whose tag comparison is
// constant time by construction), then version, then expiry, then the query
// digest.
//
// Parameters:
//   - token: the opaque token from the request.
//   - scopeKind: the scope kind derived from the CURRENT request.
//   - scopeUser: the subject user id derived from the CURRENT request.
//   - endpoint: the route identifier.
//   - expectDigest: the digest of the CURRENT normalized query and scope.
//   - now: the current time, for expiry.
//
// Return values:
//   - Cursor: the validated cursor.
//   - error: a *RejectError describing why the cursor was refused.
func Open(token, scopeKind string, scopeUser int, endpoint string, expectDigest [32]byte, now time.Time) (Cursor, error) {
	if len(token) == 0 || len(token) > maxTokenLen {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}
	if len(token) <= len(tokenPrefix) || token[:len(tokenPrefix)] != tokenPrefix {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}

	sealed, err := base64.RawURLEncoding.DecodeString(token[len(tokenPrefix):])
	if err != nil {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}
	if len(sealed) != nonceLen+payloadLen+tagLen {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}

	gcm, err := aead()
	if err != nil {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}

	plaintext, err := gcm.Open(nil, sealed[:nonceLen], sealed[nonceLen:],
		associatedData(scopeKind, scopeUser, endpoint))
	if err != nil {
		// A scope that no longer matches lands here too, because the scope is
		// associated data. Both are answered as an invalid cursor so the
		// response does not distinguish a forgery from a scope change.
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}
	if len(plaintext) != payloadLen || plaintext[offVersion] != payloadVersion {
		return Cursor{}, &RejectError{Reason: ReasonMalformed}
	}

	cursor := Cursor{
		IssuedAt:  int64(binary.BigEndian.Uint64(plaintext[offIssuedAt:])),
		ExpiresAt: int64(binary.BigEndian.Uint64(plaintext[offExpiresAt:])),
		CreatedAt: int64(binary.BigEndian.Uint64(plaintext[offCreatedAt:])),
		RowID:     int64(binary.BigEndian.Uint64(plaintext[offRowID:])),
	}
	copy(cursor.FilterDigest[:], plaintext[offFilterDigest:])

	unix := now.UTC().Unix()
	// A small future-skew allowance keeps a cursor issued by a peer whose clock
	// is marginally ahead from being rejected as not-yet-valid.
	const futureSkewSeconds = 60
	if cursor.IssuedAt > unix+futureSkewSeconds || cursor.ExpiresAt <= unix {
		return Cursor{}, &RejectError{Reason: ReasonExpired}
	}

	if subtle.ConstantTimeCompare(cursor.FilterDigest[:], expectDigest[:]) != 1 {
		return Cursor{}, &RejectError{Reason: ReasonQueryChanged}
	}

	return cursor, nil
}
