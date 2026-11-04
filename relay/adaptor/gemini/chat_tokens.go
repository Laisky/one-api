package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// NativeCountTransport keeps counting on the same provider endpoint and channel
// credentials as generation, including Vertex's regional/global routing.
type NativeCountTransport interface {
	GetRequestURL(*meta.Meta) (string, error)
	SetupRequestHeader(*gin.Context, *http.Request, *meta.Meta) error
}

// HasExtendedMedia reports inputs that cannot use the legacy text/image estimate.
func HasExtendedMedia(request *ChatRequest) bool {
	if request == nil {
		return false
	}
	for _, content := range nativePromptContents(request) {
		for _, part := range content.Parts {
			if part.FileData != nil || (part.InlineData != nil && !strings.HasPrefix(part.InlineData.MimeType, "image/")) {
				return true
			}
		}
	}
	return false
}

// nativePromptContents includes the system instruction for modality inspection.
func nativePromptContents(request *ChatRequest) []ChatContent {
	contents := append([]ChatContent(nil), request.Contents...)
	if request.SystemInstruction != nil {
		contents = append(contents, *request.SystemInstruction)
	}
	return contents
}

// CountNativeChatTokens counts the prepared provider-visible prompt before
// quota reservation. It never falls back to a text-only count for media, and
// never exposes provider error bodies or media URLs to clients or logs.
// Sources: ai.google.dev/api/tokens and the Vertex CountTokens API (2026-11-02).
func CountNativeChatTokens(c *gin.Context, routing *meta.Meta, prepared *ChatRequest, transport NativeCountTransport) (*relaymodel.Usage, error) {
	if routing == nil || prepared == nil || transport == nil {
		return nil, errors.New("native media counting requires a prepared request and channel")
	}
	countMeta := *routing
	countMeta.Mode, countMeta.IsStream = relaymode.ChatCompletions, false
	endpoint, err := transport.GetRequestURL(&countMeta)
	if err != nil {
		return nil, errors.New("cannot construct native media count endpoint")
	}
	u, err := url.Parse(endpoint)
	if err != nil || !strings.HasSuffix(u.Path, ":generateContent") {
		return nil, errors.New("native media count endpoint is unsupported")
	}
	u.Path = strings.TrimSuffix(u.Path, ":generateContent") + ":countTokens"
	u.RawPath, u.RawQuery = "", ""
	// These are exactly the token-bearing fields of the prepared request.
	prompt := struct {
		Model             string        `json:"model,omitempty"`
		Contents          []ChatContent `json:"contents"`
		SystemInstruction *ChatContent  `json:"systemInstruction,omitempty"`
		Tools             []ChatTools   `json:"tools,omitempty"`
	}{Contents: prepared.Contents, SystemInstruction: prepared.SystemInstruction, Tools: prepared.Tools}
	var payload any = prompt
	if routing.APIType == apitype.Gemini {
		prompt.Model = "models/" + routing.ActualModelName
		payload = struct {
			Request any `json:"generateContentRequest"`
		}{prompt}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("cannot encode native media count request")
	}
	ctx, cancel := context.WithTimeout(gmw.Ctx(c), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot create native media count request")
	}
	if err = transport.SetupRequestHeader(c, req, &countMeta); err != nil {
		return nil, errors.New("cannot authenticate native media count request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.New("native media token counting failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("native media token counting failed")
	}
	var count CountTokensResponse
	if err := json.Unmarshal(data, &count); err != nil || count.Error != nil || count.TotalTokens <= 0 {
		return nil, errors.New("native media token count is missing or invalid")
	}
	if cfg, ok := geminiOpenaiCompatible.ModelRatios[strings.ToLower(routing.ActualModelName)]; ok && cfg.ContextLength > 0 && int64(count.TotalTokens) > int64(cfg.ContextLength) {
		return nil, errors.New("validation failed: media input exceeds the model context limit")
	}
	remaining := count.TotalTokens
	for _, detail := range count.PromptTokensDetails {
		if detail.TokenCount < 0 || detail.TokenCount > remaining {
			return nil, errors.New("native media token detail is invalid")
		}
		remaining -= detail.TokenCount
	}
	usage := buildPromptUsageFromCountTokens(&count)
	// Admission never assumes future implicit-cache hits. Count the full prompt.
	if usage.PromptTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = 0
	}
	applyNativeGoogleAudioReceipt(usage, count.PromptTokensDetails, nil)
	// Audio can have a separate price: absence of a modality receipt must never
	// make audio input look like cheaper text. Uniform video/PDF totals suffice.
	for _, content := range nativePromptContents(prepared) {
		for _, part := range content.Parts {
			mimeType := ""
			if part.InlineData != nil {
				mimeType = part.InlineData.MimeType
			}
			if part.FileData != nil {
				mimeType = part.FileData.MimeType
			}
			if strings.HasPrefix(mimeType, "audio/") && (usage.PromptTokensDetails == nil || usage.PromptTokensDetails.AudioTokens <= 0) {
				return nil, errors.New("native audio token count lacks modality details")
			}
		}
	}
	return usage, nil
}
