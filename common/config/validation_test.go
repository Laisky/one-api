package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGinMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"debug is valid", "debug", false},
		{"release is valid", "release", false},
		{"test is valid", "test", false},
		{"invalid mode", "production", true},
		{"case sensitive", "DEBUG", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGinMode(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAutoDetectAPIFormatAction(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"transparent is valid", "transparent", false},
		{"redirect is valid", "redirect", false},
		{"invalid action", "forward", true},
		{"empty is invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAutoDetectAPIFormatAction(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLogRotationInterval(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"hourly is valid", "hourly", false},
		{"daily is valid", "daily", false},
		{"weekly is valid", "weekly", false},
		{"monthly is invalid", "monthly", true},
		{"empty is invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogRotationInterval(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTheme(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"default is valid", "default", false},
		{"berry is valid", "berry", false},
		{"air is valid", "air", false},
		{"modern is valid", "modern", false},
		{"invalid theme", "dark", true},
		{"empty is invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTheme(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateGeminiSafetySetting(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"BLOCK_NONE is valid", "BLOCK_NONE", false},
		{"BLOCK_LOW_AND_ABOVE is valid", "BLOCK_LOW_AND_ABOVE", false},
		{"BLOCK_MEDIUM_AND_ABOVE is valid", "BLOCK_MEDIUM_AND_ABOVE", false},
		{"BLOCK_ONLY_HIGH is valid", "BLOCK_ONLY_HIGH", false},
		{"invalid setting", "BLOCK_ALL", true},
		{"lowercase is invalid", "block_none", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGeminiSafetySetting(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateGeminiVersion(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"v1 is valid", "v1", false},
		{"v1beta is valid", "v1beta", false},
		{"v2 is invalid", "v2", true},
		{"empty is invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGeminiVersion(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateOpenTelemetryConfig(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		endpoint string
		wantErr  bool
	}{
		{"disabled ignores endpoint", false, "", false},
		{"enabled requires endpoint", true, "", true},
		{"enabled with endpoint", true, "collector:4318", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOpenTelemetryConfig(tt.enabled, tt.endpoint)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidatePositiveInt(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"positive value", 100, false},
		{"one is valid", 1, false},
		{"zero is invalid", 0, true},
		{"negative is invalid", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePositiveInt("TEST_VAR", tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateNonNegativeInt(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"positive value", 100, false},
		{"zero is valid", 0, false},
		{"negative is invalid", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonNegativeInt("TEST_VAR", tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateIntRange(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"within range", 50, 0, 100, false},
		{"at minimum", 0, 0, 100, false},
		{"at maximum", 100, 0, 100, false},
		{"below minimum", -1, 0, 100, true},
		{"above maximum", 101, 0, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIntRange("TEST_VAR", tt.value, tt.min, tt.max)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateFloatRange(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		min     float64
		max     float64
		wantErr bool
	}{
		{"within range", 0.5, 0.0, 1.0, false},
		{"at minimum", 0.0, 0.0, 1.0, false},
		{"at maximum", 1.0, 0.0, 1.0, false},
		{"below minimum", -0.1, 0.0, 1.0, true},
		{"above maximum", 1.1, 0.0, 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFloatRange("TEST_VAR", tt.value, tt.min, tt.max)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateURLFormat(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"http URL is valid", "http://example.com", false},
		{"https URL is valid", "https://example.com", false},
		{"URL with path is valid", "https://example.com/api/v1", false},
		{"URL with port is valid", "http://localhost:3000", false},
		{"missing protocol is invalid", "example.com", true},
		{"ftp is invalid", "ftp://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURLFormat("TEST_VAR", tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTokenKeyPrefix(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"sk- is valid", "sk-", false},
		{"custom prefix is valid", "myapi-", false},
		{"single char is valid", "x", false},
		{"empty is invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenKeyPrefix(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidationError(t *testing.T) {
	t.Run("error with allowed values", func(t *testing.T) {
		err := &ConfigValidationError{
			Variable:    "TEST_VAR",
			Value:       "invalid",
			Constraint:  "must be valid",
			AllowedVals: []string{"a", "b", "c"},
		}
		errMsg := err.Error()
		require.Contains(t, errMsg, "TEST_VAR")
		require.Contains(t, errMsg, "invalid")
		require.Contains(t, errMsg, "allowed")
	})

	t.Run("error without allowed values", func(t *testing.T) {
		err := &ConfigValidationError{
			Variable:   "TEST_VAR",
			Value:      -5,
			Constraint: "must be positive",
		}
		errMsg := err.Error()
		require.Contains(t, errMsg, "TEST_VAR")
		require.Contains(t, errMsg, "-5")
		require.NotContains(t, errMsg, "allowed")
	})
}

func TestValidationResult(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		result := &ValidationResult{}
		require.False(t, result.HasErrors())
		require.Empty(t, result.Error())
	})

	t.Run("with errors", func(t *testing.T) {
		result := &ValidationResult{
			Errors: []error{
				&ConfigValidationError{Variable: "VAR1", Value: "bad", Constraint: "must be good"},
				&ConfigValidationError{Variable: "VAR2", Value: 0, Constraint: "must be positive"},
			},
		}
		require.True(t, result.HasErrors())
		errMsg := result.Error()
		require.Contains(t, errMsg, "VAR1")
		require.Contains(t, errMsg, "VAR2")
		require.Contains(t, errMsg, "configuration validation failed")
	})
}

func TestValidateAllEnvVars(t *testing.T) {
	// This test validates that the current configuration is valid
	// (since init() already ran and didn't panic)
	result := ValidateAllEnvVars()
	require.False(t, result.HasErrors(), "Current configuration should be valid: %s", result.Error())
}
