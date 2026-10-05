package config

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// ValidateConfiguredSessionSecret rejects known public example values. An unset
// value retains the secure single-process random-key behavior; Kubernetes uses
// a required Secret reference so replicas cannot accidentally rely on it.
// This denylist is not an entropy estimator and does not prove arbitrary keys safe.
func ValidateConfiguredSessionSecret(value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "random_string", "your-session-secret-here", "your-session-secret", "your-secret-key", "your-secret-key-here", "changeme", "change-me", "change_me", "replace-me", "session-secret", "secret":
		return errors.New("unsafe SESSION_SECRET placeholder; provision a unique random secret")
	default:
		return nil
	}
}
