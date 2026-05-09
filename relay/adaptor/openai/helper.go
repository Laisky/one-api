package openai

import (
	"fmt"
	"regexp"
	"strings"

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

// openaiVersionSuffixRe matches base URLs ending with a version segment like /v1, /v4, /v1beta, etc.
var openaiVersionSuffixRe = regexp.MustCompile(`/v\d+[a-zA-Z0-9]*$`)

// GetFullRequestURL constructs the full request URL for OpenAI and compatible APIs,
// handling version suffix deduplication to avoid paths like /v4/v1/chat/completions.
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	if channelType == channeltype.OpenAICompatible {
		trimmedBase := strings.TrimRight(baseURL, "/")
		path := requestURL
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if openaiVersionSuffixRe.MatchString(trimmedBase) {
			// Preserve legacy custom-channel behaviour: if the stored base already contains a version
			// suffix (e.g. /v1, /v4, /v1beta), strip /v1 from the request path to avoid duplication.
			if path == "/v1" {
				path = "/"
			} else if strings.HasPrefix(path, "/v1/") {
				path = path[len("/v1"):]
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
