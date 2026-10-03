package xai

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportedModelListContainsRefreshedCatalog(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"grok-4.7", "grok-4.3-latest", "grok-build-latest"} {
		assert.True(t, slices.Contains(ModelList, name), name)
	}
}
