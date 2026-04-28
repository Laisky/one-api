package openai_compatible

import (
	"net/url"
	"strings"

	"github.com/Laisky/one-api/relay/relaymode"
)

const gitHubModelsHost = "models.github.ai"

// IsGitHubModelsBaseURL reports whether the provided base URL targets GitHub Models.
func IsGitHubModelsBaseURL(baseURL string) bool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return false
	}

	parsed, err := url.Parse(trimmed)
	var host string
	if err == nil {
		host = parsed.Host
		// url.Parse treats bare hosts without scheme as paths; handle that fallback.
		if host == "" {
			host = parsed.Path
		}
	} else {
		host = trimmed
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}

	// Remove scheme-less prefixes and any trailing path component.
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}

	host = strings.ToLower(strings.TrimSpace(host))
	return host == gitHubModelsHost
}

// NormalizeGitHubRequestPath rewrites OpenAI-style paths into GitHub Models inference paths.
// It preserves organization attribution segments and selects sensible defaults per relay mode.
func NormalizeGitHubRequestPath(path string, relayMode int) string {
	defaultPath := gitHubDefaultPathForMode(relayMode)

	cleaned := strings.TrimSpace(path)
	if cleaned == "" || cleaned == "/" {
		return defaultPath
	}

	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	cleaned = strings.TrimRight(cleaned, "/")

	lower := strings.ToLower(cleaned)

	if strings.Contains(lower, "/inference/") || lower == "/inference" {
		return cleaned
	}

	switch lower {
	case "/v1", "/v1/chat/completions", "/chat/completions", "/v1/responses", "/responses", "/v1/messages", "/messages":
		return defaultPath
	case "/v1/embeddings", "/embeddings":
		return gitHubDefaultPathForMode(relaymode.Embeddings)
	}

	if strings.HasPrefix(lower, "/orgs/") {
		segments := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")
		if len(segments) >= 2 {
			org := segments[1]
			return "/orgs/" + org + gitHubDefaultPathForMode(relayMode)
		}
		return defaultPath
	}

	return defaultPath
}

func gitHubDefaultPathForMode(relayMode int) string {
	switch relayMode {
	case relaymode.Embeddings:
		return "/inference/embeddings"
	default:
		return "/inference/chat/completions"
	}
}
