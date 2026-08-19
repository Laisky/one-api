package openai

import (
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// ResponseText2Usage creates a Usage struct from response text by estimating completion tokens.
func ResponseText2Usage(responseText string, modelName string, promptTokens int) *model.Usage {
	usage := &model.Usage{}
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = CountTokenText(responseText, modelName)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}

// GetFullRequestURL constructs the full request URL for OpenAI and compatible APIs,
// handling version suffix deduplication to avoid paths like /v4/v1/chat/completions.
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	trimmedBase := adaptor.NormalizeBaseURL(baseURL)
	path := adaptor.NormalizeRequestPath(requestURL)

	if channelType == channeltype.OpenAICompatible {
		if adaptor.HasVersionSuffix(trimmedBase) {
			// Preserve legacy custom-channel behaviour: if the stored base already contains a version
			// suffix (e.g. /v1, /v4, /v1beta), strip /v1 from the request path to avoid duplication.
			path = adaptor.StripOpenAIV1Prefix(path, "/")
		}
		return adaptor.JoinBaseURLAndPath(trimmedBase, path)
	}
	fullRequestURL := adaptor.JoinBaseURLAndPath(trimmedBase, path)

	if strings.HasPrefix(trimmedBase, "https://gateway.ai.cloudflare.com") {
		switch channelType {
		case channeltype.OpenAI:
			fullRequestURL = adaptor.JoinBaseURLAndPath(trimmedBase, adaptor.StripOpenAIV1Prefix(path, ""))
		case channeltype.Azure:
			fullRequestURL = adaptor.JoinBaseURLAndPath(trimmedBase, strings.TrimPrefix(path, "/openai/deployments"))
		}
	}
	return fullRequestURL
}
