package relay_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCatalogReviewInventory prevents an adaptor directory from disappearing
// from the dated review ledger. It checks coverage, not upstream availability;
// evidence limitations are explicit dispositions rather than guessed prices.
func TestCatalogReviewInventory(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filepath.Dir(file))
	entries, err := os.ReadDir(filepath.Join(root, "relay", "adaptor"))
	require.NoError(t, err)
	var directories []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		switch entry.Name() {
		case "common", "internal", "openai_compatible":
			continue
		}
		directories = append(directories, entry.Name())
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "research", "20260924_adaptor_model_catalog_audit.md"))
	require.NoError(t, err)
	pattern := regexp.MustCompile("(?m)^\\| `([A-Za-z0-9_]+)` \\|")
	var reviewed []string
	for _, match := range pattern.FindAllSubmatch(text, -1) {
		reviewed = append(reviewed, string(match[1]))
	}
	require.ElementsMatch(t, directories, reviewed, "every catalog surface needs exactly one disposition")
}
