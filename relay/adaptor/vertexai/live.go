package vertexai

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// LiveRequestURL builds Vertex's native Bidi endpoint without probing project
// entitlements or restricting model IDs to the bundled catalog. Parameters: m
// supplies administrator-owned routing. Returns: a TLS URL or configuration error.
// Source: https://cloud.google.com/vertex-ai/generative-ai/docs/live-api/start-manage-session.
func LiveRequestURL(m *meta.Meta) (string, error) {
	if m == nil || m.ChannelType != channeltype.VertextAI || m.Mode != relaymode.Realtime {
		return "", errors.Wrap(gemini.ErrLiveProtocol, "invalid Vertex Live transport")
	}
	if err := gemini.ValidateLiveModelName(m.ActualModelName); err != nil {
		return "", err
	}
	if !validLiveResourceSegment(m.Config.VertexAIProjectID) || !validLiveResourceSegment(vertexLiveLocation(m)) {
		return "", errors.Wrap(gemini.ErrLiveProtocol, "Vertex Live requires a valid project and location")
	}
	version := m.Config.APIVersion
	if version == "" {
		version = "v1"
	}
	if version != "v1" && version != "v1beta1" {
		return "", errors.Wrap(gemini.ErrLiveProtocol, "Vertex Live API version must be v1 or v1beta1")
	}
	base := m.BaseURL
	if base == "" {
		host := vertexLiveLocation(m) + "-aiplatform.googleapis.com"
		if vertexLiveLocation(m) == "global" {
			host = "aiplatform.googleapis.com"
		}
		base = "https://" + host
	}
	if override := m.UpstreamEndpointURLOverride(); override != "" {
		return gemini.ResolveLiveEndpoint(override, "", true)
	}
	return gemini.ResolveLiveEndpoint(base, "/ws/google.cloud.aiplatform."+version+".LlmBidiService/BidiGenerateContent", false)
}

// vertexLiveLocation preserves the administrator's region instead of applying
// REST's version-based global routing rule. Parameters: m is validated metadata.
// Returns: the configured location, or the default us-central1 location.
func vertexLiveLocation(m *meta.Meta) string {
	if m.Config.Region != "" {
		return m.Config.Region
	}
	return "us-central1"
}

// validLiveResourceSegment validates syntax, not upstream permission or regional
// availability. Parameters: value is one resource segment. Returns: whether it
// is nonempty and cannot escape its path position.
func validLiveResourceSegment(value string) bool {
	if len(value) == 0 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-_.:", c)) {
			return false
		}
	}
	return true
}

// LiveHandler authenticates using the selected Vertex channel's existing OAuth
// credential path, then delegates native frames and receipts to the shared pump.
// Parameters: c is authenticated and m is mapped metadata. Returns: a safe
// pre-upgrade error or the sealed ledger. Caller bearer tokens are never forwarded.
func LiveHandler(c *gin.Context, m *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	endpoint, err := LiveRequestURL(m)
	if err != nil {
		return openai.ErrorWrapper(err, "vertex_live_configuration", http.StatusBadRequest), nil
	}
	if strings.TrimSpace(m.Config.VertexAIADC) == "" {
		return openai.ErrorWrapper(errors.Wrap(gemini.ErrLiveProtocol, "missing Vertex channel credentials"), "vertex_live_configuration", http.StatusBadRequest), nil
	}
	if !websocket.IsWebSocketUpgrade(c.Request) {
		return openai.ErrorWrapper(errors.Wrap(gemini.ErrLiveProtocol, "WebSocket upgrade required"), "gemini_live_upgrade", http.StatusBadRequest), nil
	}
	token, err := getToken(c.Request.Context(), m.ChannelId, m.Config.VertexAIADC)
	if err != nil || strings.TrimSpace(token) == "" {
		// Credential-client errors may contain credential or endpoint details.
		return openai.ErrorWrapper(errors.Wrap(gemini.ErrLiveProtocol, "Vertex OAuth credential exchange failed"), "vertex_live_authentication", http.StatusBadGateway), nil
	}
	resource := "projects/" + m.Config.VertexAIProjectID + "/locations/" + vertexLiveLocation(m) + "/publishers/google/models/" + m.ActualModelName
	return gemini.LiveHandlerWithTransport(c, m, gemini.LiveTransport{
		Endpoint: endpoint, Headers: http.Header{"Authorization": []string{"Bearer " + token}},
		ModelResource: resource,
	})
}
