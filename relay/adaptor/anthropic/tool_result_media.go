package anthropic

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/relay/model"
)

// maxToolResultMediaBytes bounds the total base64 image bytes that tool
// results may add to one Claude request. Anthropic rejects request bodies over
// 32 MB, so a larger total could only fail upstream after every fetch.
const maxToolResultMediaBytes = 24 << 20

// toolResultMediaBudget tracks one Claude request's remaining tool-result image
// bytes and the request context whose cancellation stops further fetches.
type toolResultMediaBudget struct {
	ctx       context.Context
	remaining int
}

// newToolResultMediaBudget returns a budget of limit bytes bound to ctx; a nil
// ctx is treated as context.Background.
func newToolResultMediaBudget(ctx context.Context, limit int) *toolResultMediaBudget {
	if ctx == nil {
		ctx = context.Background()
	}
	return &toolResultMediaBudget{ctx: ctx, remaining: limit}
}

// check reports whether size more bytes fit and the request is still live. It
// returns a validation error when the request was cancelled or the images of
// all tool results would exceed the per-request byte limit.
func (b *toolResultMediaBudget) check(size int) error {
	if b == nil {
		return nil
	}
	if err := b.ctx.Err(); err != nil {
		return errors.Wrap(err, "validation failed: request cancelled while loading tool result images")
	}
	if size > b.remaining {
		return errors.Errorf("validation failed: tool result images exceed %d MiB per request", maxToolResultMediaBytes>>20)
	}
	return nil
}

// reserve charges size bytes against the budget after check succeeds. It
// returns the same errors as check.
func (b *toolResultMediaBudget) reserve(size int) error {
	if err := b.check(size); err != nil {
		return err
	}
	if b != nil {
		b.remaining -= size
	}
	return nil
}

// convertToolResultMediaBlocks rebuilds the content of a Claude tool_result
// block from the ordered parts a Chat tool message carries in ToolResultParts.
// Text parts become text blocks (empty ones are dropped because Claude rejects
// them) and image_url parts become base64 image blocks through the shared,
// size-limited and SSRF-guarded image fetcher used for user-message images.
// Parameters: budget bounds the request's total image bytes and stops fetching
// once the request is cancelled; parts is the tool message's ToolResultParts.
// Returns: the blocks, or nil when the parts carry no image so the caller keeps
// the historical string content; an error when an image cannot be loaded or
// the budget is exhausted.
func convertToolResultMediaBlocks(budget *toolResultMediaBudget, parts []model.MessageContent) ([]Content, error) {
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
			// A data URL is already in memory; reject it before decoding when it
			// alone exceeds what is left. Remote images are charged after fetch.
			precheck := 0
			if strings.HasPrefix(part.ImageURL.Url, "data:") {
				precheck = len(part.ImageURL.Url)
			}
			if err := budget.check(precheck); err != nil {
				return nil, errors.Wrapf(err, "load tool result image part %d", index)
			}
			mimeType, data, err := image.GetImageFromUrl(part.ImageURL.Url)
			if err != nil {
				return nil, errors.Wrapf(err, "validation failed: load tool result image part %d", index)
			}
			if err := budget.reserve(len(data)); err != nil {
				return nil, errors.Wrapf(err, "load tool result image part %d", index)
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
