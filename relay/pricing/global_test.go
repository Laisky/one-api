package pricing

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

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
				"gemini-pro": {Ratio: 0.5 * 0.000001, CompletionRatio: 3.0},
				"gpt-4":      {Ratio: 25 * 0.000001, CompletionRatio: 2.5}, // Conflict with OpenAI
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

	if globalPricingManager.getAdaptorFunc == nil {
		t.Error("Expected adaptor function to be set")
	}

	if len(globalPricingManager.contributingAdapters) == 0 {
		t.Error("Expected contributing adapters to be loaded from default configuration")
	}

	// Check that it matches the default adapters
	if len(globalPricingManager.contributingAdapters) != len(DefaultGlobalPricingAdapters) {
		t.Errorf("Expected %d adapters, got %d", len(DefaultGlobalPricingAdapters), len(globalPricingManager.contributingAdapters))
	}
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
	if _, exists := pricing["gpt-3.5-turbo"]; !exists {
		t.Error("Expected gpt-3.5-turbo from OpenAI to be in global pricing")
	}

	// Check Anthropic models
	if _, exists := pricing["claude-3-opus"]; !exists {
		t.Error("Expected claude-3-opus from Anthropic to be in global pricing")
	}

	// Check Gemini models
	if _, exists := pricing["gemini-pro"]; !exists {
		t.Error("Expected gemini-pro from Gemini to be in global pricing")
	}

	// Test conflict resolution (first adapter wins)
	if gpt4Price, exists := pricing["gpt-4"]; exists {
		expectedRatio := 30 * 0.000001 // OpenAI's pricing should win
		if gpt4Price.Ratio != expectedRatio {
			t.Errorf("Expected gpt-4 ratio to be %f (OpenAI), got %f", expectedRatio, gpt4Price.Ratio)
		}
	} else {
		t.Error("Expected gpt-4 to be in global pricing")
	}
}

func TestGetGlobalModelRatio(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test existing model
	ratio := GetGlobalModelRatio("gpt-3.5-turbo")
	expectedRatio := 1.5 * 0.000001
	if ratio != expectedRatio {
		t.Errorf("Expected ratio %f, got %f", expectedRatio, ratio)
	}

	// Test non-existing model
	ratio = GetGlobalModelRatio("non-existent-model")
	if ratio != 0 {
		t.Errorf("Expected 0 for non-existent model, got %f", ratio)
	}
}

func TestGetGlobalCompletionRatio(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test existing model
	ratio := GetGlobalCompletionRatio("claude-3-opus")
	expectedRatio := 5.0
	if ratio != expectedRatio {
		t.Errorf("Expected completion ratio %f, got %f", expectedRatio, ratio)
	}

	// Test non-existing model
	ratio = GetGlobalCompletionRatio("non-existent-model")
	if ratio != 0 {
		t.Errorf("Expected 0 for non-existent model, got %f", ratio)
	}
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
	if ratio != expectedRatio {
		t.Errorf("Expected channel override ratio %f, got %f", expectedRatio, ratio)
	}

	// Test Layer 2: Adapter pricing (second priority)
	ratio = GetModelRatioWithThreeLayers("gpt-4", nil, openaiAdaptor)
	expectedRatio = 30 * 0.000001 // OpenAI's pricing
	if ratio != expectedRatio {
		t.Errorf("Expected adapter ratio %f, got %f", expectedRatio, ratio)
	}

	// Test Layer 3: Global pricing (third priority)
	// Use a model that exists in global pricing but not in the current adapter
	ratio = GetModelRatioWithThreeLayers("claude-3-opus", nil, openaiAdaptor)
	expectedRatio = 15 * 0.000001 // From global pricing (Anthropic)
	if ratio != expectedRatio {
		t.Errorf("Expected global pricing ratio %f, got %f", expectedRatio, ratio)
	}

	// Test Layer 4: Final fallback
	ratio = GetModelRatioWithThreeLayers("completely-unknown-model", nil, openaiAdaptor)
	expectedRatio = 2.5 * 0.000001 // Final fallback
	if ratio != expectedRatio {
		t.Errorf("Expected fallback ratio %f, got %f", expectedRatio, ratio)
	}
}

func TestSetContributingAdapters(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{}
	InitializeGlobalPricingManager(mockGetAdaptor)

	// Test setting new adapters
	newAdapters := []int{apitype.OpenAI}
	SetContributingAdapters(newAdapters)

	adapters := GetContributingAdapters()
	if len(adapters) != 1 || adapters[0] != apitype.OpenAI {
		t.Errorf("Expected [%d], got %v", apitype.OpenAI, adapters)
	}

	// Verify pricing is reloaded
	pricing := GetGlobalModelPricing()
	if _, exists := pricing["gpt-4"]; !exists {
		t.Error("Expected gpt-4 to be in global pricing after adapter change")
	}
	if _, exists := pricing["claude-3-opus"]; exists {
		t.Error("Expected claude-3-opus to NOT be in global pricing after removing Anthropic adapter")
	}
}

func TestGetGlobalPricingStats(t *testing.T) {
	// Setup
	globalPricingManager = &GlobalPricingManager{
		contributingAdapters: []int{apitype.OpenAI, apitype.Anthropic},
	}
	InitializeGlobalPricingManager(mockGetAdaptor)

	modelCount, adapterCount := GetGlobalPricingStats()

	if adapterCount != 2 {
		t.Errorf("Expected 2 adapters, got %d", adapterCount)
	}

	if modelCount == 0 {
		t.Error("Expected some models in global pricing")
	}
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
	if newModelCount <= initialModelCount {
		t.Errorf("Expected more models after reload, initial: %d, new: %d", initialModelCount, newModelCount)
	}
}

func TestDefaultGlobalPricingAdapters(t *testing.T) {
	// Test that the default adapters slice is properly defined
	if len(DefaultGlobalPricingAdapters) == 0 {
		t.Error("DefaultGlobalPricingAdapters should not be empty")
	}

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
		if !adapterMap[expected] {
			t.Errorf("Expected core adapter %d to be in DefaultGlobalPricingAdapters", expected)
		}
	}

	// Test that we have a reasonable number of adapters (should be more than core but not excessive)
	if len(DefaultGlobalPricingAdapters) < len(coreAdapters) {
		t.Errorf("Expected at least %d adapters, got %d", len(coreAdapters), len(DefaultGlobalPricingAdapters))
	}

	if len(DefaultGlobalPricingAdapters) > 30 {
		t.Errorf("Too many default adapters (%d), consider reducing the list", len(DefaultGlobalPricingAdapters))
	}
}

func TestIsGlobalPricingInitialized(t *testing.T) {
	// Test uninitialized state
	globalPricingManager = &GlobalPricingManager{}
	if IsGlobalPricingInitialized() {
		t.Error("Expected global pricing to be uninitialized")
	}

	// Test initialized state
	InitializeGlobalPricingManager(mockGetAdaptor)
	// Force initialization by accessing global pricing
	GetGlobalModelRatio("test-model")
	if !IsGlobalPricingInitialized() {
		t.Error("Expected global pricing to be initialized")
	}
}
