package pricing

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// MockAdaptor implements the adaptor.Adaptor interface for testing
type MockAdaptor struct {
	pricing map[string]adaptor.ModelConfig
	name    string
}

func (m *MockAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return m.pricing
}

func (m *MockAdaptor) GetModelRatio(modelName string) float64 {
	if price, exists := m.pricing[modelName]; exists {
		return price.Ratio
	}
	return 2.5 * 0.000001 // Default fallback
}

func (m *MockAdaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := m.pricing[modelName]; exists {
		return price.CompletionRatio
	}
	return 1.0 // Default fallback
}

// Implement other required methods with minimal implementations
func (m *MockAdaptor) Init(meta *meta.Meta)                          {}
func (m *MockAdaptor) GetRequestURL(meta *meta.Meta) (string, error) { return "", nil }
func (m *MockAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	return nil
}
func (m *MockAdaptor) ConvertRequest(c *gin.Context, relayMode int, request *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (m *MockAdaptor) ConvertImageRequest(c *gin.Context, request *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}
func (m *MockAdaptor) ConvertClaudeRequest(c *gin.Context, request *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}
func (m *MockAdaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return nil, nil
}
func (m *MockAdaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}
func (m *MockAdaptor) GetModelList() []string { return []string{} }
func (m *MockAdaptor) GetChannelName() string { return m.name }

// Mock GetAdaptor function for testing
func mockGetAdaptor(apiType int) adaptor.Adaptor {
	switch apiType {
	case apitype.OpenAI:
		return &MockAdaptor{
			name: "openai",
			pricing: map[string]adaptor.ModelConfig{
				"gpt-4":         {Ratio: 30 * 0.000001, CompletionRatio: 2.0},
				"gpt-3.5-turbo": {Ratio: 1.5 * 0.000001, CompletionRatio: 2.0},
			},
		}
	case apitype.Anthropic:
		return &MockAdaptor{
			name: "anthropic",
			pricing: map[string]adaptor.ModelConfig{
				"claude-3-opus":   {Ratio: 15 * 0.000001, CompletionRatio: 5.0},
				"claude-3-sonnet": {Ratio: 3 * 0.000001, CompletionRatio: 5.0},
			},
		}
	case apitype.Gemini:
		return &MockAdaptor{
			name: "gemini",
			pricing: map[string]adaptor.ModelConfig{
				"gemini-2.5-flash": {Ratio: 0.30 * 0.000001, CompletionRatio: 2.5 / 0.30},
				"gpt-4":            {Ratio: 25 * 0.000001, CompletionRatio: 2.5}, // Conflict with OpenAI
			},
		}
	default:
		return nil
	}
}

func TestGlobalPricingManagerInitialization(t *testing.T) {
	// Reset global state
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: nil,
	}

	// Test initialization
	InitializeGlobalPricingManager(mockGetAdaptor)

	require.NotNil(t, globalPricingManager.getAdaptorFunc, "Expected adaptor function to be set")
	require.NotEmpty(t, globalPricingManager.contributingAdapters, "Expected contributing adapters to be loaded from default configuration")

	// Check that it matches the default adapters
	require.Len(t, globalPricingManager.contributingAdapters, len(DefaultGlobalPricingAdapters))
}

func TestGlobalPricingMerging(t *testing.T) {
	// Reset and initialize
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic, apitype.Gemini},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Force initialization
	globalPricingManager.mu.Lock()
	globalPricingManager.initializeUnsafe()
	globalPricingManager.mu.Unlock()

	// Test that models from all adapters are merged
	pricing := GetGlobalModelPricing()

	// Check OpenAI models
	require.Contains(t, pricing, "gpt-3.5-turbo", "Expected gpt-3.5-turbo from OpenAI to be in global pricing")

	// Check Anthropic models
	require.Contains(t, pricing, "claude-3-opus", "Expected claude-3-opus from Anthropic to be in global pricing")

	// Check Gemini models
	require.Contains(t, pricing, "gemini-2.5-flash", "Expected gemini-2.5-flash from Gemini to be in global pricing")

	// Test conflict resolution (first adapter wins)
	require.Contains(t, pricing, "gpt-4", "Expected gpt-4 to be in global pricing")
	expectedRatio := 30 * 0.000001 // OpenAI's pricing should win
	require.Equal(t, expectedRatio, pricing["gpt-4"].Ratio, "Expected gpt-4 ratio to be OpenAI's pricing")
}

func TestGetGlobalModelRatio(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test existing model
	ratio, exists := GetGlobalModelRatio("gpt-3.5-turbo")
	expectedRatio := 1.5 * 0.000001
	require.True(t, exists)
	require.Equal(t, expectedRatio, ratio)

	// Test non-existing model
	ratio, exists = GetGlobalModelRatio("non-existent-model")
	require.False(t, exists)
	require.Equal(t, float64(0), ratio, "Expected 0 for non-existent model")
}

func TestGetGlobalCompletionRatio(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test existing model
	ratio, exists := GetGlobalCompletionRatio("claude-3-opus")
	expectedRatio := 5.0
	require.True(t, exists)
	require.Equal(t, expectedRatio, ratio)

	// Test non-existing model
	ratio, exists = GetGlobalCompletionRatio("non-existent-model")
	require.False(t, exists)
	require.Equal(t, float64(0), ratio, "Expected 0 for non-existent model")
}

func TestThreeLayerPricing(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test Layer 1: Channel overrides (highest priority)
	channelOverrides := map[string]float64{
		"gpt-4": 100 * 0.000001, // Override
	}
	openaiAdaptor := mockGetAdaptor(apitype.OpenAI)

	ratio := GetModelRatioWithThreeLayers("gpt-4", channelOverrides, openaiAdaptor)
	expectedRatio := 100 * 0.000001
	require.Equal(t, expectedRatio, ratio, "Expected channel override ratio")

	// Test Layer 2: Adapter pricing (second priority)
	ratio = GetModelRatioWithThreeLayers("gpt-4", nil, openaiAdaptor)
	expectedRatio = 30 * 0.000001 // OpenAI's pricing
	require.Equal(t, expectedRatio, ratio, "Expected adapter ratio")

	// Test Layer 3: Global pricing (third priority)
	// Use a model that exists in global pricing but not in the current adapter
	ratio = GetModelRatioWithThreeLayers("claude-3-opus", nil, openaiAdaptor)
	expectedRatio = 15 * 0.000001 // From global pricing (Anthropic)
	require.Equal(t, expectedRatio, ratio, "Expected global pricing ratio")

	// Test Layer 4: Final fallback
	ratio = GetModelRatioWithThreeLayers("completely-unknown-model", nil, openaiAdaptor)
	expectedRatio = 2.5 * 0.000001 // Final fallback
	require.Equal(t, expectedRatio, ratio, "Expected fallback ratio")
}

// TestGetCompletionRatioWithThreeLayers verifies completion ratio fallback across layers.
func TestGetCompletionRatioWithThreeLayers(t *testing.T) {
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	openaiAdaptor := mockGetAdaptor(apitype.OpenAI)

	ratio := GetCompletionRatioWithThreeLayers("gpt-4", nil, openaiAdaptor)
	require.InDelta(t, 2.0, ratio, 0.0000001, "expected adapter completion ratio")

	ratio = GetCompletionRatioWithThreeLayers("claude-3-opus", nil, openaiAdaptor)
	require.InDelta(t, 5.0, ratio, 0.0000001, "expected global completion ratio")
}

func TestSetContributingAdapters(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test setting new adapters
	newAdapters := []int{apitype.OpenAI}
	SetContributingAdapters(newAdapters)

	adapters := GetContributingAdapters()
	require.Len(t, adapters, 1)
	require.Equal(t, apitype.OpenAI, adapters[0])

	// Verify pricing is reloaded
	pricing := GetGlobalModelPricing()
	require.Contains(t, pricing, "gpt-4", "Expected gpt-4 to be in global pricing after adapter change")
	require.NotContains(t, pricing, "claude-3-opus", "Expected claude-3-opus to NOT be in global pricing after removing Anthropic adapter")
}

func TestGetGlobalPricingStats(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	modelCount, adapterCount := GetGlobalPricingStats()

	require.Equal(t, 2, adapterCount)
	require.NotZero(t, modelCount, "Expected some models in global pricing")
}

func TestReloadGlobalPricing(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Get initial stats
	initialModelCount, _ := GetGlobalPricingStats()

	// Add more adapters and reload
	SetContributingAdapters([]int{apitype.OpenAI, apitype.Anthropic})
	ReloadGlobalPricing()

	// Check that more models are now available
	newModelCount, _ := GetGlobalPricingStats()
	require.Greater(t, newModelCount, initialModelCount, "Expected more models after reload")
}

func TestDefaultGlobalPricingAdapters(t *testing.T) {
	// Test that the default adapters slice is properly defined
	require.NotEmpty(t, DefaultGlobalPricingAdapters, "DefaultGlobalPricingAdapters should not be empty")

	// Test that core adapters with comprehensive pricing models are included
	coreAdapters := []int{
		apitype.OpenAI,
		apitype.Anthropic,
		apitype.Gemini,
		apitype.Ali,
		apitype.Baidu,
		apitype.Zhipu,
	}

	// Create a map for efficient lookup
	adapterMap := make(map[int]bool)
	for _, adapter := range DefaultGlobalPricingAdapters {
		adapterMap[adapter] = true
	}

	// Verify that all core adapters are present
	for _, expected := range coreAdapters {
		require.True(t, adapterMap[expected], "Expected core adapter %d to be in DefaultGlobalPricingAdapters", expected)
	}

	// Test that we have a reasonable number of adapters (should be more than core but not excessive)
	require.GreaterOrEqual(t, len(DefaultGlobalPricingAdapters), len(coreAdapters))
	require.LessOrEqual(t, len(DefaultGlobalPricingAdapters), 30, "Too many default adapters, consider reducing the list")
}

func TestIsGlobalPricingInitialized(t *testing.T) {
	// Test uninitialized state
	globalPricingManager = &GlobalPricingManager{}
	require.False(t, IsGlobalPricingInitialized(), "Expected global pricing to be uninitialized")

	// Test initialized state
	InitializeGlobalPricingManager(mockGetAdaptor)
	// Force initialization by accessing global pricing
	GetGlobalModelRatio("test-model") // trigger init
	require.True(t, IsGlobalPricingInitialized(), "Expected global pricing to be initialized")
}
