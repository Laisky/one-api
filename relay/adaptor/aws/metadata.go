package aws

// Shared metadata helpers for AWS Bedrock model entries. These slices are
// reused across many ModelRatios entries to keep the table compact and
// consistent with sibling adaptors (anthropic, cohere, openai, etc.).
var (
	// awsClaudeVisionInputs lists input modalities for Claude 3+ vision models on Bedrock.
	awsClaudeVisionInputs = []string{"text", "image", "file"}
	// awsTextInputs lists input modalities for text-only chat models.
	awsTextInputs = []string{"text"}
	// awsVisionInputs lists input modalities for multimodal text+image chat models.
	awsVisionInputs = []string{"text", "image"}
	// awsTextOutputs lists output modalities for chat completions.
	awsTextOutputs = []string{"text"}

	// awsClaudeFeaturesWithReasoning advertises features for Claude 4+ models on Bedrock.
	awsClaudeFeaturesWithReasoning = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	// awsClaudeFeaturesNoReasoning advertises features for Claude 3.x models on Bedrock.
	awsClaudeFeaturesNoReasoning = []string{"tools", "json_mode", "structured_outputs"}
	// awsClaudeLegacyFeatures advertises the limited capability set for Claude 2.x / instant.
	awsClaudeLegacyFeatures = []string{}

	// awsClaudeSamplingParams lists Claude-compatible sampling parameters.
	awsClaudeSamplingParams = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
	// awsBasicSamplingParams lists OpenAI-compatible sampling parameters supported by Bedrock chat models.
	awsBasicSamplingParams = []string{"temperature", "top_p", "stop", "max_tokens"}

	// awsLlamaFeatures advertises features for Llama 3+ models on Bedrock.
	awsLlamaFeatures = []string{}
	// awsLlamaToolsFeatures advertises tool-capable Llama 3.1+ on Bedrock (Bedrock exposes tools via Converse API).
	awsLlamaToolsFeatures = []string{"tools"}

	// awsNovaFeatures advertises features for Amazon Nova chat models.
	awsNovaFeatures = []string{"tools", "structured_outputs"}

	// awsCohereFeatures advertises features for Cohere Command R/R+ on Bedrock.
	awsCohereFeatures = []string{"tools", "structured_outputs"}

	// awsQwenFeatures advertises features for Qwen3 chat models on Bedrock.
	awsQwenFeatures = []string{"tools", "reasoning"}

	// awsDeepSeekFeatures advertises features for DeepSeek chat models on Bedrock.
	awsDeepSeekFeatures = []string{"reasoning"}

	// awsMistralFeatures advertises features for Mistral chat models on Bedrock.
	awsMistralFeatures = []string{}

	// awsOpenAIOSSFeatures advertises features for OpenAI gpt-oss models on Bedrock.
	awsOpenAIOSSFeatures = []string{"reasoning"}

	// awsWriterFeatures advertises features for Writer Palmyra chat models on Bedrock.
	awsWriterFeatures = []string{}
)
