package state

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// withConfig snapshots and restores the response-state config vars around a test.
func withConfig(t *testing.T) {
	t.Helper()
	orig := struct {
		enabled   bool
		shadow    bool
		legacy    bool
		keys      string
		allowlist string
		secretEnv string
	}{
		enabled:   config.ResponseStateEnabled,
		shadow:    config.ResponseStateShadow,
		legacy:    config.ResponseStateLegacyPassthrough,
		keys:      config.ResponseStateEncryptionKeys,
		allowlist: config.ResponseStateAllowlist,
		secretEnv: config.SessionSecretEnvValue,
	}
	// Default to no SESSION_SECRET so key-material tests are deterministic; a test
	// that exercises the derivation sets it explicitly.
	config.SessionSecretEnvValue = ""
	t.Cleanup(func() {
		config.ResponseStateEnabled = orig.enabled
		config.ResponseStateShadow = orig.shadow
		config.ResponseStateLegacyPassthrough = orig.legacy
		config.ResponseStateEncryptionKeys = orig.keys
		config.ResponseStateAllowlist = orig.allowlist
		config.SessionSecretEnvValue = orig.secretEnv
		_ = initWithStore(func(*KeyRing) (ResponseStateStore, error) { return NewMemoryStore(DefaultLimits()), nil })
	})
}

func validKeySpec() string {
	return "1:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
}

// TestInitDisabledKeepsCurrentBehavior verifies the feature stays off and no
// store is installed when RESPONSE_STATE_ENABLED is false (row O01).
func TestInitDisabledKeepsCurrentBehavior(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = false
	config.ResponseStateEncryptionKeys = validKeySpec()

	require.NoError(t, initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return NewMemoryStore(DefaultLimits()), nil
	}))
	require.False(t, Enabled())
	require.Nil(t, Store())
}

// TestInitEnabledRequiresKeys verifies enabling without a stable encryption key
// is a startup error, never a silent downgrade.
func TestInitEnabledRequiresKeys(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateEncryptionKeys = ""

	err := initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return NewMemoryStore(DefaultLimits()), nil
	})
	require.Error(t, err)
	require.False(t, Enabled())
	require.Nil(t, Store())
}

// TestInitEnabledRequiresBackend verifies enabling surfaces a backend error (e.g.
// Redis unavailable) rather than degrading to an in-process store.
func TestInitEnabledRequiresBackend(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateEncryptionKeys = validKeySpec()

	err := initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return nil, ErrStoreUnavailable
	})
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.False(t, Enabled())
	require.Nil(t, Store())
}

// TestInitEnabledInstallsStore verifies a healthy backend installs the store and
// activates the toggles.
func TestInitEnabledInstallsStore(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateShadow = true
	config.ResponseStateLegacyPassthrough = true
	config.ResponseStateEncryptionKeys = validKeySpec()

	require.NoError(t, initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return NewMemoryStore(DefaultLimits()), nil
	}))
	require.True(t, Enabled())
	require.True(t, ShadowMode())
	require.True(t, LegacyPassthroughEnabled())
	require.NotNil(t, Store())
}

// TestAllowlistScoping verifies allowlist parsing and matching (row O03).
func TestAllowlistScoping(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateEncryptionKeys = validKeySpec()
	config.ResponseStateAllowlist = "user:5, token:9, channel:3"

	require.NoError(t, initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return NewMemoryStore(DefaultLimits()), nil
	}))
	require.True(t, AllowedFor(5, 1, 1))
	require.True(t, AllowedFor(1, 9, 1))
	require.True(t, AllowedFor(1, 1, 3))
	require.False(t, AllowedFor(1, 1, 1))
}

// TestInitDerivesKeyFromSessionSecret verifies that, with no
// RESPONSE_STATE_ENCRYPTION_KEYS, an explicitly configured SESSION_SECRET is used
// to derive a stable encryption key and the feature installs its store.
func TestInitDerivesKeyFromSessionSecret(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateEncryptionKeys = ""
	config.SessionSecretEnvValue = "an-explicit-stable-operator-secret"

	require.True(t, encryptionKeyMaterialPresent())
	require.NoError(t, initWithStore(func(ring *KeyRing) (ResponseStateStore, error) {
		require.NotNil(t, ring)
		// The derived ring round-trips ciphertext.
		token, err := ring.Encrypt([]byte("hello state"))
		require.NoError(t, err)
		out, err := ring.Decrypt(token)
		require.NoError(t, err)
		require.Equal(t, "hello state", string(out))
		return NewMemoryStore(DefaultLimits()), nil
	}))
	require.True(t, Enabled())
	require.NotNil(t, Store())
}

// TestDeriveKeyRingFromSecretIsStableAndSecretDependent verifies the derived key
// version is deterministic for a given secret and changes when the secret changes
// (so a rotated SESSION_SECRET fails cleanly rather than mis-decrypting).
func TestDeriveKeyRingFromSecretIsStableAndSecretDependent(t *testing.T) {
	a1, err := DeriveKeyRingFromSecret("secret-A")
	require.NoError(t, err)
	a2, err := DeriveKeyRingFromSecret("secret-A")
	require.NoError(t, err)
	b, err := DeriveKeyRingFromSecret("secret-B")
	require.NoError(t, err)
	require.Equal(t, a1.PrimaryVersion(), a2.PrimaryVersion())
	require.NotEqual(t, a1.PrimaryVersion(), b.PrimaryVersion())

	// A token sealed under secret-A cannot be opened by secret-B's ring.
	token, err := a1.Encrypt([]byte("x"))
	require.NoError(t, err)
	_, err = b.Decrypt(token)
	require.Error(t, err)

	_, err = DeriveKeyRingFromSecret("   ")
	require.Error(t, err)
}

// TestAutoEnableDecision verifies the auto-enable rule: with no explicit
// RESPONSE_STATE_ENABLED, the feature turns on only when BOTH a stable encryption
// key and Redis are present; an explicit setting always wins.
func TestAutoEnableDecision(t *testing.T) {
	// Not explicitly set: auto-enable iff key AND redis are both present.
	require.True(t, autoEnable(false, true, true))
	require.False(t, autoEnable(false, true, false))
	require.False(t, autoEnable(false, false, true))
	require.False(t, autoEnable(false, false, false))

	// Explicitly set: never auto-enable (the explicit value governs directly).
	require.False(t, autoEnable(true, true, true))
	require.False(t, autoEnable(true, false, false))
}

// TestDisabledReason verifies the startup log reason is specific and content-free.
func TestDisabledReason(t *testing.T) {
	require.Contains(t, disabledReason(true, true, true), "explicitly set to false")
	require.Contains(t, disabledReason(false, false, false), "encryption key")
	require.Contains(t, disabledReason(false, false, false), "Redis")
	require.Contains(t, disabledReason(false, false, true), "encryption key")
	require.Contains(t, disabledReason(false, true, false), "Redis")
}

// TestAllowlistEmptyMeansAll verifies an empty allowlist admits everyone once the
// feature is enabled.
func TestAllowlistEmptyMeansAll(t *testing.T) {
	withConfig(t)
	config.ResponseStateEnabled = true
	config.ResponseStateEncryptionKeys = validKeySpec()
	config.ResponseStateAllowlist = ""

	require.NoError(t, initWithStore(func(*KeyRing) (ResponseStateStore, error) {
		return NewMemoryStore(DefaultLimits()), nil
	}))
	require.True(t, AllowedFor(999, 999, 999))
}
