package relay

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/apitype"
)

// TestJinaAdaptorFactory verifies registration and pricing work before request initialization.
func TestJinaAdaptorFactory(t *testing.T) {
	t.Parallel()
	a := GetAdaptor(apitype.Jina)
	require.IsType(t, &jina.Adaptor{}, a)
	require.Len(t, a.GetDefaultModelPricing(), 25)
	require.InDelta(t, 0.025, a.GetModelRatio("jina-embeddings-v3"), 1e-12)
}
