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
	claudeMessagesTestTarget  = "claude_messages"
	responseAPITestTarget     = "response_api"
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
	if !channelHasChatSurfaceWithContext(ctx, channel) {
		return "", false, errChannelTestNotApplicable(
			"channel exposes no chat-capable endpoint (%s) to health check",
			chatSurfaceEndpointList())
	}

	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel != "" {
		if !channel.SupportsModel(requestedModel) {
			return "", false, errors.Errorf("test model %q is not supported by channel", requestedModel)
		}
		// An explicitly requested model is the administrator's own choice, so a model
		// of unknown API format is honoured; only a known non-chat format is refused.
		if !channelTestModelUsesChatFormat(channel, requestedModel, true) {
			return "", false, errors.Errorf("test model %q is not served through a chat API format", requestedModel)
		}
		return requestedModel, false, nil
	}

	// An administrator who selected SKIP for this channel has opted it out of health
	// checking. This is checked after the explicit requestedModel branch so a manual
	// test naming a model can still force a one-off probe.
	if channelTestingSkipped(channel) {
		return "", false, errChannelTestNotApplicable(
			"channel testing is disabled for this channel (testing model set to SKIP)")
	}

	clearStored := false
	if channel.TestingModel != nil && strings.TrimSpace(*channel.TestingModel) != "" {
		storedModel := strings.TrimSpace(*channel.TestingModel)
		if channel.SupportsModel(storedModel) && channelTestModelUsesChatFormat(channel, storedModel, true) {
			return storedModel, false, nil
		}
		clearStored = true
	}

	modelName := cheapestTextTestModel(ctx, channel)
	if modelName == "" {
		return "", clearStored, errChannelTestNotApplicable(
			"channel has no model known to be served through a chat API format: every configured model is " +
				"either a non-chat format (embeddings, rerank, moderation, completions, media) or of unknown " +
				"format; set the channel testing_model to probe one deliberately")
	}
	return modelName, clearStored, nil
}

// cheapestTextTestModel returns the cheapest chat-format model the sweep may pick.
// Parameters: channel provides the configured model list, model mapping, and channel-specific pricing.
// Returns: the selected model name, or an empty string when no compatible model is available.
func cheapestTextTestModel(ctx context.Context, channel *model.Channel) string {
	names := channelAutoTestModels(channel)
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
	// The admin selector is an explicit choice, so it also offers models whose API
	// format the gateway cannot determine. That is how an operator opts a
	// custom-named chat model back into testing after auto-selection skipped it.
	return channelTestModelCandidates(channel, true)
}

// channelAutoTestModels returns the models the automatic sweep may pick on its own.
// Parameters: channel provides configured models, mappings, endpoints, and provider metadata.
// Returns: a sorted list of models known to be served through a chat API format.
func channelAutoTestModels(channel *model.Channel) []string {
	return channelTestModelCandidates(channel, false)
}

// channelTestModelCandidates lists the channel models eligible for the chat probe.
// Parameters: channel supplies the model list and metadata; explicit widens the result
// to models of unknown API format, which only an administrator may choose deliberately.
// Returns: a sorted, non-nil list of public model names.
func channelTestModelCandidates(channel *model.Channel, explicit bool) []string {
	testModels := make([]string, 0)
	if channel == nil {
		return testModels
	}
	// A channel with no chat-capable surface has nothing to offer the test-model selector.
	if !channelHasChatSurface(channel) {
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
		if channelTestModelUsesChatFormat(channel, name, explicit) {
			testModels = append(testModels, name)
		}
	}
	sort.Strings(testModels)
	return testModels
}

// channelTestingSkipped reports whether an administrator excluded this channel from
// health checking by selecting SKIP as its testing model.
// Parameters: channel is the channel under consideration.
// Returns: true when the channel must not be probed.
func channelTestingSkipped(channel *model.Channel) bool {
	if channel == nil || channel.TestingModel == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*channel.TestingModel), model.ChannelTestingModelSkip)
}

// channelTestModelUsesChatFormat reports whether a model is served through one of
// the chat API formats the health probe speaks: OpenAI Chat Completions, the
// Response API, Anthropic Messages, or Gemini generateContent.
//
// Modality is deliberately not the criterion. Embedding, rerank, moderation and
// legacy completions models all declare text input and text output -- OpenAI's own
// text-embedding-3-small is registered as text-in/text-out -- so only the API format
// separates them from chat models.
//
// Parameters: channel provides provider metadata and model mappings, modelName is the
// public model name, and explicit marks a model an administrator named themselves
// (the channel's testing_model, or ?model= on a manual test).
// Returns: true when the model may receive the chat probe.
func channelTestModelUsesChatFormat(channel *model.Channel, modelName string, explicit bool) bool {
	if channel == nil {
		return false
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	defaultPricing := defaultModelPricingForChannel(channel)
	mappedName := mappedChannelTestModelName(channel, modelName)
	if modelNameLooksNonChatFormat(modelName) || modelNameLooksNonChatFormat(mappedName) {
		return false
	}

	if cfg, ok := lookupModelConfig(defaultPricing, mappedName); ok {
		return modelConfigUsesChatFormat(cfg)
	}
	if cfg, ok := lookupModelConfig(defaultPricing, modelName); ok {
		return modelConfigUsesChatFormat(cfg)
	}

	// Fall back to the cross-provider catalogue. Which API format serves a model is
	// a property of the model, not of the channel carrying it, and a channel type's
	// own table can legitimately lack it: GeminiOpenAICompatible resolves to the
	// OpenAI adaptor's catalogue, which lists no Gemini models at all. Without this
	// step every Gemini model on such a channel would count as unknown format.
	if cfg, ok := globalModelConfig(mappedName); ok {
		return modelConfigUsesChatFormat(cfg)
	}
	if cfg, ok := globalModelConfig(modelName); ok {
		return modelConfigUsesChatFormat(cfg)
	}

	// Unknown model: nothing states which API format serves it. Assuming "chat" is
	// exactly what sends a Chat Completions request to a self-hosted embeddings
	// deployment, so the default is to leave it out of the probe's scope. An
	// administrator who knows better can still name it as the channel testing model.
	return explicit
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

// modelNameLooksNonChatFormat rejects model families served through a non-chat API
// format, for the cases where provider metadata is incomplete or wrong (several
// embedding models are registered with no Embedding pricing block and plain
// text/text modalities, so only the name identifies them).
//
// Markers here must denote a non-chat API FORMAT, not merely a specialized chat
// model: a vision-language, OCR-tuned or safety-classifier model that is served over
// Chat Completions belongs in the probe's scope.
// Parameters: modelName is the public or resolved upstream model name.
// Returns: true when the model name identifies a non-chat API format.
func modelNameLooksNonChatFormat(modelName string) bool {
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
		"distilbert",
		"resnet",
		"m2m100",
		"indictrans",
		"bart-large-cnn",
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

// globalModelConfig looks a model up in the cross-provider catalogue.
// Parameters: modelName is the public or mapped upstream model name.
// Returns: the catalogue entry and whether any provider declares this model.
func globalModelConfig(modelName string) (adaptor.ModelConfig, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return adaptor.ModelConfig{}, false
	}
	return pricing.GetGlobalModelConfig(modelName)
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

// modelConfigUsesChatFormat reports whether provider metadata describes a model
// served through a chat API format.
// Parameters: cfg is the provider model metadata for a known model.
// Returns: true when nothing in the metadata marks the model as a non-chat surface.
func modelConfigUsesChatFormat(cfg adaptor.ModelConfig) bool {
	// A billing-shape pointer is the reliable non-chat signal: it marks a model
	// priced per embedding, per call (rerank), per image or per second of video.
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
