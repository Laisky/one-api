package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// withInitializedOptionMap ensures config.OptionMap is non-nil and restores any
// previous state when the test exits. updateOptionMap writes to OptionMap,
// which is normally seeded by InitOptionMap during application startup.
func withInitializedOptionMap(t *testing.T) {
	t.Helper()

	config.OptionMapRWMutex.Lock()
	originalMap := config.OptionMap
	config.OptionMap = make(map[string]string)
	config.OptionMapRWMutex.Unlock()

	originalWhitelist := config.EmailDomainWhitelist

	t.Cleanup(func() {
		config.OptionMapRWMutex.Lock()
		config.OptionMap = originalMap
		config.OptionMapRWMutex.Unlock()
		config.EmailDomainWhitelist = originalWhitelist
	})
}

func TestUpdateOptionMapEmailDomainWhitelistEmptyString(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", ""))
	require.Empty(t, config.EmailDomainWhitelist,
		"empty value must reset whitelist so len(...) == 0 and the restriction check disables itself")
	require.Nil(t, config.EmailDomainWhitelist,
		"empty value should set the slice to nil rather than [\"\"]")
}

func TestUpdateOptionMapEmailDomainWhitelistWhitespaceOnly(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", "  "))
	require.Empty(t, config.EmailDomainWhitelist)
	require.Nil(t, config.EmailDomainWhitelist)
}

func TestUpdateOptionMapEmailDomainWhitelistSingleDomain(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", "a.com"))
	require.Equal(t, []string{"a.com"}, config.EmailDomainWhitelist)
}

func TestUpdateOptionMapEmailDomainWhitelistMultipleDomains(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", "a.com,b.com"))
	require.Equal(t, []string{"a.com", "b.com"}, config.EmailDomainWhitelist)
}

func TestUpdateOptionMapEmailDomainWhitelistSkipsEmptyEntries(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", "a.com,,b.com"))
	require.Equal(t, []string{"a.com", "b.com"}, config.EmailDomainWhitelist)
}

func TestUpdateOptionMapEmailDomainWhitelistTrimsWhitespace(t *testing.T) {
	withInitializedOptionMap(t)

	require.NoError(t, updateOptionMap("EmailDomainWhitelist", " a.com , b.com "))
	require.Equal(t, []string{"a.com", "b.com"}, config.EmailDomainWhitelist)
}

func TestUpdateOptionReturnsDatabaseErrors(t *testing.T) {
	withInitializedOptionMap(t)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	prevDB := DB
	DB = db
	t.Cleanup(func() {
		DB = prevDB
	})

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = UpdateOption("SystemName", "Broken")
	require.Error(t, err)
	require.Contains(t, err.Error(), "first or create option")

	config.OptionMapRWMutex.RLock()
	_, exists := config.OptionMap["SystemName"]
	config.OptionMapRWMutex.RUnlock()
	require.False(t, exists, "in-memory options must not change when persistence fails")
}

func TestUpdateOptionMapEmailEnvOverridesPersistedValues(t *testing.T) {
	withInitializedOptionMap(t)
	t.Setenv(config.EnvEmailProvider, "SMTP")
	t.Setenv(config.EnvResendAPIKey, "re_env")

	prevProvider := config.EmailProvider
	prevKey := config.ResendAPIKey
	t.Cleanup(func() {
		config.EmailProvider = prevProvider
		config.ResendAPIKey = prevKey
	})

	require.NoError(t, updateOptionMap("EmailProvider", "resend"))
	require.Equal(t, "smtp", config.EmailProvider)

	require.NoError(t, updateOptionMap("ResendAPIKey", ""))
	require.Equal(t, "re_env", config.ResendAPIKey)

	config.OptionMapRWMutex.RLock()
	require.Equal(t, "smtp", config.OptionMap["EmailProvider"])
	require.Equal(t, "re_env", config.OptionMap["ResendAPIKey"])
	config.OptionMapRWMutex.RUnlock()
}
