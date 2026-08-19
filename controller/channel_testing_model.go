package controller

import (
	"context"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

const (
	textTestModality          = "text"
	chatCompletionsTestTarget = "chat_completions"
)

// chooseChannelTestModel returns the model name that should be used for a Chat Completions channel test.
// Parameters: channel is the channel being tested, and requestedModel is an optional explicit model name.
// Returns: the selected model name, whether an incompatible stored testing model should be cleared, and an error.
func chooseChannelTestModel(channel *model.Channel, requestedModel string) (string, bool, error) {
	return chooseChannelTestModelWithContext(context.Background(), channel, requestedModel)
}

// chooseChannelTestModelWithContext selects a text Chat Completions model with request-scoped configuration diagnostics.
// Parameters: ctx carries request logging, channel is the channel being tested, and requestedModel is optional.
// Returns: the selected model name, whether an incompatible stored model should be cleared, and an error.
func chooseChannelTestModelWithContext(ctx context.Context, channel *model.Channel, requestedModel string) (string, bool, error) {
	if channel == nil {
		return "", false, errors.New("channel is nil")
	}
	if !channelSupportsChatCompletionsWithContext(ctx, channel) {
		return "", false, errors.New("channel does not support the Chat Completions endpoint required for channel testing")
	}

	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel != "" {
		if !channel.SupportsModel(requestedModel) {
			return "", false, errors.Errorf("test model %q is not supported by channel", requestedModel)
		}
		if !channelTestModelSupportsText(channel, requestedModel) {
			return "", false, errors.Errorf("test model %q does not support both text input and text output through Chat Completions", requestedModel)
		}
		return requestedModel, false, nil
	}

	clearStored := false
	if channel.TestingModel != nil && strings.TrimSpace(*channel.TestingModel) != "" {
		storedModel := strings.TrimSpace(*channel.TestingModel)
		if channel.SupportsModel(storedModel) && channelTestModelSupportsText(channel, storedModel) {
			return storedModel, false, nil
		}
		clearStored = true
	}

	modelName := cheapestTextTestModel(ctx, channel)
	if modelName == "" {
		return "", clearStored, errors.New("channel has no model that supports both text input and text output through Chat Completions for testing")
	}
	return modelName, clearStored, nil
}

// channelSupportsChatCompletionsWithContext reports whether the channel exposes the endpoint used by live channel tests.
// Parameters: ctx carries request logging and channel contains the effective endpoint configuration.
// Returns: true when Chat Completions is explicitly configured or enabled by the channel type defaults.
func channelSupportsChatCompletionsWithContext(ctx context.Context, channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	endpoints := channel.GetSupportedEndpointsWithContext(ctx)
	if len(endpoints) == 0 {
		endpoints = channeltype.DefaultEndpointNamesForChannelType(channel.Type)
	}
	return channeltype.IsEndpointSupportedByName(chatCompletionsTestTarget, endpoints)
}

// cheapestTextTestModel returns the cheapest text Chat Completions model available on the channel.
// Parameters: channel provides the configured model list, model mapping, and channel-specific pricing.
// Returns: the selected model name, or an empty string when no compatible model is available.
func cheapestTextTestModel(ctx context.Context, channel *model.Channel) string {
	names := channelTextTestModels(channel)
	defaultPricing := defaultModelPricingForChannel(channel)

	var (
		cheapestName  string
		cheapestRatio float64
		initialized   bool
	)
	for _, name := range names {
		ratio := testModelRatio(ctx, channel, name, defaultPricing)
		if !initialized || ratio < cheapestRatio {
			cheapestName = name
			cheapestRatio = ratio
			initialized = true
		}
	}
	return cheapestName
}

// channelListItem is the admin channel-list row: the boundary channel DTO plus
// the Chat Completions-compatible test-model choices for the per-channel test selector.
//
// It embeds dto.ChannelResponse (a plain struct with no methods), so json.Marshal
// promotes the channel fields inline and appends test_models — byte-identical to
// the response the retired byte-splicing MarshalJSON produced, but without the
// embedding-promotion hazard that override existed to work around (model.Channel
// no longer carries a MarshalJSON to promote).
type channelListItem struct {
	dto.ChannelResponse
	TestModels []string `json:"test_models"`
}

// buildChannelListResponse wraps channel rows with Chat Completions-compatible test model choices.
// Parameters: channels is the list returned by channel list or search APIs.
// Returns: channel response rows with an added test_models field for the admin UI.
func buildChannelListResponse(channels []*model.Channel) []channelListItem {
	items := make([]channelListItem, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		// channelTextTestModels always returns a non-nil slice, so test_models
		// serializes as [] (never null) when empty — matching the old splicer.
		items = append(items, channelListItem{
			ChannelResponse: channel.ToResponse(),
			TestModels:      channelTextTestModels(channel),
		})
	}
	return items
}

// channelTextTestModels returns model names that can receive the standard text Chat Completions probe.
// Parameters: channel provides configured models, model mapping, endpoint configuration, and provider metadata.
// Returns: a sorted list of public model names suitable for the admin testing-model selector.
func channelTextTestModels(channel *model.Channel) []string {
	testModels := make([]string, 0)
	if channel == nil || !channelSupportsChatCompletionsWithContext(context.Background(), channel) {
		return testModels
	}

	names := channel.GetSupportedModelNames()
	defaultPricing := defaultModelPricingForChannel(channel)
	if len(names) == 0 {
		names = make([]string, 0, len(defaultPricing))
		for name := range defaultPricing {
			names = append(names, name)
		}
	}

	for _, name := range names {
		if channelTestModelSupportsText(channel, name) {
			testModels = append(testModels, name)
		}
	}
	sort.Strings(testModels)
	return testModels
}

// channelTestModelSupportsText reports whether a model can support the standard text Chat Completions test.
// Parameters: channel provides provider metadata and model mappings, while modelName is the public model name.
// Returns: true when the model accepts text input, returns text, and is not a specialized non-chat task model.
func channelTestModelSupportsText(channel *model.Channel, modelName string) bool {
	if channel == nil {
		return false
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	defaultPricing := defaultModelPricingForChannel(channel)
	mappedName := mappedChannelTestModelName(channel, modelName)
	if modelNameLooksNonTextTestable(modelName) || modelNameLooksNonTextTestable(mappedName) {
		return false
	}

	if cfg, ok := lookupModelConfig(defaultPricing, mappedName); ok {
		return modelConfigSupportsTextTest(cfg, true)
	}
	if cfg, ok := lookupModelConfig(defaultPricing, modelName); ok {
		return modelConfigSupportsTextTest(cfg, true)
	}
	return modelConfigSupportsTextTest(adaptor.ModelConfig{}, false)
}

// mappedChannelTestModelName resolves a public model name to its configured upstream model name.
// Parameters: channel supplies model mappings and modelName is the public channel model name.
// Returns: the mapped upstream name, or the original model name when no mapping exists.
func mappedChannelTestModelName(channel *model.Channel, modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if channel == nil {
		return modelName
	}
	if mapping := channel.GetModelMapping(); mapping != nil {
		if mapped := strings.TrimSpace(mapping[modelName]); mapped != "" {
			return mapped
		}
	}
	return modelName
}

// modelNameLooksNonTextTestable rejects known non-chat model families even when provider metadata is incomplete.
// Parameters: modelName is the public or resolved upstream model name.
// Returns: true when the model name identifies a task that cannot receive a Chat Completions request.
func modelNameLooksNonTextTestable(modelName string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(modelName))
	if lowerName == "" {
		return false
	}

	nonChatMarkers := []string{
		"embedding",
		"embeddings",
		"bge-",
		"bge_",
		"rerank",
		"reranker",
		"moderation",
		"llama-guard",
		"/guard-",
		"distilbert",
		"resnet",
		"m2m100",
		"indictrans",
		"bart-large-cnn",
		"moondream",
		"ocr",
		"layout-parsing",
		"voice-clone",
		"voice_clone",
		"whisper",
		"melotts",
		"aura-",
		"tts",
		"transcribe",
		"transcription",
		"speech-to-text",
		"text-to-speech",
		"dall-e",
		"gpt-image",
		"stable-diffusion",
		"dreamshaper",
		"flux-",
		"imagen",
		"sora",
		"veo",
		"video",
		"text-davinci-",
		"code-davinci-",
		"text-curie-",
		"text-babbage-",
		"text-ada-",
		"gpt-3.5-turbo-instruct",
	}
	for _, marker := range nonChatMarkers {
		if strings.Contains(lowerName, marker) {
			return true
		}
	}

	switch lowerName {
	case "ada", "babbage", "curie", "davinci":
		return true
	default:
		return false
	}
}

// defaultModelPricingForChannel returns provider model metadata for channel test selection.
// Parameters: channel identifies the upstream channel type.
// Returns: the provider default model configuration map, or nil when no adaptor metadata is available.
func defaultModelPricingForChannel(channel *model.Channel) map[string]adaptor.ModelConfig {
	if channel == nil {
		return nil
	}
	if channeltype.IsOpenAICompatible(channel.Type) {
		return pricing.GetGlobalModelPricing()
	}
	pricingAdaptor := relay.GetAdaptor(channeltype.ToAPIType(channel.Type))
	if pricingAdaptor == nil {
		return nil
	}
	return pricingAdaptor.GetDefaultModelPricing()
}

// lookupModelConfig finds model metadata by exact or case-insensitive model name.
// Parameters: configs is a provider model metadata map, and modelName is the desired model key.
// Returns: the matching model configuration and whether a match was found.
func lookupModelConfig(configs map[string]adaptor.ModelConfig, modelName string) (adaptor.ModelConfig, bool) {
	if len(configs) == 0 {
		return adaptor.ModelConfig{}, false
	}
	if cfg, ok := configs[modelName]; ok {
		return cfg, true
	}
	for name, cfg := range configs {
		if strings.EqualFold(name, modelName) {
			return cfg, true
		}
	}
	return adaptor.ModelConfig{}, false
}

// modelConfigSupportsTextTest reports whether model metadata is compatible with text Chat Completions tests.
// Parameters: cfg is the provider model metadata, and known indicates whether the metadata came from a known model entry.
// Returns: true when the model accepts text input, returns text, and is not explicitly specialized for another task.
func modelConfigSupportsTextTest(cfg adaptor.ModelConfig, known bool) bool {
	if !known {
		return true
	}
	if cfg.Embedding != nil || cfg.PerCall != nil || cfg.Image != nil || cfg.Video != nil {
		return false
	}
	if !modalitiesContainTextOrDefault(cfg.InputModalities) || !modalitiesContainTextOrDefault(cfg.OutputModalities) {
		return false
	}
	return !modelDescriptionLooksNonChatTestable(cfg.Description)
}

// modelDescriptionLooksNonChatTestable rejects provider descriptions that explicitly identify a specialized non-chat task.
// Parameters: description is provider-maintained model metadata.
// Returns: true when the description identifies embeddings, classification, media processing, or another non-chat task.
func modelDescriptionLooksNonChatTestable(description string) bool {
	lowerDescription := strings.ToLower(strings.TrimSpace(description))
	if lowerDescription == "" {
		return false
	}

	nonChatDescriptions := []string{
		"embedding model",
		"embeddings model",
		"text embeddings",
		"vector embedding",
		"reranker",
		"reranking model",
		"text classifier",
		"text classification",
		"sentiment classifier",
		"classification model",
		"safety classifier",
		"content safety classification",
		"moderation model",
		"translation model",
		"multilingual translation",
		"speech-to-text",
		"automatic speech recognition",
		"text-to-speech",
		"voice cloning",
		"transcription model",
		"text-to-image",
		"image generation model",
		"image classifier",
		"object detection",
		"image-to-text",
		"video generation",
		"summarization model",
		"document ocr",
		"layout parsing",
	}
	for _, marker := range nonChatDescriptions {
		if strings.Contains(lowerDescription, marker) {
			return true
		}
	}
	return false
}

// modalitiesContainTextOrDefault reports whether a modality list supports text.
// Parameters: modalities is the provider-declared modality list; an empty list follows the legacy text default.
// Returns: true when text is present or the list is empty.
func modalitiesContainTextOrDefault(modalities []string) bool {
	if len(modalities) == 0 {
		return true
	}
	for _, modality := range modalities {
		if strings.EqualFold(strings.TrimSpace(modality), textTestModality) {
			return true
		}
	}
	return false
}

// testModelRatio returns the configured ratio used to rank fallback test models.
// Parameters: channel provides channel-specific pricing, modelName is the candidate, and defaultPricing is provider metadata.
// Returns: the best available input ratio for ordering candidates.
func testModelRatio(ctx context.Context, channel *model.Channel, modelName string, defaultPricing map[string]adaptor.ModelConfig) float64 {
	if channel == nil {
		return 0
	}
	if configs := channel.GetModelPriceConfigsWithContext(ctx); len(configs) > 0 {
		if cfg, ok := configs[modelName]; ok {
			return cfg.Ratio
		}
	}
	if ratios := channel.GetModelRatioWithContext(ctx); len(ratios) > 0 {
		if ratio, ok := ratios[modelName]; ok {
			return ratio
		}
	}
	mappedName := mappedChannelTestModelName(channel, modelName)
	if cfg, ok := lookupModelConfig(defaultPricing, mappedName); ok {
		return cfg.Ratio
	}
	if cfg, ok := lookupModelConfig(defaultPricing, modelName); ok {
		return cfg.Ratio
	}
	return 0
}
