package controller

import (
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// chatProviderMetadataScope selects which gateway provider-metadata carriers the Chat destination accepts.
// Native Google routes (the Gemini API and every model Vertex dispatches to native GenerateContent,
// including non-"gemini-" IDs such as Gemma) consume every carrier. Google's OpenAI-compatible wire
// (the dedicated channel, Vertex OpenAI-compatible routes, or any channel whose base URL is a Google
// generative API host) accepts only extra_content.google. Every other provider receives none. The
// decision follows the channel type, the registered dispatch route and the base URL, never model-name
// heuristics. Parameters: meta describes the selected channel. Returns the destination scope.
func chatProviderMetadataScope(meta *metalib.Meta) relaymodel.ProviderMetadataScope {
	switch {
	case meta == nil:
		return relaymodel.ProviderMetadataNone
	case meta.APIType == apitype.Gemini,
		meta.APIType == apitype.VertexAI && vertexai.UsesNativeGemini(meta.ActualModelName):
		return relaymodel.ProviderMetadataNative
	case meta.ChannelType == channeltype.GeminiOpenAICompatible,
		meta.APIType == apitype.VertexAI,
		gemini.IsGoogleGenerativeAPIBaseURL(meta.BaseURL):
		return relaymodel.ProviderMetadataGoogleWire
	default:
		return relaymodel.ProviderMetadataNone
	}
}

// scopeChatRequestForDestination scopes the Chat request's provider metadata to the selected channel.
// The request object is kept (adaptors normalize it in place and billing reads those fields); only
// its message and tool slices are replaced by clones when a carrier changes. Parameters: c carries
// the request logger, meta the selected channel and request the destination-bound request.
func scopeChatRequestForDestination(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest) {
	scope := chatProviderMetadataScope(meta)
	if count := relaymodel.ScopeChatRequestProviderMetadata(request, scope); count > 0 {
		lg := gmw.GetLogger(c)
		lg.Debug("scoped gateway provider metadata for chat destination",
			zap.String("scope", scope.String()),
			zap.Int("scoped_carriers", count),
		)
	}
}
