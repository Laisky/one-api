package vertexai

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor"
	nativegemini "github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

var vertexGoogleModels = []string{"gemini-4-neon", "gemini-4-hydrogen", "gemini-4-carbon", "gemini-4-carbon-cyber-permissive", "gemini-4-argon-cyber-permissive", "gemini-3.8-flash-cyber"}

// argonCyberModel identifies the published Vertex-only Argon cybersecurity variant.
const argonCyberModel = "gemini-4-argon-cyber-permissive"

// VertexGoogleModelConfigs returns independently owned global standard tariffs and metadata.
// Sources verified 2026-11-22: https://cloud.google.com/model-garden/pricing
// and https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen.
// Neon: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-neon.
// Argon Cyber: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-argon.
// Promotional credits and regional/account-specific tariffs require operator configuration.
func VertexGoogleModelConfigs() map[string]adaptor.ModelConfig {
	configs := make(map[string]adaptor.ModelConfig)
	for _, spec := range []struct {
		name                  string
		input, output, cached float64
		limit                 int32
		efforts               []string
		def, note             string
	}{
		{"gemini-4-neon", 3, 15, .15, 32768, []string{"low", "medium", "high", "xhigh"}, "high", "GA for verified enterprise customers in global, us and eu. Global Standard pricing; regional tariffs require an override. Relay foreground output is capped at 32,768 until billing for retrievable usage on longer failed requests is implemented. Provider segments support 65,536; continuation and background execution are not implemented."},
		{"gemini-4-hydrogen", 1.5, 7.5, .075, 65536, []string{"low", "medium", "high", "xhigh"}, "high", "GA short-term availability. Global, us and eu. Foreground GenerateContent only; continuation and background execution are not implemented."},
		{"gemini-4-carbon", 4, 20, .20, 32768, []string{"low", "medium", "high"}, "medium", "Verified enterprise customers in select regions only. Standard gross tariff; promotional credits are not deducted. Foreground only."},
		{"gemini-4-carbon-cyber-permissive", 4, 20, .20, 32768, []string{"low", "medium", "high"}, "high", "Fairwind allowlist required; global and us only. Computer use is unavailable. Foreground only."},
		{argonCyberModel, 4, 20, .20, 32768, []string{"low", "medium", "high"}, "high", "Fairwind allowlist required; global, us and eu. Global Standard pricing without promotional credits; regional tariffs require an override. Computer use is unavailable. Relay foreground output is capped at 32,768 until billing for longer failed requests is implemented."},
		{"gemini-3.8-flash-cyber", 1.5, 7.5, .15, 65536, []string{"low", "medium", "high"}, "medium", "Cyber access approval required. Global standard pricing; regional tariffs require an override."},
	} {
		configs[spec.name] = adaptor.ModelConfig{
			Ratio: spec.input * ratio.MilliTokensUsd, CompletionRatio: spec.output / spec.input,
			CachedInputRatio: spec.cached * ratio.MilliTokensUsd,
			ContextLength:    1048576, MaxOutputTokens: spec.limit,
			InputModalities: []string{"text", "image", "audio", "video", "file"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens"}, SupportedReasoningEfforts: spec.efforts,
			DefaultReasoningEffort: spec.def, Description: spec.note,
		}
	}
	// Flash Cyber documents ordinary sampling defaults rather than Gemini 4 fixed values.
	// The current native transport forwards temperature and top_p; top_k is not exposed here.
	flash := configs["gemini-3.8-flash-cyber"]
	flash.SupportedSamplingParameters = []string{"max_tokens", "temperature", "top_p"}
	configs["gemini-3.8-flash-cyber"] = flash
	return configs
}

// configureVertexGoogleRequest validates native reasoning and foreground limits.
// request is read only; converted owns its generation configuration.
// Gemini 3+ uses levels: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking.
func configureVertexGoogleRequest(name string, request *model.GeneralOpenAIRequest, converted any) error {
	name = strings.ToLower(name)
	xhighModel := name == "gemini-4-hydrogen" || name == "gemini-4-neon"
	four := xhighModel || name == "gemini-4-carbon" || name == "gemini-4-carbon-cyber-permissive" || name == "gemini-4-argon-cyber-permissive"
	if !four && name != "gemini-3.8-flash-cyber" {
		return nil
	}
	native, ok := converted.(*nativegemini.ChatRequest)
	if !ok || request == nil {
		return nil
	}
	if request.Thinking != nil {
		return errors.Errorf("%s requires reasoning_effort levels; thinking budgets and provider-specific thinking controls are unsupported", name)
	}
	level := "MEDIUM"
	if xhighModel || name == "gemini-4-carbon-cyber-permissive" || name == "gemini-4-argon-cyber-permissive" {
		level = "HIGH"
	}
	if request.ReasoningEffort != nil && request.Reasoning != nil && request.Reasoning.Effort != nil && !strings.EqualFold(*request.ReasoningEffort, *request.Reasoning.Effort) {
		return errors.New("conflicting reasoning_effort and reasoning.effort")
	}
	if request.ReasoningEffort == nil && request.Reasoning != nil && request.Reasoning.Effort != nil {
		level = strings.ToUpper(*request.Reasoning.Effort)
	}
	if request.ReasoningEffort != nil {
		level = strings.ToUpper(*request.ReasoningEffort)
	}
	if level != "LOW" && level != "MEDIUM" && level != "HIGH" && !(xhighModel && level == "XHIGH") {
		return errors.Errorf("unsupported reasoning_effort %q for %s", level, name)
	}
	limit := 65536
	cappedAt32K := name == "gemini-4-neon" || name == argonCyberModel
	if cappedAt32K || (four && !xhighModel && !request.Stream) {
		limit = 32768
	}
	if cappedAt32K && native.GenerationConfig.MaxOutputTokens == 0 {
		// An omitted upstream limit permits 64K and the paid-failure contract
		// above 32K, which this foreground relay cannot settle yet.
		native.GenerationConfig.MaxOutputTokens = limit
	}
	if native.GenerationConfig.MaxOutputTokens < 0 || native.GenerationConfig.MaxOutputTokens > limit {
		return errors.Errorf("%s foreground output limit is %d tokens; long decoding is unsupported", name, limit)
	}
	if four && request.N != nil && *request.N > 1 {
		return errors.Errorf("%s supports one candidate", name)
	}
	native.GenerationConfig.ThinkingConfig = &nativegemini.ThinkingConfig{ThinkingLevel: level}
	if four {
		native.GenerationConfig.Temperature = nil
		native.GenerationConfig.TopP = nil
		native.GenerationConfig.TopK = 0
	}
	return nil
}
