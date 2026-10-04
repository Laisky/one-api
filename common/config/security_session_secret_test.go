package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestSecuritySessionConfigChild is entered only in a new process after actual
// package initialization; the parent never substitutes a validation function.
func TestSecuritySessionConfigChild(t *testing.T) {
	if os.Getenv("ONEAPI_SESSION_CONFIG_CHILD") != "1" {
		t.Skip("subprocess entrypoint")
	}
	require.NotEmpty(t, SessionSecret)
}

// TestSecuritySessionPlaceholderStartup exercises real initialization rather
// than claiming a string validator alone prevents unsafe runtime configuration.
func TestSecuritySessionPlaceholderStartup(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		reject      bool
	}{
		{"published_placeholder", "your-session-secret-here", true},
		{"legacy_example_placeholder", "random_string", true},
		{"uppercase_legacy_example", "RANDOM_STRING", true},
		{"trimmed_mixed_case_legacy_example", " \tRaNdOm_StRiNg \n", true},
		{"trimmed_placeholder", "  YOUR-SESSION-SECRET-HERE  ", true},
		{"generated_control", "pGPVdnHEyOsM2C83t6RIpTsDvkhjaN4d", false},
		{"legacy_key_control", "fixture-key-16ch", false},
		{"unset_random_control", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecuritySessionConfigChild$", "-test.count=1")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "SESSION_SECRET=") && !strings.HasPrefix(entry, "ONEAPI_SESSION_CONFIG_CHILD=") {
					command.Env = append(command.Env, entry)
				}
			}
			command.Env = append(command.Env, "ONEAPI_SESSION_CONFIG_CHILD=1")
			if tc.value != "" {
				command.Env = append(command.Env, "SESSION_SECRET="+tc.value)
			}
			output, err := command.CombinedOutput()
			if tc.reject {
				require.Error(t, err, "the documented public key must never reach application startup")
				require.Contains(t, string(output), "unsafe SESSION_SECRET placeholder")
			} else {
				require.NoError(t, err, string(output))
			}
		})
	}
}

// TestSecurityKubernetesSessionSecretWiring parses the shipped YAML blocks and
// checks that the application cannot select an executable public key default.
// This is manifest validation, not a claim that a Kubernetes cluster was deployed.
func TestSecurityKubernetesSessionSecretWiring(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "manuals", "k8s.md"))
	require.NoError(t, err)
	text := string(raw)
	require.NotContains(t, text, "SESSION_SECRET: 'your-session-secret-here'")
	found := false
	for _, block := range strings.Split(text, "```yaml\n")[1:] {
		source := strings.SplitN(block, "```", 2)[0]
		var object map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(source), &object))
		if object["kind"] == "ConfigMap" {
			if data, ok := object["data"].(map[string]any); ok {
				require.NotContains(t, data, "SESSION_SECRET")
			}
		}
		if object["kind"] != "Deployment" {
			continue
		}
		spec := object["spec"].(map[string]any)
		template := spec["template"].(map[string]any)["spec"].(map[string]any)
		for _, entry := range template["containers"].([]any) {
			container := entry.(map[string]any)
			if container["name"] != "one-api" {
				continue
			}
			for _, setting := range container["env"].([]any) {
				variable := setting.(map[string]any)
				if variable["name"] != "SESSION_SECRET" {
					continue
				}
				require.NotContains(t, variable, "value")
				ref := variable["valueFrom"].(map[string]any)["secretKeyRef"].(map[string]any)
				require.Equal(t, "one-api-session", ref["name"])
				require.Equal(t, "SESSION_SECRET", ref["key"])
				require.NotEqual(t, true, ref["optional"])
				found = true
			}
		}
	}
	require.True(t, found, "application deployment must reference a required session Secret")
	require.Contains(t, text, "openssl rand -base64 32")
	require.Contains(t, text, "RESPONSE_STATE_ENCRYPTION_KEYS", "rotation must not silently orphan state derived from the session key")
}
