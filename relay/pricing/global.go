package pricing

import (
	"fmt"
	"sync"

	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/apitype"
)

// DefaultGlobalPricingAdapters defines which adapters contribute to global pricing fallback
// This can be easily modified to add or remove adapters from the global pricing system
// Only includes adapters that have been refactored with comprehensive pricing models
var DefaultGlobalPricingAdapters = []int{
	apitype.OpenAI,    // Comprehensive GPT models with pricing
	apitype.Anthropic, // Claude models with pricing
	apitype.Gemini,    // Google Gemini models with pricing
	apitype.Ali,       // Alibaba Qwen models with pricing
	apitype.Baidu,     // Baidu ERNIE models with pricing
	apitype.Zhipu,     // Zhipu GLM models with pricing
	apitype.DeepSeek,  // DeepSeek models with pricing
	apitype.Groq,      // Groq models with pricing
	apitype.Mistral,   // Mistral models with pricing
	apitype.Moonshot,  // Moonshot models with pricing
	apitype.Cohere,    // Cohere models with pricing
	apitype.Tencent,   // Tencent Hunyuan models with pricing
	apitype.Xunfei,    // Xunfei Spark models with pricing
}

// GlobalPricingManager manages the third-layer global model pricing
// It merges pricing from selected adapters to provide fallback pricing
// for OpenAI-compatible channels (including legacy "custom" entries) that don't have specific model pricing
type GlobalPricingManager struct {
	mu                   sync.RWMutex
	globalModelPricing   map[string]adaptor.ModelConfig
	contributingAdapters []int // API types of adapters to include in global pricing
	initialized          bool
	getAdaptorFunc       func(apiType int) adaptor.Adaptor
}

// Global instance of the pricing manager
var globalPricingManager = &GlobalPricingManager{
	// Will be initialized from configuration
	contributingAdapters: nil,
}

// InitializeGlobalPricingManager sets up the global pricing manager with the adaptor getter function
func InitializeGlobalPricingManager(getAdaptor func(apiType int) adaptor.Adaptor) {
	globalPricingManager.mu.Lock()
	defer globalPricingManager.mu.Unlock()

	globalPricingManager.getAdaptorFunc = getAdaptor

	// Load contributing adapters from default configuration
	if globalPricingManager.contributingAdapters == nil {
		globalPricingManager.contributingAdapters = make([]int, len(DefaultGlobalPricingAdapters))
		copy(globalPricingManager.contributingAdapters, DefaultGlobalPricingAdapters)
		logger.Logger.Info("Loaded adapters for global pricing", zap.Int("adapter_count", len(globalPricingManager.contributingAdapters)))
	}

	globalPricingManager.initialized = false // Force re-initialization with new function

	logger.Logger.Info("Global pricing manager initialized")
}

// SetContributingAdapters allows configuration of which adapters contribute to global pricing
func SetContributingAdapters(apiTypes []int) {
	globalPricingManager.mu.Lock()
	defer globalPricingManager.mu.Unlock()

	globalPricingManager.contributingAdapters = make([]int, len(apiTypes))
	copy(globalPricingManager.contributingAdapters, apiTypes)
	globalPricingManager.initialized = false // Force re-initialization

	logger.Logger.Info("Global pricing adapters updated, will reload on next access")
}

// ReloadDefaultConfiguration reloads the adapter configuration from the default slice
func ReloadDefaultConfiguration() {
	globalPricingManager.mu.Lock()
	defer globalPricingManager.mu.Unlock()

	globalPricingManager.contributingAdapters = make([]int, len(DefaultGlobalPricingAdapters))
	copy(globalPricingManager.contributingAdapters, DefaultGlobalPricingAdapters)
	globalPricingManager.initialized = false // Force re-initialization

	logger.Logger.Info("Reloaded global pricing configuration", zap.Int("adapter_count", len(globalPricingManager.contributingAdapters)))
}

// GetContributingAdapters returns the current list of contributing adapters
func GetContributingAdapters() []int {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	result := make([]int, len(globalPricingManager.contributingAdapters))
	copy(result, globalPricingManager.contributingAdapters)
	return result
}

// ensureInitialized ensures the global pricing is initialized
// Must be called with at least read lock held
func (gpm *GlobalPricingManager) ensureInitialized() {
	if gpm.initialized || gpm.getAdaptorFunc == nil {
		return
	}

	// Upgrade to write lock
	gpm.mu.RUnlock()
	gpm.mu.Lock()
	defer func() {
		gpm.mu.Unlock()
		gpm.mu.RLock()
	}()

	// Check again after acquiring write lock
	if gpm.initialized {
		return
	}

	gpm.initializeUnsafe()
}

// initializeUnsafe performs the actual initialization without locking
// Must be called with write lock held
func (gpm *GlobalPricingManager) initializeUnsafe() {
	if gpm.getAdaptorFunc == nil {
		logger.Logger.Warn("Global pricing manager not properly initialized - missing adaptor getter function")
		return
	}

	logger.Logger.Info("Initializing global model pricing from contributing adapters...")

	gpm.globalModelPricing = make(map[string]adaptor.ModelConfig)
	successCount := 0

	for _, apiType := range gpm.contributingAdapters {
		if gpm.mergeAdapterPricing(apiType) {
			successCount++
		}
	}

	gpm.initialized = true
	logger.Logger.Info("Global model pricing initialized",
		zap.Int("model_count", len(gpm.globalModelPricing)),
		zap.Int("successful_adapters", successCount),
		zap.Int("total_adapters", len(gpm.contributingAdapters)))
}

// mergeAdapterPricing merges pricing from a specific adapter
// Must be called with write lock held
// Returns true if successful, false otherwise
func (gpm *GlobalPricingManager) mergeAdapterPricing(apiType int) bool {
	adaptor := gpm.getAdaptorFunc(apiType)
	if adaptor == nil {
		logger.Logger.Warn(fmt.Sprintf("No adaptor found for API type %d", apiType))
		return false
	}

	pricing := adaptor.GetDefaultModelPricing()
	if len(pricing) == 0 {
		logger.Logger.Warn(fmt.Sprintf("Adaptor %d returned empty pricing", apiType))
		return false
	}

	mergedCount := 0
	conflictCount := 0

	for modelName, modelPrice := range pricing {
		if existingPrice, exists := gpm.globalModelPricing[modelName]; exists {
			// Handle conflict: prefer the first adapter's pricing (could be configurable)
			logger.Logger.Warn(fmt.Sprintf("Model %s pricing conflict: existing=%.9f, new=%.9f (keeping existing)",
				modelName, existingPrice.Ratio, modelPrice.Ratio))
			conflictCount++
		} else {
			gpm.globalModelPricing[modelName] = cloneModelConfig(modelPrice)
			mergedCount++
		}
	}

	logger.Logger.Info("Merged models from adapter",
		zap.Int("merged_count", mergedCount),
		zap.Int("api_type", apiType),
		zap.Int("conflict_count", conflictCount))
	return true
}

// GetGlobalModelRatio returns the global model ratio for a given model
// and a boolean indicating whether the model exists in global pricing.
func GetGlobalModelRatio(modelName string) (float64, bool) {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if price, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return price.Ratio, true
	}

	return 0, false
}

// GetGlobalCompletionRatio returns the global completion ratio for a given model
// and a boolean indicating whether the model exists in global pricing.
func GetGlobalCompletionRatio(modelName string) (float64, bool) {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if price, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return price.CompletionRatio, true
	}

	return 0, false
}

// GetGlobalModelPricing returns a copy of the entire global pricing map
func GetGlobalModelPricing() map[string]adaptor.ModelConfig {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	// Return a copy to prevent external modification
	result := make(map[string]adaptor.ModelConfig, len(globalPricingManager.globalModelPricing))
	for modelName, cfg := range globalPricingManager.globalModelPricing {
		result[modelName] = cloneModelConfig(cfg)
	}

	return result
}

// GetGlobalModelConfig returns the full global model configuration for a given model name.
// The returned configuration is cloned to prevent external mutation of the cache.
func GetGlobalModelConfig(modelName string) (adaptor.ModelConfig, bool) {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if cfg, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return cloneModelConfig(cfg), true
	}
	return adaptor.ModelConfig{}, false
}

// GetGlobalVideoPricing returns the global video pricing configuration for a given model.
// The returned configuration is cloned to prevent external mutation of the cache.
func GetGlobalVideoPricing(modelName string) *adaptor.VideoPricingConfig {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if cfg, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return cfg.Video.Clone()
	}
	return nil
}

// GetGlobalAudioPricing returns the global audio pricing configuration for a given model.
// The returned configuration is cloned to prevent external mutation of the cache.
func GetGlobalAudioPricing(modelName string) *adaptor.AudioPricingConfig {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if cfg, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return cfg.Audio.Clone()
	}
	return nil
}

// GetGlobalImagePricing returns the global image pricing configuration for a given model.
// The returned configuration is cloned to prevent external mutation of the cache.
func GetGlobalImagePricing(modelName string) *adaptor.ImagePricingConfig {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	if cfg, exists := globalPricingManager.globalModelPricing[modelName]; exists {
		return cfg.Image.Clone()
	}
	return nil
}

// ReloadGlobalPricing forces a reload of the global pricing from contributing adapters
func ReloadGlobalPricing() {
	globalPricingManager.mu.Lock()
	defer globalPricingManager.mu.Unlock()

	globalPricingManager.initialized = false
	globalPricingManager.initializeUnsafe()
}

// GetGlobalPricingStats returns statistics about the global pricing
func GetGlobalPricingStats() (int, int) {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	globalPricingManager.ensureInitialized()

	return len(globalPricingManager.globalModelPricing), len(globalPricingManager.contributingAdapters)
}

// IsGlobalPricingInitialized returns whether the global pricing has been initialized
func IsGlobalPricingInitialized() bool {
	globalPricingManager.mu.RLock()
	defer globalPricingManager.mu.RUnlock()

	return globalPricingManager.initialized && globalPricingManager.getAdaptorFunc != nil
}

// GetModelRatioWithThreeLayers implements the three-layer pricing fallback:
// 1. Channel-specific overrides (highest priority)
// 2. Adapter default pricing (second priority)
// 3. Global pricing fallback (third priority)
// 4. Final default (lowest priority)
func GetModelRatioWithThreeLayers(modelName string, channelOverrides map[string]float64, adaptor adaptor.Adaptor) float64 {
	// Layer 1: User custom ratio (channel-specific overrides)
	if channelOverrides != nil {
		if override, exists := channelOverrides[modelName]; exists {
			return override
		}
	}

	// Layer 2: Channel default ratio (adapter's default pricing)
	if adaptor != nil {
		// Use GetDefaultModelPricing directly to avoid redundant lookups and function calls
		if price, exists := adaptor.GetDefaultModelPricing()[modelName]; exists {
			return price.Ratio
		}
	}

	// Layer 3: Global model pricing (merged from selected adapters)
	// Respect explicit zero pricing by checking existence, not value.
	if ratio, exists := GetGlobalModelRatio(modelName); exists {
		return ratio
	}

	// Layer 4: Final fallback - reasonable default
	return 2.5 * 0.000001 // 2.5 USD per million tokens
}

// GetCompletionRatioWithThreeLayers implements the three-layer completion ratio fallback
func GetCompletionRatioWithThreeLayers(modelName string, channelOverrides map[string]float64, adaptor adaptor.Adaptor) float64 {
	// Layer 1: User custom ratio (channel-specific overrides)
	if channelOverrides != nil {
		if override, exists := channelOverrides[modelName]; exists {
			return override
		}
	}

	// Layer 2: Channel default ratio (adapter's default pricing)
	if adaptor != nil {
		// Use GetDefaultModelPricing directly to avoid redundant lookups and function calls
		if price, exists := adaptor.GetDefaultModelPricing()[modelName]; exists {
			return price.CompletionRatio
		}
	}

	// Layer 3: Global model pricing (merged from selected adapters)
	// Respect explicit zero pricing by checking existence, not value.
	if ratio, exists := GetGlobalCompletionRatio(modelName); exists {
		return ratio
	}

	// Layer 4: Final fallback - reasonable default
	return 1.0 // Default completion ratio
}

// GetVideoPricingWithThreeLayers resolves video pricing metadata using the same precedence rules
// as token pricing: channel overrides, adapter defaults, then global pricing.
func GetVideoPricingWithThreeLayers(modelName string, channelOverride *adaptor.VideoPricingConfig, provider adaptor.Adaptor) *adaptor.VideoPricingConfig {
	if channelOverride != nil && channelOverride.HasData() {
		return channelOverride.Clone()
	}

	if provider != nil {
		if cfg, exists := provider.GetDefaultModelPricing()[modelName]; exists {
			if cfg.Video != nil && cfg.Video.HasData() {
				return cfg.Video.Clone()
			}
		}
	}

	if cfg := GetGlobalVideoPricing(modelName); cfg != nil {
		if cfg.HasData() {
			return cfg
		}
	}

	return nil
}

func cloneModelConfig(src adaptor.ModelConfig) adaptor.ModelConfig {
	clone := src
	if len(src.Tiers) > 0 {
		clone.Tiers = append([]adaptor.ModelRatioTier(nil), src.Tiers...)
	}
	if src.Video != nil {
		clone.Video = src.Video.Clone()
	}
	if src.Audio != nil {
		clone.Audio = src.Audio.Clone()
	}
	if src.Image != nil {
		clone.Image = src.Image.Clone()
	}
	return clone
}

// EffectivePricing holds fully-resolved pricing numbers for the current request
// after applying tiers and cached discounts.
type EffectivePricing struct {
	// Per-token prices (per 1 token)
	InputRatio       float64
	OutputRatio      float64 // equals InputRatio * CompletionRatio
	CachedInputRatio float64 // negative means free
	// Cache-write prices (per 1 token)
	CacheWrite5mRatio    float64 // zero => use InputRatio; negative => free
	CacheWrite1hRatio    float64 // zero => use InputRatio; negative => free
	AppliedTierThreshold int     // 0 for base tier
}

// ResolveEffectivePricing determines the effective pricing for a model given the
// input token count and the adapter's default pricing table. Channel overrides
// are already handled in higher-level ratio resolution and should be folded into
// the per-token ratio before calling this if overrides apply globally.
//
// Behavior:
// - If no tiers exist, returns base ratios.
// - If tiers exist, finds the tier whose InputTokenThreshold <= inputTokens and is the highest such threshold.
// - Optional tier fields inherit from base if zero. Negative cached ratios mean free.
func ResolveEffectivePricing(modelName string, inputTokens int, adaptor adaptor.Adaptor) EffectivePricing {
	eff := EffectivePricing{}
	if adaptor == nil {
		// Fallback to defaults if adaptor missing
		baseIn := 2.5 * 0.000001
		baseComp := 1.0
		eff.InputRatio = baseIn
		eff.OutputRatio = baseIn * baseComp
		eff.CachedInputRatio = 0
		eff.CacheWrite5mRatio = 0
		eff.CacheWrite1hRatio = 0
		eff.AppliedTierThreshold = 0
		return eff
	}

	pricing := adaptor.GetDefaultModelPricing()
	base, ok := pricing[modelName]
	if !ok {
		// Use adaptor fallbacks
		baseRatio := adaptor.GetModelRatio(modelName)
		baseComp := adaptor.GetCompletionRatio(modelName)
		eff.InputRatio = baseRatio
		eff.OutputRatio = baseRatio * baseComp
		eff.CachedInputRatio = base.CachedInputRatio // will be zero, as base not exists
		eff.CacheWrite5mRatio = base.CacheWrite5mRatio
		eff.CacheWrite1hRatio = base.CacheWrite1hRatio
		eff.AppliedTierThreshold = 0
		return eff
	}

	// Start with base
	in := base.Ratio
	comp := base.CompletionRatio
	cachedIn := base.CachedInputRatio
	cw5 := base.CacheWrite5mRatio
	cw1 := base.CacheWrite1hRatio
	appliedThreshold := 0

	// Find applicable tier (tiers are sorted ascending by threshold)
	if len(base.Tiers) > 0 {
		for _, t := range base.Tiers {
			if inputTokens >= t.InputTokenThreshold {
				// Apply overrides from this tier
				if t.Ratio != 0 {
					in = t.Ratio
				}
				if t.CompletionRatio != 0 {
					comp = t.CompletionRatio
				}
				if t.CachedInputRatio != 0 {
					cachedIn = t.CachedInputRatio
				}
				if t.CacheWrite5mRatio != 0 {
					cw5 = t.CacheWrite5mRatio
				}
				if t.CacheWrite1hRatio != 0 {
					cw1 = t.CacheWrite1hRatio
				}
				appliedThreshold = t.InputTokenThreshold
			} else {
				break
			}
		}
	}

	eff.InputRatio = in
	// Allow completion ratio to be zero if explicitly configured (means free completion tokens)
	eff.OutputRatio = in * comp
	eff.CachedInputRatio = cachedIn
	eff.CacheWrite5mRatio = cw5
	eff.CacheWrite1hRatio = cw1
	eff.AppliedTierThreshold = appliedThreshold
	return eff
}
