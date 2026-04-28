package openai

import (
	"fmt"
	"strings"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

func ResponseText2Usage(responseText string, modelName string, promptTokens int) *model.Usage {
	usage := &model.Usage{}
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = CountTokenText(responseText, modelName)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}

func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	if channelType == channeltype.OpenAICompatible {
		trimmedBase := strings.TrimRight(baseURL, "/")
		path := requestURL
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if strings.HasSuffix(trimmedBase, "/v1") {
			// Preserve legacy custom-channel behaviour: if the stored base already contains /v1,
			// avoid duplicating the segment. Otherwise leave the path untouched so providers that
			// expect /v1 in the request keep working.
			path = strings.TrimPrefix(path, "/v1")
			if path == "" {
				path = "/"
			}
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
		}
		return trimmedBase + path
	}
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

	if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
		switch channelType {
		case channeltype.OpenAI:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/v1"))
		case channeltype.Azure:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/openai/deployments"))
		}
	}
	return fullRequestURL
}
