package anthropic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestToolResultMediaBudgetBoundsTotalBytes verifies that tool-result images
// share one per-request byte budget: images within it convert, and the image
// that would exceed it fails with a validation error instead of being loaded.
func TestToolResultMediaBudgetBoundsTotalBytes(t *testing.T) {
	t.Parallel()
	image := toolMediaImage("data:image/png;base64," + toolMediaPayload)
	_, probe, err := loadToolMediaPayloadSize(image)
	require.NoError(t, err)

	budget := newRequestMediaBudget(context.Background(), 2*probe+probe/2)
	blocks, err := convertToolResultMediaBlocks(budget, []model.MessageContent{image, image})
	require.NoError(t, err)
	require.Len(t, blocks, 2)

	_, err = convertToolResultMediaBlocks(budget, []model.MessageContent{image})
	require.Error(t, err, "a third image exceeds the shared budget")
	require.Contains(t, err.Error(), "validation failed")
	require.Contains(t, err.Error(), "per request")
}

// TestToolResultMediaBudgetStopsOnCancel verifies that no further image is
// loaded once the request context is cancelled.
func TestToolResultMediaBudgetStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := newRequestMediaBudget(ctx, maxRequestMediaBytes)
	_, err := convertToolResultMediaBlocks(budget, []model.MessageContent{toolMediaImage("data:image/png;base64," + toolMediaPayload)})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// loadToolMediaPayloadSize converts one image with an unlimited budget and
// returns the resulting block plus its base64 size, for budget arithmetic.
func loadToolMediaPayloadSize(part model.MessageContent) ([]Content, int, error) {
	blocks, err := convertToolResultMediaBlocks(newRequestMediaBudget(context.Background(), maxRequestMediaBytes), []model.MessageContent{part})
	if err != nil || len(blocks) == 0 || blocks[0].Source == nil {
		return nil, 0, err
	}
	size := len(blocks[0].Source.Data)
	if len("data:image/png;base64,"+toolMediaPayload) > size {
		size = len("data:image/png;base64," + toolMediaPayload)
	}
	return blocks, size, nil
}
