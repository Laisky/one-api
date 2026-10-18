package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestAppLogExplicitContentLimitInput accepts zero/unlimited and positive
// operator budgets, while negative values remain invalid configuration.
func TestAppLogExplicitContentLimitInput(t *testing.T) {
	for _, name := range []string{EnvAppLogOTLPMaxAttributes, EnvAppLogOTLPMaxAttributeValueBytes} {
		for _, value := range []string{"0", "1", "4096"} {
			require.Empty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(map[string]string{name: value})), name+"="+value)
		}
		require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(map[string]string{name: "-1"})), name)
	}
}
