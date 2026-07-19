package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
)

// KeyRing holds the versioned AES-256 keys used to encrypt state payloads before
// they are written to Redis. Writes always use the newest key; reads try the key
// named by the ciphertext's version prefix, so a rotation can proceed while old
// records remain readable (SEC02).
//
// Keys come from RESPONSE_STATE_ENCRYPTION_KEYS, or — when that is unset — are
// derived from an EXPLICITLY configured SESSION_SECRET (DeriveKeyRingFromSecret).
// An auto-generated per-boot SESSION_SECRET must never be used, because it would
// orphan durable ciphertext after a restart (Section 5.4); the caller guarantees
// the secret was set by the operator before deriving from it.
type KeyRing struct {
	primary keyEntry
	byVer   map[string]keyEntry
}

// makeKeyEntry builds one AES-GCM key entry from a raw key (16/24/32 bytes).
func makeKeyEntry(version string, rawKey []byte) (keyEntry, error) {
	switch len(rawKey) {
	case 16, 24, 32:
	default:
		return keyEntry{}, errors.Errorf("state: encryption key %q must be 16/24/32 bytes, got %d", version, len(rawKey))
	}
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return keyEntry{}, errors.Wrapf(err, "state: build cipher for key %q", version)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return keyEntry{}, errors.Wrapf(err, "state: build gcm for key %q", version)
	}
	return keyEntry{version: version, key: rawKey, gcm: gcm}, nil
}

// DeriveKeyRingFromSecret builds a single-key KeyRing from an operator-configured
// SESSION_SECRET. It is safe ONLY because the caller passes the explicitly set
// value (config.SessionSecretEnvValue), which is stable across restarts; an
// auto-generated per-boot secret must never reach here (Section 5.4). The AES-256
// key is sha256(secret), and the version is derived from the secret so rotating
// SESSION_SECRET yields a NEW key version — old ciphertext then fails cleanly with
// "unknown key version" instead of being silently mis-decrypted.
func DeriveKeyRingFromSecret(secret string) (*KeyRing, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("state: cannot derive encryption key from an empty SESSION_SECRET")
	}
	sum := sha256.Sum256([]byte(secret))
	key := make([]byte, 32)
	copy(key, sum[:])
	version := "sess-" + hex.EncodeToString(sum[:4])
	entry, err := makeKeyEntry(version, key)
	if err != nil {
		return nil, errors.Wrap(err, "state: build session-secret-derived key entry")
	}
	return &KeyRing{primary: entry, byVer: map[string]keyEntry{version: entry}}, nil
}

type keyEntry struct {
	version string
	key     []byte
	gcm     cipher.AEAD
}

// ParseKeyRing parses the RESPONSE_STATE_ENCRYPTION_KEYS specification. Each entry
// is "<version>:<base64-key>"; entries are separated by commas or whitespace and
// ordered newest-first. A key must base64-decode to 16, 24, or 32 bytes (AES-128
// / 192 / 256); 32 bytes is recommended. An empty spec yields an error so the
// feature cannot silently enable without encryption.
func ParseKeyRing(spec string) (*KeyRing, error) {
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return nil, errors.New("state: no encryption keys configured")
	}

	ring := &KeyRing{byVer: make(map[string]keyEntry, len(fields))}
	for i, field := range fields {
		sep := strings.IndexByte(field, ':')
		if sep <= 0 || sep == len(field)-1 {
			return nil, errors.Errorf("state: encryption key entry %d is not <version>:<base64-key>", i)
		}
		version := field[:sep]
		rawKey, err := base64.StdEncoding.DecodeString(field[sep+1:])
		if err != nil {
			return nil, errors.Wrapf(err, "state: decode encryption key %q", version)
		}
		if _, dup := ring.byVer[version]; dup {
			return nil, errors.Errorf("state: duplicate encryption key version %q", version)
		}
		entry, err := makeKeyEntry(version, rawKey)
		if err != nil {
			return nil, errors.Wrapf(err, "state: build encryption key entry %d", i)
		}
		ring.byVer[version] = entry
		if i == 0 {
			ring.primary = entry
		}
	}
	return ring, nil
}

// PrimaryVersion returns the version label of the key used for new writes.
func (r *KeyRing) PrimaryVersion() string { return r.primary.version }

// Encrypt seals plaintext with the primary key and returns a self-describing
// "<version>:<base64(nonce||ciphertext)>" token.
func (r *KeyRing) Encrypt(plaintext []byte) (string, error) {
	if r == nil || r.primary.gcm == nil {
		return "", errors.New("state: key ring not initialized")
	}
	nonce := make([]byte, r.primary.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Wrap(err, "state: read nonce")
	}
	sealed := r.primary.gcm.Seal(nil, nonce, plaintext, nil)
	payload := append(nonce, sealed...)
	return r.primary.version + ":" + base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt opens a token produced by Encrypt, selecting the key by its version
// prefix. An unknown version or a tampered payload is an error.
func (r *KeyRing) Decrypt(token string) ([]byte, error) {
	if r == nil {
		return nil, errors.New("state: key ring not initialized")
	}
	sep := strings.IndexByte(token, ':')
	if sep <= 0 {
		return nil, errors.New("state: malformed ciphertext token")
	}
	version := token[:sep]
	entry, ok := r.byVer[version]
	if !ok {
		return nil, errors.Errorf("state: unknown encryption key version %q", version)
	}
	payload, err := base64.StdEncoding.DecodeString(token[sep+1:])
	if err != nil {
		return nil, errors.Wrap(err, "state: decode ciphertext")
	}
	nonceSize := entry.gcm.NonceSize()
	if len(payload) < nonceSize {
		return nil, errors.New("state: ciphertext too short")
	}
	nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
	plaintext, err := entry.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "state: decrypt payload")
	}
	return plaintext, nil
}
