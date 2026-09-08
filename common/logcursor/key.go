// Package logcursor seals and opens the opaque pagination cursors used by the
// additive keyset log-list routes (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// A cursor is SEALED, not merely signed. It carries the anchor row's primary
// key, and `logs.id` is an internal integer that dto.LogResponse deliberately
// never exposes: the project moved its external identifiers to UUIDs precisely
// so integer keys stay off the API boundary. A signed-but-readable cursor would
// put them back on it.
//
// A cursor is never an authorization grant. The handler re-derives the caller's
// scope on every request and passes it in as associated data; the sealed
// payload additionally carries a digest of the normalized query. Both are
// CHECKS that produce an explicit restart response, never inputs to the SQL. A
// perfectly forged cursor could therefore only move the anchor within rows the
// caller may already read.
package logcursor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// keyDerivationInfo domain-separates the cursor key from every other key the
// project derives from the same secret.
const keyDerivationInfo = "one-api/log-cursor/key/v1"

var (
	sealerOnce sync.Once
	sealerAEAD cipher.AEAD
	sealerErr  error
	// stableKey records whether the key survives a restart, which decides
	// whether a cursor issued by one node or boot is usable by another.
	stableKey bool
)

// aead returns the process cipher, deriving it once.
//
// The key comes from config.SessionSecretEnvValue, the RAW environment value.
// config.SessionSecret must not be used: it is normalized in config's init and
// then overwritten again by common.Init, so a key derived from it depends on
// when the derivation ran. relay/state/bootstrap.go sets the same precedent.
//
// When no explicit SESSION_SECRET exists the secret is regenerated every boot,
// so a per-process random key is used instead. Cursors are then node-local and
// a cursor presented to another node or after a restart fails its seal and
// yields an ordinary restart-required response. That is correct for the default
// single-node deployment and is warned about at startup.
//
// Parameters: none.
//
// Return values:
//   - cipher.AEAD: the sealing primitive.
//   - error: wrapped failure when the cipher cannot be built.
func aead() (cipher.AEAD, error) {
	sealerOnce.Do(func() {
		secret := strings.TrimSpace(config.SessionSecretEnvValue)
		var ikm []byte
		if secret != "" {
			ikm = []byte(secret)
			stableKey = true
		} else {
			ikm = make([]byte, 32)
			if _, err := rand.Read(ikm); err != nil {
				sealerErr = errors.Wrap(err, "logcursor: generate per-process cursor key")
				return
			}
			logger.Logger.Warn("log cursors are node-local: SESSION_SECRET is not set, so the cursor key "+
				"is regenerated every boot",
				zap.String("effect", "a cursor presented after a restart, or to another node behind a "+
					"load balancer, returns restart_required"),
				zap.String("fix", "set SESSION_SECRET to a stable value"))
		}

		key, err := hkdf.Key(sha256.New, ikm, nil, keyDerivationInfo, 32)
		if err != nil {
			sealerErr = errors.Wrap(err, "logcursor: derive cursor key")
			return
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			sealerErr = errors.Wrap(err, "logcursor: build cursor cipher")
			return
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			sealerErr = errors.Wrap(err, "logcursor: build cursor gcm")
			return
		}
		sealerAEAD = gcm
	})
	return sealerAEAD, sealerErr
}

// KeyIsStable reports whether cursors survive a restart and work across nodes.
//
// Parameters: none.
//
// Return values:
//   - bool: true when an explicit SESSION_SECRET backs the cursor key.
func KeyIsStable() bool {
	_, _ = aead()
	return stableKey
}

// resetForTest rebuilds the process cipher, for tests that vary the secret.
//
// Parameters: none.
//
// Return values: none.
func resetForTest() {
	sealerOnce = sync.Once{}
	sealerAEAD = nil
	sealerErr = nil
	stableKey = false
}
