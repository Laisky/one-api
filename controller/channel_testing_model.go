package controller

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

const textTestModality = "text"

// chooseChannelTestModel returns the model name that should be used for a text-only channel test.
// Parameters: channel is the channel being tested, and requestedModel is an optional explicit model name.
// Returns: the selected model name, whether an incompatible stored testing model should be cleared, and an error.
func chooseChannelTestModel(channel *model.Channel, requestedModel string) (string, bool, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel != "" {
		if !channel.SupportsModel(requestedModel) {
			return "", false, errors.Errorf("test model %q is not supported by channel", requestedModel)
		}
		if !channelTestModelSupportsText(channel, requestedModel) {
			return "", false, errors.Errorf("test model %q does not support both text input and text output", requestedModel)
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

	modelName := cheapestTextTestModel(channel)
	if modelName == "" {
		return "", clearStored, errors.New("channel has no model that supports both text input and text output for testing")
	}
	return modelName, clearStored, nil
}

// cheapestTextTestModel returns the cheapest text-in/text-out model available on the channel.
// Parameters: channel provides the configured model list, model mapping, and channel-specific pricing.
// Returns: the selected model name, or an empty string when no text-capable test model is available.
func cheapestTextTestModel(channel *model.Channel) string {
	names := channelTextTestModels(channel)
	defaultPricing := defaultModelPricingForChannel(channel)

	var (
		cheapestName  string
		cheapestRatio float64
		initialized   bool
	)
	for _, name := range names {
		ratio := testModelRatio(channel, name, defaultPricing)
		if !initialized || ratio < cheapestRatio {
			cheapestName = name
			cheapestRatio = ratio
			initialized = true
		}
	}
	return cheapestName
}

type channelListItem struct {
	*model.Channel
	TestModels []string `json:"test_models"`
}

// MarshalJSON surfaces test_models alongside the embedded channel fields.
//
// model.Channel defines a value-receiver MarshalJSON that the embedded *Channel
// promotes onto channelListItem. Without this override, json.Marshal would call
// the promoted Channel.MarshalJSON and silently drop test_models, breaking the
// admin UI's per-channel test-model selector. We marshal the channel and splice
// test_models in (as [] when empty, never omitted) while preserving field order.
func (item channelListItem) MarshalJSON() ([]byte, error) {
	testModels := item.TestModels
	if testModels == nil {
		testModels = []string{}
	}
	testModelsJSON, err := json.Marshal(testModels)
	if err != nil {
		return nil, errors.Wrap(err, "marshal test_models")
	}

	if item.Channel == nil {
		return append(append([]byte(`{"test_models":`), testModelsJSON...), '}'), nil
	}

	channelJSON, err := json.Marshal(item.Channel)
	if err != nil {
		return nil, errors.Wrap(err, "marshal channel")
	}
	end := bytes.LastIndexByte(channelJSON, '}')
	if end < 0 {
		return nil, errors.Errorf("unexpected channel json payload: %s", channelJSON)
	}

	inner := bytes.TrimSpace(channelJSON[1:end])
	var buf bytes.Buffer
	buf.Grow(len(channelJSON) + len(testModelsJSON) + 16)
	buf.WriteByte('{')
	if len(inner) > 0 {
		buf.Write(inner)
		buf.WriteByte(',')
	}
	buf.WriteString(`"test_models":`)
	buf.Write(testModelsJSON)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// buildChannelListResponse wraps channel rows with text-compatible test model choices.
// Parameters: channels is the list returned by channel list or search APIs.
// Returns: channel response rows with an added test_models field for the admin UI.
func buildChannelListResponse(channels []*model.Channel) []channelListItem {
	items := make([]channelListItem, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		items = append(items, channelListItem{
			Channel:    channel,
			TestModels: channelTextTestModels(channel),
		})
	}
	return items
}

// channelTextTestModels returns text-in/text-out model names that can be used for channel tests.
// Parameters: channel provides configured models, model mapping, and provider metadata.
// Returns: a sorted list of public model names suitable for the admin testing-model selector.
func channelTextTestModels(channel *model.Channel) []string {
	names := channel.GetSupportedModelNames()
	defaultPricing := defaultModelPricingForChannel(channel)
	if len(names) == 0 {
		names = make([]string, 0, len(defaultPricing))
		for name := range defaultPricing {
			names = append(names, name)
		}
	}

	testModels := make([]string, 0, len(names))
	for _, name := range names {
		if channelTestModelSupportsText(channel, name) {
			testModels = append(testModels, name)
		}
	}
	sort.Strings(testModels)
	return testModels
}

// channelTestModelSupportsText reports whether a model can support the text-only channel test.
// Parameters: channel provides provider metadata and model mappings, while modelName is the public model name.
// Returns: true when the model accepts text input and can produce text output.
func channelTestModelSupportsText(channel *model.Channel, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	defaultPricing := defaultModelPricingForChannel(channel)
	mappedName := modelName
	if mapping := channel.GetModelMapping(); mapping != nil {
		if mapped := strings.TrimSpace(mapping[modelName]); mapped != "" {
			mappedName = mapped
		}
	}
	if modelNameLooksNonTextTestable(mappedName) {
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

// modelNameLooksNonTextTestable rejects known non-chat model families even when legacy metadata is incomplete.
// Parameters: modelName is the resolved upstream model name when available.
// Returns: true when the model name identifies a model that cannot return chat text.
func modelNameLooksNonTextTestable(modelName string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(modelName))
	if lowerName == "" {
		return false
	}
	nonTextMarkers := []string{
		"embedding",
		"rerank",
		"sora",
		"tts",
		"transcribe",
		"whisper",
		"dall-e",
		"gpt-image",
		"imagen",
		"veo",
		"video",
	}
	for _, marker := range nonTextMarkers {
		if strings.Contains(lowerName, marker) {
			return true
		}
	}
	return false
}

// defaultModelPricingForChannel returns provider model metadata for channel test selection.
// Parameters: channel identifies the upstream channel type.
// Returns: the provider default model configuration map, or nil when no adaptor metadata is available.
func defaultModelPricingForChannel(channel *model.Channel) map[string]adaptor.ModelConfig {
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

// modelConfigSupportsTextTest reports whether model metadata is compatible with text channel tests.
// Parameters: cfg is the provider model metadata, and known indicates whether the metadata came from a known model entry.
// Returns: true when the model accepts text input and produces text chat output.
func modelConfigSupportsTextTest(cfg adaptor.ModelConfig, known bool) bool {
	if !known {
		return true
	}
	if cfg.Embedding != nil || cfg.PerCall != nil {
		return false
	}
	return modalitiesContainTextOrDefault(cfg.InputModalities) && modalitiesContainTextOrDefault(cfg.OutputModalities)
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
func testModelRatio(channel *model.Channel, modelName string, defaultPricing map[string]adaptor.ModelConfig) float64 {
	if configs := channel.GetModelPriceConfigs(); len(configs) > 0 {
		if cfg, ok := configs[modelName]; ok {
			return cfg.Ratio
		}
	}
	if ratios := channel.GetModelRatio(); len(ratios) > 0 {
		if ratio, ok := ratios[modelName]; ok {
			return ratio
		}
	}
	if cfg, ok := lookupModelConfig(defaultPricing, modelName); ok {
		return cfg.Ratio
	}
	return 0
}
