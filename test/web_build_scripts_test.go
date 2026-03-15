package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// packageJSON represents the package.json fragment needed for build script assertions.
type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

// TestCreateReactThemesCleanupBuildOutput ensures CRA themes clear target directories before moving build artifacts.
func TestCreateReactThemesCleanupBuildOutput(t *testing.T) {
	t.Parallel()

	themes := map[string]string{
		"berry": "../build/berry",
		"air":   "../build/air",
	}

	for theme, expectedTarget := range themes {
		expectedTarget := expectedTarget

		t.Run(theme, func(t *testing.T) {
			t.Parallel()

			pkgPath := filepath.Join("..", "web", theme, "package.json")
			raw, err := os.ReadFile(pkgPath)
			require.NoError(t, err, "failed to read package.json for %s", theme)

			var pkg packageJSON
			err = json.Unmarshal(raw, &pkg)
			require.NoError(t, err, "failed to parse package.json for %s", theme)

			script, ok := pkg.Scripts["build"]
			require.True(t, ok, "missing build script for %s", theme)

			require.Contains(t, script, "rm -rf "+expectedTarget, "build script for %s must clear %s before moving artifacts", theme, expectedTarget)
		})
	}
}
