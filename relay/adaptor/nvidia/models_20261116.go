package nvidia

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor"
)

// novemberModels returns independently owned defaults for published free trial
// chat endpoints. It takes no parameters. Prices do not describe paid Partner
// Endpoints or private deployments. Hosted specifications and embedded API
// schemas were checked on 2026-11-16 alongside each page's /modelcard:
// https://build.nvidia.com/mistralai/mistral-large-4-1t-a52b
// https://build.nvidia.com/nvidia/nemotron-3.5-bolt-e4b
// https://build.nvidia.com/minimaxai/minimax-m3.1-flash
// https://build.nvidia.com/deepseek-ai/deepseek-v4.1-flash
// https://build.nvidia.com/nvidia/nemotron-3.5-super-vl-120b-a12b
// https://build.nvidia.com/nvidia/nemotron-3.5-lightning-30b-a3b
// https://build.nvidia.com/stepfun-ai/step-5-preview
// https://build.nvidia.com/moonshotai/kimi-k3_1-pro
// https://build.nvidia.com/meta/muse-glimmer-30b
func novemberModels() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig)
	for _, entry := range []struct {
		id            string
		context       int32
		vision        bool
		structured    bool
		precision     string
		efforts       []string
		defaultEffort string
		description   string
	}{
		{
			id: "mistralai/mistral-large-4-1t-a52b", context: 262_144, vision: true, structured: true,
			efforts: []string{"none", "high"}, defaultEffort: "high",
			description: "NVIDIA-hosted Mistral Large 4. Hosted specifications publish 262144 context, while the checkpoint card describes 1M. The hosted API schema still exposes none/high reasoning, default high; it does not inherit the first-party Mistral effort vocabulary.",
		},
		{
			id: "nvidia/nemotron-3.5-bolt-e4b", context: 65_536, structured: true, precision: "bf16",
			description: "NVIDIA Nemotron 3.5 Bolt E4B BF16, text-only hosted 64K context. Native reasoning is controlled by chat_template_kwargs.enable_thinking; no unsupported string effort vocabulary is advertised.",
		},
		{
			id: "minimaxai/minimax-m3.1-flash", context: 262_144, vision: true, structured: true, precision: "fp4",
			description: "NVIDIA-hosted MiniMax M3.1 Flash NVFP4. Hosted specifications publish 262144 context; the checkpoint supports up to 1M depending on serving configuration. Text and image chat are advertised here; native video and chat-template reasoning controls require their own payload contracts.",
		},
		{
			id: "deepseek-ai/deepseek-v4.1-flash", context: 1_048_576, vision: true,
			description: "NVIDIA-hosted DeepSeek V4.1 Flash with text/image input and a 1048576-token context. Numeric checkpoint reasoning controls are not translated into an undocumented string reasoning_effort interface.",
		},
		{
			id: "nvidia/nemotron-3.5-super-vl-120b-a12b", context: 262_144, vision: true, structured: true, precision: "fp4",
			efforts: []string{"none", "low", "high"}, defaultEffort: "high",
			description: "NVIDIA Nemotron 3.5 Super VL NVFP4. Hosted specifications publish 262144 context rather than the checkpoint's potential 1M. Native reasoning_effort supports none/low/high, default high. Text/image chat is advertised; native video transport is not inferred.",
		},
		{
			id: "nvidia/nemotron-3.5-lightning-30b-a3b", context: 1_048_576, structured: true, precision: "fp4",
			description: "NVIDIA Nemotron 3.5 Lightning NVFP4 with text-only 1048576-token context. Native enable_thinking and numeric reasoning_budget controls are distinct from string reasoning_effort; no effort vocabulary is inferred.",
		},
		{
			id: "stepfun-ai/step-5-preview", context: 1_048_576, vision: true, precision: "fp4",
			description: "NVIDIA-hosted Step 5 Preview with NVFP4 experts, text/image input and 1048576-token context. Reasoning is always enabled in this endpoint; the hosted schema does not support disabling thinking. Structured output and an exact hosted output ceiling are not advertised.",
		},
		{
			id: "moonshotai/kimi-k3.1-pro", context: 1_048_576, vision: true, structured: true,
			efforts: []string{"low", "medium", "high", "max"}, defaultEffort: "low",
			description: "NVIDIA-hosted Kimi K3.1 Pro, exact dotted API ID despite the underscore in the catalog URL. Hosted text/image input, 1048576-token context and low/medium/high/max effort, default low. A separate maximum output ceiling is undisclosed.",
		},
		{
			id: "meta/muse-glimmer-30b", context: 131_072, vision: true,
			description: "NVIDIA-hosted Muse Glimmer 30B with text/image input and 131072-token hosted context. Tools and reasoning are available; structured output is explicitly unsupported. The embedded effort schema refers to a different model, so no effort vocabulary or default is inferred from that inconsistent evidence.",
		},
	} {
		cfg := adaptor.ModelConfig{
			Ratio: 0, CompletionRatio: 1, ContextLength: entry.context,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			SupportedReasoningEfforts:   slices.Clone(entry.efforts), DefaultReasoningEffort: entry.defaultEffort,
			Quantization: entry.precision,
			Description:  entry.description + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		}
		if entry.vision {
			cfg.InputModalities = append(cfg.InputModalities, "image")
		}
		if entry.structured {
			cfg.SupportedFeatures = append(cfg.SupportedFeatures, "json_mode", "structured_outputs")
		}
		if len(entry.efforts) > 0 {
			cfg.SupportedSamplingParameters = append(cfg.SupportedSamplingParameters, "reasoning_effort")
		}
		models[entry.id] = cfg
	}
	return models
}
