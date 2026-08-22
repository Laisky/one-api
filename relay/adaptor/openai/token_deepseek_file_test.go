package openai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestCountTokenMessages_DeepSeekFileImagesUseDocumentedUpperBound verifies
// file-backed image parts reserve the same documented upper bound as URL images.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when a file image is omitted from estimation.
func TestCountTokenMessages_DeepSeekFileImagesUseDocumentedUpperBound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	modelName := "deepseek-v4-flash-vision-exp"
	base := CountTokenMessages(ctx, []model.Message{
		{Role: "user", Content: "describe this image"},
	}, modelName)

	for name, source := range map[string]map[string]any{
		"file_id":   {"file_id": "file-api-123"},
		"file_data": {"file_data": "data:image/png;base64,AAAA"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			content := []any{
				map[string]any{"type": "text", "text": "describe this image"},
			}
			fileBlock := map[string]any{"type": "file"}
			for key, value := range source {
				fileBlock[key] = value
			}
			content = append(content, fileBlock)
			got := CountTokenMessages(ctx, []model.Message{{Role: "user", Content: content}}, modelName)
			require.Equal(t, base+deepseekV4VisionMaxImageTokens, got)
		})
	}
}
