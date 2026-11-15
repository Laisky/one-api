package geminiOpenaiCompatible

import (
	"fmt"
	"strings"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// GetRequestURL returns the compatible endpoint for the translated request mode.
func GetRequestURL(meta *meta.Meta) (string, error) {
	baseURL := strings.TrimSuffix(meta.BaseURL, "/")
	requestPath := strings.TrimPrefix(meta.RequestURLPath, "/v1")
	path, _, _ := strings.Cut(requestPath, "?")
	if meta.Mode == relaymode.ClaudeMessages || (meta.Mode == relaymode.ChatCompletions && path == "/messages") {
		// Claude ingress is converted to Chat Completions, not a Messages API.
		requestPath = "/chat/completions"
	}
	return fmt.Sprintf("%s%s", baseURL, requestPath), nil
}
