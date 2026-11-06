package anthropic

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/relay/model"
)

// convertToolResultMediaBlocks rebuilds the content of a Claude tool_result
// block from the ordered parts a Chat tool message carries in ToolResultParts.
// Text parts become text blocks (empty ones are dropped because Claude rejects
// them) and image_url parts become base64 image blocks through the shared,
// size-limited and SSRF-guarded image fetcher used for user-message images.
// Parameters: parts is the tool message's ToolResultParts.
// Returns: the blocks, or nil when the parts carry no image so the caller keeps
// the historical string content; an error when an image cannot be loaded.
func convertToolResultMediaBlocks(parts []model.MessageContent) ([]Content, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	blocks := make([]Content, 0, len(parts))
	images := 0
	for index, part := range parts {
		switch part.Type {
		case model.ContentTypeText:
			if part.Text != nil && *part.Text != "" {
				blocks = append(blocks, Content{Type: "text", Text: *part.Text})
			}
		case model.ContentTypeImageURL:
			if part.ImageURL == nil || part.ImageURL.Url == "" {
				continue
			}
			mimeType, data, err := image.GetImageFromUrl(part.ImageURL.Url)
			if err != nil {
				return nil, errors.Wrapf(err, "validation failed: load tool result image part %d", index)
			}
			blocks = append(blocks, Content{
				Type:   "image",
				Source: &ImageSource{Type: "base64", MediaType: mimeType, Data: data},
			})
			images++
		}
	}
	if images == 0 {
		return nil, nil
	}
	return blocks, nil
}
