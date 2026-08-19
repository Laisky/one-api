package moonshot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }
func stringPtr(v string) *string    { return &v }

// newTestContext builds the minimal gin context the shared Claude Messages
// conversion needs; it writes into the context, so a nil one panics.
func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

// Kimi K3 answers 400 when temperature and friends carry a non-fixed value, so
// the adaptor must drop them before the request leaves.
func TestStripPinnedSamplingParams_K3DropsPinnedKnobs(t *testing.T) {
	request := &model.GeneralOpenAIRequest{
		Model:            "kimi-k3",
		Temperature:      float64Ptr(0.7),
		TopP:             float64Ptr(0.5),
		N:                intPtr(2),
		PresencePenalty:  float64Ptr(1.5),
		FrequencyPenalty: float64Ptr(-1),
		MaxTokens:        4096,
	}

	stripPinnedSamplingParams(request)

	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.TopP)
	assert.Nil(t, request.N)
	assert.Nil(t, request.PresencePenalty)
	assert.Nil(t, request.FrequencyPenalty)
	// Knobs K3 does accept must survive.
	assert.Equal(t, 4096, request.MaxTokens)
}

// The K2 generation and the V1 SKUs take caller-chosen sampling values; the K3
// rule must not reach them. `n` is the sharp edge here: no Moonshot config
// advertises it, yet these models accept it.
func TestStripPinnedSamplingParams_ConventionalModelsUntouched(t *testing.T) {
	for _, modelName := range []string{"kimi-k2.6", "kimi-k2.7-code", "moonshot-v1-32k"} {
		t.Run(modelName, func(t *testing.T) {
			request := &model.GeneralOpenAIRequest{
				Model:       modelName,
				Temperature: float64Ptr(0.3),
				TopP:        float64Ptr(0.9),
				N:           intPtr(2),
			}

			stripPinnedSamplingParams(request)

			require.NotNil(t, request.Temperature)
			assert.InDelta(t, 0.3, *request.Temperature, 1e-9)
			require.NotNil(t, request.TopP)
			assert.InDelta(t, 0.9, *request.TopP, 1e-9)
			require.NotNil(t, request.N)
			assert.Equal(t, 2, *request.N)
		})
	}
}

func TestStripPinnedSamplingParams_UnknownModelUntouched(t *testing.T) {
	request := &model.GeneralOpenAIRequest{
		Model:       "kimi-some-future-model",
		Temperature: float64Ptr(0.42),
	}

	stripPinnedSamplingParams(request)

	require.NotNil(t, request.Temperature)
	assert.InDelta(t, 0.42, *request.Temperature, 1e-9)
}

func TestNormalizeReasoningEffort(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		effort   *string
		expected *string
	}{
		{"k3 keeps low", "kimi-k3", stringPtr("low"), stringPtr("low")},
		{"k3 keeps high", "kimi-k3", stringPtr("high"), stringPtr("high")},
		{"k3 keeps max", "kimi-k3", stringPtr("max"), stringPtr("max")},
		// The OpenAI ladder is mapped down, never escalated to K3's costly default.
		{"k3 maps medium to high", "kimi-k3", stringPtr("medium"), stringPtr("high")},
		{"k3 maps minimal to low", "kimi-k3", stringPtr("minimal"), stringPtr("low")},
		{"k3 maps none to low", "kimi-k3", stringPtr("none"), stringPtr("low")},
		{"k3 maps xhigh to max", "kimi-k3", stringPtr("xhigh"), stringPtr("max")},
		{"k3 normalizes case and spacing", "kimi-k3", stringPtr("  HIGH "), stringPtr("high")},
		{"k3 drops unknown tier", "kimi-k3", stringPtr("turbo"), nil},
		{"k3 leaves absent field absent", "kimi-k3", nil, nil},
		// Every other Moonshot model rejects reasoning_effort outright.
		{"k2.6 drops effort", "kimi-k2.6", stringPtr("high"), nil},
		{"v1 drops effort", "moonshot-v1-8k", stringPtr("low"), nil},
		{"unknown model drops effort", "kimi-some-future-model", stringPtr("high"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &model.GeneralOpenAIRequest{Model: tt.model, ReasoningEffort: tt.effort}

			normalizeReasoningEffort(request)

			if tt.expected == nil {
				assert.Nil(t, request.ReasoningEffort)
				return
			}
			require.NotNil(t, request.ReasoningEffort)
			assert.Equal(t, *tt.expected, *request.ReasoningEffort)
		})
	}
}

// ConvertRequest is the seam the relay actually calls, so pin the end-to-end
// behaviour for K3 there too.
func TestConvertRequest_K3(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertRequest(nil, 0, &model.GeneralOpenAIRequest{
		Model:           "kimi-k3",
		Temperature:     float64Ptr(0.7),
		ReasoningEffort: stringPtr("medium"),
	})
	require.NoError(t, err)

	request, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Nil(t, request.Temperature)
	require.NotNil(t, request.ReasoningEffort)
	assert.Equal(t, "high", *request.ReasoningEffort)
}

// /v1/messages traffic reaches the same upstream, and the shared Claude
// conversion copies temperature/top_p off the Claude request, so K3 needs the
// pinned-parameter scrub on this path too.
func TestConvertClaudeRequest_K3StripsPinnedSamplingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertClaudeRequest(newTestContext(), &model.ClaudeRequest{
		Model:       "kimi-k3",
		MaxTokens:   1024,
		Temperature: float64Ptr(0.7),
		TopP:        float64Ptr(0.5),
		Messages: []model.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)

	request, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.TopP)
}

// The same path must leave the conventional models' sampling values alone.
func TestConvertClaudeRequest_ConventionalModelKeepsSamplingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertClaudeRequest(newTestContext(), &model.ClaudeRequest{
		Model:       "kimi-k2.6",
		MaxTokens:   1024,
		Temperature: float64Ptr(0.7),
		Messages: []model.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)

	request, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, request.Temperature)
	assert.InDelta(t, 0.7, *request.Temperature, 1e-9)
}

func TestConvertRequest_NilRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	_, err := adaptor.ConvertRequest(nil, 0, nil)

	assert.Error(t, err)
}
