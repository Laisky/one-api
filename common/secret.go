package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/config"
)

const secretMask = "******"

// MaskSecret returns a masked placeholder for secrets.
func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	return secretMask
}

// IsMaskedSecret reports whether the supplied value is a masked placeholder.
func IsMaskedSecret(value string) bool {
	return value == secretMask
}

// EncryptSecret encrypts a sensitive value using AES-GCM and a key derived from SessionSecret.
func EncryptSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	key := deriveSecretKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.Wrap(err, "create cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.Wrap(err, "create gcm")
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Wrap(err, "read nonce")
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	payload := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptSecret decrypts a value encrypted by EncryptSecret.
func DecryptSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	payload, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", errors.Wrap(err, "decode secret")
	}

	key := deriveSecretKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.Wrap(err, "create cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.Wrap(err, "create gcm")
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("secret payload too short")
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.Wrap(err, "decrypt secret")
	}

	return string(plaintext), nil
}

// deriveSecretKey returns a stable 32-byte key derived from SessionSecret.
func deriveSecretKey() []byte {
	secret := config.SessionSecret
	if secret == "" {
		secret = "one-api-default-secret"
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
