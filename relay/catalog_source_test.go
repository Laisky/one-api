package relay_test

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProductionCatalogSources checks that maintaining models requires Go code
// and tests, not embedded catalogs, a patch loader, or a generation stage.
func TestProductionCatalogSources(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filepath.Dir(file))
	for _, relative := range []string{"scripts/model_catalog", "relay/adaptor/internal/catalogsnapshot", "docs/research/model_catalog_20260924"} {
		_, err := os.Stat(filepath.Join(root, relative))
		require.True(t, os.IsNotExist(err), relative)
	}
	for _, provider := range []string{"ali", "alibailian", "baichuan", "baiduv2", "deepinfra", "novita", "openrouter", "siliconflow", "togetherai"} {
		dir := filepath.Join(root, "relay", "adaptor", provider)
		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "catalog_") && strings.HasSuffix(f.Name(), ".json") {
				t.Fatalf("unexpected runtime catalog: %s/%s", provider, f.Name())
			}
			if strings.HasPrefix(f.Name(), "models_") && strings.HasSuffix(f.Name(), ".go") && !strings.HasSuffix(f.Name(), "_test.go") {
				raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
				require.NoError(t, err)
				require.NotContains(t, string(raw), "//go:generate")
				require.NotContains(t, string(raw), "DO NOT EDIT")
				require.LessOrEqual(t, strings.Count(string(raw), "\n"), 800)
			}
		}
	}
}
