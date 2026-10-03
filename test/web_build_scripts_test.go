package test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// packageJSON represents the package.json fragment needed for build script assertions.
type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

// legacyBuildConfiguration contains the safety properties of the actual Vite configuration.
type legacyBuildConfiguration struct {
	OutDir      string `json:"outDir"`
	EmptyOutDir bool   `json:"emptyOutDir"`
}

// readLegacyBuildConfiguration evaluates the real dependency-free factory for one theme.
func readLegacyBuildConfiguration(t *testing.T, root, theme string) legacyBuildConfiguration {
	t.Helper()
	node, err := exec.LookPath("node")
	require.NoError(t, err, "Node.js is required to validate frontend build configuration")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const script = `
import path from 'node:path';
import { pathToFileURL } from 'node:url';
const [root, theme] = process.argv.slice(1);
const { legacyConfig } = await import(pathToFileURL(path.join(root, 'web/legacy/vite-config.mjs')));
const config = legacyConfig({
  root: path.join(root, 'web', theme), mode: 'production', runtime: {},
  react: () => ({ name: 'react' }), svgr: () => ({ name: 'svgr' }),
  transformWithOxc: () => { throw new Error('The configuration check must not transform application code'); },
});
process.stdout.write(JSON.stringify(config.build));
`
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, root, theme)
	command.Dir = root
	output, err := command.CombinedOutput()
	require.NoError(t, err, "evaluate %s build configuration: %s", theme, output)

	var configuration legacyBuildConfiguration
	require.NoError(t, json.Unmarshal(output, &configuration), "parse %s build configuration", theme)
	return configuration
}

// TestLegacyThemesCleanupBuildOutput preserves clean, isolated outputs after the CRA-to-Vite migration.
func TestLegacyThemesCleanupBuildOutput(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("..")
	require.NoError(t, err, "resolve repository root")

	for _, theme := range []string{"air", "berry"} {
		t.Run(theme, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(root, "web", theme, "package.json"))
			require.NoError(t, err, "read %s package.json", theme)
			var pkg packageJSON
			require.NoError(t, json.Unmarshal(raw, &pkg), "parse %s package.json", theme)
			for _, name := range []string{"build", "build:dev", "build:prod"} {
				require.Contains(t, pkg.Scripts[name], "yarn lint", "%s/%s must retain lint validation", theme, name)
				require.Contains(t, pkg.Scripts[name], "vite build", "%s/%s must use the validated Vite configuration", theme, name)
				require.NotContains(t, pkg.Scripts[name], "react-scripts", "%s/%s must not restore CRA", theme, name)
			}

			configuration := readLegacyBuildConfiguration(t, root, theme)
			require.Equal(t, filepath.Join(root, "web", "build", theme), configuration.OutDir,
				"build output must remain inside the selected theme, never the shared build root")
			require.True(t, configuration.EmptyOutDir,
				"Vite must remove stale artifacts before writing the selected theme's output")
		})
	}
}
