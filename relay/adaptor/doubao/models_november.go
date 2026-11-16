package doubao

import "github.com/Laisky/one-api/relay/adaptor"

// novemberArkModels returns newly published Ark models with representable
// Standard tariffs. It takes no arguments and returns independently owned metadata.
// Sources checked 2026-11-16:
// https://docs.volcengine.com/docs/ark/model-list?lang=zh
// https://docs.volcengine.com/docs/ark/model-pricing?lang=zh
// https://docs.volcengine.com/docs/ark/image-understanding?lang=zh
// https://docs.volcengine.com/docs/ark/video-understanding?lang=zh
// https://docs.volcengine.com/docs/ark/chat-api?lang=zh
func novemberArkModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"kimi-k3-1-pro-261024": arkChatConfig(13, 1.3, 68, 1_024_000, 1_024_000,
			[]string{"text", "image", "video"}, []string{"tools", "reasoning", "structured_outputs"},
			"Ark-hosted Kimi K3.1 Pro October snapshot. Thinking is always enabled; temperature is fixed at 1. Image and video input require Base64 data, not remote URLs. The compatible gateway does not forward this model's upstream reasoning_effort. Native Moonshot cache-write tariffs and Coding Plan multipliers do not apply."),
	}
}
