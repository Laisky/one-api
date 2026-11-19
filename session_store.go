package main

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions/cookie"

	"github.com/Laisky/one-api/common/logger"
)

// newSessionStore builds the session cookie store from SESSION_SECRET.
//
// A non-base64 secret keeps the historical behaviour: it is used raw as the
// cookie authentication key, without encryption. A base64 secret that decodes
// to a valid AES key size (16, 24 or 32 bytes) is used as both the
// authentication and the encryption key, exactly as before. A base64 secret of
// any other length used to be passed to AES unchanged, so every login failed
// at runtime with "crypto/aes: invalid key size"; it now keeps the decoded
// bytes as the authentication key and derives a 32-byte encryption key with
// SHA-256, logging a warning so operators can rotate to a well-formed secret.
//
// Parameters: secret is the configured SESSION_SECRET. It returns the store.
func newSessionStore(secret string) cookie.Store {
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		logger.Logger.Info("session secret is not base64 encoded, using raw value instead")
		return cookie.NewStore([]byte(secret))
	}
	switch len(decoded) {
	case 16, 24, 32:
		return cookie.NewStore(decoded, decoded)
	default:
		logger.Logger.Warn("SESSION_SECRET decodes to a length that is not an AES key size (16, 24 or 32 bytes); deriving a 32-byte encryption key with SHA-256",
			zap.Int("decoded_bytes", len(decoded)))
		encryptionKey := sha256.Sum256(decoded)
		return cookie.NewStore(decoded, encryptionKey[:])
	}
}
