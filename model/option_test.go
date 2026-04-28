package model

import (
	"testing"

	"github.com/stretchr/testify/require"

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
