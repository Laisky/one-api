package openai

import (
	"context"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/claudevision"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// claudeImageReservationModel resolves an Azure deployment alias for admission only.
// Canonical target model IDs and non-Azure channels keep their own image policy.
func claudeImageReservationModel(ctx context.Context, name string) string {
	if ctx == nil || claudevision.IsSonnet55(name) {
		return name
	}
	m, ok := ctx.Value(ctxkey.Meta).(*meta.Meta)
	if c, found := ctx.Value(gmw.CtxKeyGin).(*gin.Context); found && c != nil {
		if value, found := c.Get(ctxkey.Meta); found {
			m, ok = value.(*meta.Meta)
		}
	}
	if pricing.IsGlobalPricingInitialized() {
		if _, known := pricing.GetGlobalModelConfig(name); known {
			return name
		}
	}
	if ok && m != nil && m.ChannelType == channeltype.Azure &&
		name == m.ActualModelName &&
		claudevision.IsSonnet55(m.OriginModelName) {
		return m.OriginModelName
	}
	return name
}

// countSonnet55FileImages reserves one image allowance for each opaque file part.
// This is not an estimate of all pages of a PDF; invalid inputs remain provider errors.
func countSonnet55FileImages(content any) int {
	count := 0
	switch blocks := content.(type) {
	case []any:
		for _, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := block["type"].(string)
			if kind != model.ContentTypeFile {
				continue
			}
			id, _ := block["file_id"].(string)
			data, _ := block["file_data"].(string)
			if strings.TrimSpace(id) != "" || strings.TrimSpace(data) != "" {
				count++
			}
		}
	case []model.MessageContent:
		for _, block := range blocks {
			if block.Type == model.ContentTypeFile && (strings.TrimSpace(block.FileID) != "" || strings.TrimSpace(block.FileData) != "") {
				count++
			}
		}
	}
	return count
}
