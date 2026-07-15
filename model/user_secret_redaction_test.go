package model

import (
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// TestUserZapLoggingRedactsSecrets_T16 proves the secret-confinement net (G3/T16)
// extends to structured logging: zap.Any("user", user) serializes the struct
// through the JSON encoder's reflection path (encoding/json), so the json:"-"
// tags on Password/AccessToken/TotpSecret/VerificationCode keep a bcrypt hash or
// TOTP seed out of the logs even if someone logs a whole User by accident.
func TestUserZapLoggingRedactsSecrets_T16(t *testing.T) {
	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey: "msg",
	})
	u := User{
		Id:               7,
		UUID:             "018f0000-0000-7000-8000-00000000f001",
		Username:         "redaction-user",
		Password:         "BCRYPT_SECRET_VALUE",
		AccessToken:      "ACCESS_SECRET_VALUE",
		TotpSecret:       "TOTP_SECRET_VALUE",
		VerificationCode: "VERIFY_SECRET_VALUE",
	}
	buf, err := enc.EncodeEntry(zapcore.Entry{Message: "user"}, []zapcore.Field{zap.Any("user", u)})
	require.NoError(t, err)
	out := buf.String()

	require.NotContains(t, out, "BCRYPT_SECRET_VALUE", "password must never reach the logs")
	require.NotContains(t, out, "ACCESS_SECRET_VALUE", "access_token must never reach the logs")
	require.NotContains(t, out, "TOTP_SECRET_VALUE", "totp_secret must never reach the logs")
	require.NotContains(t, out, "VERIFY_SECRET_VALUE", "verification_code must never reach the logs")

	// Sanity: non-secret fields still serialize (rules out a false positive where
	// nothing was encoded at all).
	require.Contains(t, out, "redaction-user")
}
