package vertexai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/deepseek"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/qwen"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
	relayModel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestGetDefaultModelPricingIncludesGeminiEmbeddingPreview(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	pricing := a.GetDefaultModelPricing()

	cfg, ok := pricing["gemini-embedding-2-preview"]
	require.True(t, ok, "gemini-embedding-2-preview missing from Vertex AI pricing map")
	require.InDelta(t, 0.20*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 1.0, cfg.CompletionRatio, 1e-12)
	require.Equal(t, geminiOpenaiCompatible.ModelRatios["gemini-embedding-2-preview"], cfg)
}

func TestGetModelListIncludesGeminiEmbeddingPreviewOnce(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	models := a.GetModelList()

	count := 0
	for _, model := range models {
		if model == "gemini-embedding-2-preview" {
			count++
		}
	}

	require.Equal(t, 1, count, "expected gemini-embedding-2-preview exactly once in Vertex AI model list")
}

func TestGetDefaultModelPricingIncludesLatestVertexMaaSModels(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	pricing := a.GetDefaultModelPricing()

	deepseekCfg, ok := pricing["deepseek-ai/deepseek-v3.2-maas"]
	require.True(t, ok, "deepseek-ai/deepseek-v3.2-maas missing from Vertex AI pricing map")
	require.Equal(t, deepseek.ModelRatios["deepseek-ai/deepseek-v3.2-maas"], deepseekCfg)

	qwenCfg, ok := pricing["qwen/qwen3-next-80b-a3b-thinking-maas"]
	require.True(t, ok, "qwen/qwen3-next-80b-a3b-thinking-maas missing from Vertex AI pricing map")
	require.Equal(t, qwen.ModelRatios["qwen/qwen3-next-80b-a3b-thinking-maas"], qwenCfg)
}

// TestBuildGeminiURLUsesBatchEmbedContents verifies Vertex AI Gemini embeddings use the batch embedding endpoint.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestBuildGeminiURLUsesBatchEmbedContents(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	url, err := adaptor.buildGeminiURL(&meta.Meta{
		ActualModelName: "gemini-embedding-2-preview",
		Mode:            relaymode.Embeddings,
		Config: model.ChannelConfig{
			Region:            "us-central1",
			VertexAIProjectID: "test-project",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-embedding-2-preview:batchEmbedContents", url)
}

func TestAdaptor_GetRequestURL(t *testing.T) {
	Convey("GetRequestURL", t, func() {
		adaptor := &Adaptor{}

		Convey("imagen models should use predict endpoint", func() {
			imagenTests := []struct {
				model  string
				region string
			}{
				{"imagen-4.0-fast-generate-001", "us-central1"},
				{"imagen-4.0-generate-001", "us-central1"},
				{"imagen-3.0-fast-generate-001", "europe-west4"},
				{"imagegeneration@006", "asia-southeast1"},
			}

			for _, tc := range imagenTests {
				Convey(tc.model+" predict", func() {
					meta := &meta.Meta{
						ActualModelName: tc.model,
						IsStream:        false,
						Config: model.ChannelConfig{
							Region:            tc.region,
							VertexAIProjectID: "test-project",
						},
					}

					url, err := adaptor.GetRequestURL(meta)
					So(err, ShouldBeNil)
					expectedURL := "https://" + tc.region + "-aiplatform.googleapis.com/v1/projects/test-project/locations/" + tc.region + "/publishers/google/models/" + tc.model + ":predict"
					So(url, ShouldEqual, expectedURL)
				})
			}
		})

		Convey("veo models should use predictLongRunning endpoint", func() {
			veoTests := []struct {
				model  string
				region string
			}{
				{"veo-3.1-generate-001", "us-central1"},
				{"veo-3.1-fast-generate-preview", "europe-west4"},
				{"veo-2.0-generate-preview", "asia-southeast1"},
			}

			for _, tc := range veoTests {
				Convey(tc.model+" predictLongRunning", func() {
					meta := &meta.Meta{
						ActualModelName: tc.model,
						IsStream:        false,
						Config: model.ChannelConfig{
							Region:            tc.region,
							VertexAIProjectID: "test-project",
						},
					}

					url, err := adaptor.GetRequestURL(meta)
					So(err, ShouldBeNil)
					expectedURL := "https://" + tc.region + "-aiplatform.googleapis.com/v1/projects/test-project/locations/" + tc.region + "/publishers/google/models/" + tc.model + ":predictLongRunning"
					So(url, ShouldEqual, expectedURL)
				})
			}
		})

		Convey("gemini models >=2.5 should use global endpoint", func() {
			testCases := []struct {
				name           string
				modelName      string
				isStream       bool
				expectedHost   string
				expectedLoc    string
				expectedSuffix string
			}{
				{
					name:           "gemini-2.5-pro non-stream",
					modelName:      "gemini-2.5-pro",
					isStream:       false,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "generateContent",
				},
				{
					name:           "gemini-2.5-flash stream",
					modelName:      "gemini-2.5-flash",
					isStream:       true,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "streamGenerateContent?alt=sse",
				},
				{
					name:           "gemini-2.6-pro-preview non-stream",
					modelName:      "gemini-2.6-pro-preview",
					isStream:       false,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "generateContent",
				},
				{
					name:           "gemini-2.7-flash stream",
					modelName:      "gemini-2.7-flash",
					isStream:       true,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "streamGenerateContent?alt=sse",
				},
				{
					name:           "gemini-3-pro-preview",
					modelName:      "gemini-3-pro-preview",
					isStream:       false,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "generateContent",
				},
				{
					name:           "gemini-4-ultra",
					modelName:      "gemini-4-ultra",
					isStream:       false,
					expectedHost:   "aiplatform.googleapis.com",
					expectedLoc:    "global",
					expectedSuffix: "generateContent",
				},
			}

			for _, tc := range testCases {
				Convey(tc.name, func() {
					meta := &meta.Meta{
						ActualModelName: tc.modelName,
						IsStream:        tc.isStream,
						Config: model.ChannelConfig{
							Region:            "us-central1",
							VertexAIProjectID: "test-project",
						},
					}

					url, err := adaptor.GetRequestURL(meta)
					So(err, ShouldBeNil)

					expectedURL := "https://" + tc.expectedHost + "/v1/projects/test-project/locations/" + tc.expectedLoc + "/publishers/google/models/" + tc.modelName + ":" + tc.expectedSuffix
					So(url, ShouldEqual, expectedURL)
				})
			}
		})

		Convey("gemini models <2.5 should use configured region", func() {
			testCases := []struct {
				name           string
				modelName      string
				isStream       bool
				region         string
				expectedSuffix string
			}{
				{
					name:           "gemini-2.0-flash non-stream",
					modelName:      "gemini-2.0-flash",
					isStream:       false,
					region:         "us-central1",
					expectedSuffix: "generateContent",
				},
				{
					name:           "gemini-2.0-flash-lite stream",
					modelName:      "gemini-2.0-flash-lite",
					isStream:       true,
					region:         "europe-west4",
					expectedSuffix: "streamGenerateContent?alt=sse",
				},
			}

			for _, tc := range testCases {
				Convey(tc.name, func() {
					meta := &meta.Meta{
						ActualModelName: tc.modelName,
						IsStream:        tc.isStream,
						Config: model.ChannelConfig{
							Region:            tc.region,
							VertexAIProjectID: "test-project",
						},
					}

					url, err := adaptor.GetRequestURL(meta)
					So(err, ShouldBeNil)

					expectedURL := "https://" + tc.region + "-aiplatform.googleapis.com/v1/projects/test-project/locations/" + tc.region + "/publishers/google/models/" + tc.modelName + ":" + tc.expectedSuffix
					So(url, ShouldEqual, expectedURL)
				})
			}
		})

		Convey("non-gemini models should use regional endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "claude-3-sonnet",
				IsStream:        false,
				Config: model.ChannelConfig{
					Region:            "us-central1",
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)

			expectedURL := "https://us-central1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-3-sonnet:rawPredict"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("custom BaseURL should work for all models", func() {
			testCases := []struct {
				name        string
				modelName   string
				isStream    bool
				expectedLoc string
				suffix      string
			}{
				{
					name:        "gemini-3-pro-preview with custom BaseURL",
					modelName:   "gemini-3-pro-preview",
					isStream:    false,
					expectedLoc: "global",
					suffix:      "generateContent",
				},
				{
					name:        "regular gemini with custom BaseURL",
					modelName:   "gemini-2.0-flash",
					isStream:    false,
					expectedLoc: "us-central1",
					suffix:      "generateContent",
				},
			}

			for _, tc := range testCases {
				Convey(tc.name, func() {
					customBaseURL := "https://custom-vertex-proxy.example.com"
					meta := &meta.Meta{
						ActualModelName: tc.modelName,
						IsStream:        tc.isStream,
						BaseURL:         customBaseURL,
						Config: model.ChannelConfig{
							Region:            "us-central1",
							VertexAIProjectID: "test-project",
						},
					}

					url, err := adaptor.GetRequestURL(meta)
					So(err, ShouldBeNil)

					expectedURL := customBaseURL + "/v1/projects/test-project/locations/" + tc.expectedLoc + "/publishers/google/models/" + tc.modelName + ":" + tc.suffix
					So(url, ShouldEqual, expectedURL)
				})
			}
		})
	})
}

func TestIsRequireGlobalEndpoint(t *testing.T) {
	Convey("IsRequireGlobalEndpoint", t, func() {
		testCases := []struct {
			model    string
			expected bool
		}{
			{"gemini-2.5-pro-preview", true},
			{"gemini-2.5001-pro", true},
			{"gemini-2.6-flash", true},
			{"gemini-3-pro-preview", true},
			{"gemini-10-ultra", true},
			{"gemini-2.5-flash-lite", true},
			{"gemini-2.1", false},
			{"gemini-2.0-flash", false},
			{"gemini-2.0-flash-lite", false},
			{"claude-3-sonnet", false},
			{"gpt-4", false},
			{"imagen-3.0", false},
			{"", false},
		}

		for _, tc := range testCases {
			Convey("model "+tc.model, func() {
				result := IsRequireGlobalEndpoint(tc.model)
				So(result, ShouldEqual, tc.expected)
			})
		}
	})
}

func TestIsDeepSeekModel(t *testing.T) {
	Convey("isDeepSeekModel", t, func() {
		cases := []struct {
			model    string
			expected bool
		}{
			{"deepseek-ai/deepseek-v3.1-maas", true},
			{"deepseek-ai/deepseek-r1-0528-maas", true},
			{"deepseek-ai/deepseek-v2", true},
			{"gemini-2.5-pro", false},
			{"claude-3-sonnet", false},
			{"", false},
		}
		for _, c := range cases {
			Convey("model "+c.model, func() {
				So(isDeepSeekModel(c.model), ShouldEqual, c.expected)
			})
		}
	})
}

func TestDeepSeekRequestURL(t *testing.T) {
	Convey("DeepSeek GetRequestURL", t, func() {
		adaptor := &Adaptor{}

		Convey("deepseek-v3.1-maas should use us-west2 endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "deepseek-ai/deepseek-v3.1-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://us-west2-aiplatform.googleapis.com/v1/projects/test-project/locations/us-west2/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("deepseek-r1-0528-maas should use us-central1 endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "deepseek-ai/deepseek-r1-0528-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://us-central1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("deepseek models with custom BaseURL and region", func() {
			meta := &meta.Meta{
				ActualModelName: "deepseek-ai/deepseek-v3.1-maas",
				IsStream:        false,
				BaseURL:         "https://custom-deepseek-proxy.example.com",
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
					Region:            "us-west1",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://custom-deepseek-proxy.example.com/v1/projects/test-project/locations/us-west1/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("deepseek models without project ID should return error", func() {
			meta := &meta.Meta{
				ActualModelName: "deepseek-ai/deepseek-v3.1-maas",
				IsStream:        false,
				Config:          model.ChannelConfig{
					// Missing VertexAI project ID
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldNotBeNil)
			So(url, ShouldEqual, "")
			So(err.Error(), ShouldContainSubstring, "VertexAI project ID is required")
		})
	})
}

func TestDeepSeekConvertRequest(t *testing.T) {
	Convey("DeepSeek ConvertRequest", t, func() {
		adaptor := &deepseek.Adaptor{}

		Convey("should convert max_completion_tokens to max_tokens", func() {
			maxCompletionTokens := 1000
			request := &relayModel.GeneralOpenAIRequest{
				Model:               "deepseek-ai/deepseek-v3.1-maas",
				MaxTokens:           0, // Not set
				MaxCompletionTokens: &maxCompletionTokens,
				Messages: []relayModel.Message{
					{Role: "user", Content: "Hello"},
				},
			}

			result, err := adaptor.ConvertRequest(nil, 0, request)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			convertedRequest := result.(*relayModel.GeneralOpenAIRequest)
			So(convertedRequest.MaxTokens, ShouldEqual, 1000)
			So(convertedRequest.MaxCompletionTokens, ShouldBeNil)
		})

		Convey("should preserve max_tokens if already set", func() {
			maxCompletionTokens := 1000
			request := &relayModel.GeneralOpenAIRequest{
				Model:               "deepseek-ai/deepseek-v3.1-maas",
				MaxTokens:           500, // Already set
				MaxCompletionTokens: &maxCompletionTokens,
				Messages: []relayModel.Message{
					{Role: "user", Content: "Hello"},
				},
			}

			result, err := adaptor.ConvertRequest(nil, 0, request)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			convertedRequest := result.(*relayModel.GeneralOpenAIRequest)
			So(convertedRequest.MaxTokens, ShouldEqual, 500) // Should preserve original
			So(convertedRequest.MaxCompletionTokens, ShouldBeNil)
		})

		Convey("should handle nil request", func() {
			result, err := adaptor.ConvertRequest(nil, 0, nil)
			So(err, ShouldNotBeNil)
			So(result, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "request is nil")
		})
	})
}

func TestIsImagenModel(t *testing.T) {
	Convey("isImagenModel", t, func() {
		cases := []struct {
			model    string
			expected bool
		}{
			{"imagen-4.0-fast-generate-001", true},
			{"imagen-3.0-generate-002", true},
			{"imagegeneration@006", true},
			{"gemini-2.5-pro", false},
			{"claude-3-sonnet", false},
			{"", false},
		}
		for _, c := range cases {
			Convey("model "+c.model, func() {
				So(isImagenModel(c.model), ShouldEqual, c.expected)
			})
		}
	})
}

func TestIsOpenAIModel(t *testing.T) {
	Convey("isOpenAIModel", t, func() {
		cases := []struct {
			model    string
			expected bool
		}{
			{"openai/gpt-oss-20b-maas", true},
			{"openai/gpt-oss-120b-maas", true},
			{"gemini-2.5-pro", false},
			{"claude-3-sonnet", false},
			{"", false},
		}
		for _, c := range cases {
			Convey("model "+c.model, func() {
				So(isOpenAIModel(c.model), ShouldEqual, c.expected)
			})
		}
	})
}

func TestIsVeoModel(t *testing.T) {
	Convey("isVeoModel", t, func() {
		cases := []struct {
			model    string
			expected bool
		}{
			{"veo-3.1-generate-001", true},
			{"veo-3.0-fast-generate-preview", true},
			{"veo-2.0-generate-exp", true},
			{"imagen-4.0-generate-001", false},
			{"gemini-2.5-pro", false},
			{"", false},
		}
		for _, c := range cases {
			Convey("model "+c.model, func() {
				So(isVeoModel(c.model), ShouldEqual, c.expected)
			})
		}
	})
}

func TestIsQwenModel(t *testing.T) {
	Convey("isQwenModel", t, func() {
		cases := []struct {
			model    string
			expected bool
		}{
			{"qwen/qwen3-coder-480b-a35b-instruct-maas", true},
			{"qwen/qwen3-235b-a22b-instruct-2507-maas", true},
			{"qwen/qwen3-next-80b-a3b-instruct-maas", true},
			{"qwen/qwen-turbo", true},
			{"qwen/qwen-plus", true},
			{"openai/gpt-oss-20b-maas", false},
			{"gemini-2.5-pro", false},
			{"claude-3-sonnet", false},
			{"", false},
		}
		for _, c := range cases {
			Convey("model "+c.model, func() {
				So(isQwenModel(c.model), ShouldEqual, c.expected)
			})
		}
	})
}

func TestQwenRequestURL(t *testing.T) {
	Convey("Qwen GetRequestURL", t, func() {
		adaptor := &Adaptor{}

		Convey("qwen3-coder-480b-a35b-instruct-maas should use us-south1 endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "qwen/qwen3-coder-480b-a35b-instruct-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://us-south1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("qwen3-235b-a22b-instruct-2507-maas should use us-south1 endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "qwen/qwen3-235b-a22b-instruct-2507-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://us-south1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("qwen3-next-80b-a3b-instruct-maas should use global endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "qwen/qwen3-next-80b-a3b-instruct-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://aiplatform.googleapis.com/v1/projects/test-project/locations/global/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("qwen models with custom BaseURL and region", func() {
			meta := &meta.Meta{
				ActualModelName: "qwen/qwen3-235b-a22b-instruct-2507-maas",
				IsStream:        false,
				BaseURL:         "https://custom-qwen-proxy.example.com",
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
					Region:            "us-west1",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://custom-qwen-proxy.example.com/v1/projects/test-project/locations/us-west1/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("qwen models without project ID should return error", func() {
			meta := &meta.Meta{
				ActualModelName: "qwen/qwen3-235b-a22b-instruct-2507-maas",
				IsStream:        false,
				Config:          model.ChannelConfig{
					// Missing VertexAI project ID
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldNotBeNil)
			So(url, ShouldEqual, "")
			So(err.Error(), ShouldContainSubstring, "VertexAI project ID is required")
		})
	})
}

func TestQwenConvertRequest(t *testing.T) {
	Convey("Qwen ConvertRequest", t, func() {
		adaptor := &qwen.Adaptor{}

		Convey("should convert max_completion_tokens to max_tokens", func() {
			maxCompletionTokens := 1000
			request := &relayModel.GeneralOpenAIRequest{
				Model:               "qwen/qwen3-coder-480b-a35b-instruct-maas",
				MaxTokens:           0, // Not set
				MaxCompletionTokens: &maxCompletionTokens,
				Messages: []relayModel.Message{
					{Role: "user", Content: "Hello"},
				},
			}

			result, err := adaptor.ConvertRequest(nil, 0, request)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			convertedRequest := result.(*relayModel.GeneralOpenAIRequest)
			So(convertedRequest.MaxTokens, ShouldEqual, 1000)
			So(convertedRequest.MaxCompletionTokens, ShouldBeNil)
		})

		Convey("should preserve max_tokens if already set", func() {
			maxCompletionTokens := 1000
			request := &relayModel.GeneralOpenAIRequest{
				Model:               "qwen/qwen3-coder-480b-a35b-instruct-maas",
				MaxTokens:           500, // Already set
				MaxCompletionTokens: &maxCompletionTokens,
				Messages: []relayModel.Message{
					{Role: "user", Content: "Hello"},
				},
			}

			result, err := adaptor.ConvertRequest(nil, 0, request)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			convertedRequest := result.(*relayModel.GeneralOpenAIRequest)
			So(convertedRequest.MaxTokens, ShouldEqual, 500) // Should preserve original
			So(convertedRequest.MaxCompletionTokens, ShouldBeNil)
		})

		Convey("should handle nil request", func() {
			result, err := adaptor.ConvertRequest(nil, 0, nil)
			So(err, ShouldNotBeNil)
			So(result, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "request is nil")
		})
	})
}

func TestOpenAIRequestURL(t *testing.T) {
	Convey("OpenAI GetRequestURL", t, func() {
		adaptor := &Adaptor{}

		Convey("openai/gpt-oss-20b-maas should use global endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "openai/gpt-oss-20b-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://aiplatform.googleapis.com/v1/projects/test-project/locations/global/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("openai/gpt-oss-120b-maas should use global endpoint", func() {
			meta := &meta.Meta{
				ActualModelName: "openai/gpt-oss-120b-maas",
				IsStream:        false,
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://aiplatform.googleapis.com/v1/projects/test-project/locations/global/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("openai models with custom BaseURL", func() {
			meta := &meta.Meta{
				ActualModelName: "openai/gpt-oss-20b-maas",
				IsStream:        false,
				BaseURL:         "https://custom-openai-proxy.example.com",
				Config: model.ChannelConfig{
					VertexAIProjectID: "test-project",
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldBeNil)
			expectedURL := "https://custom-openai-proxy.example.com/v1/projects/test-project/locations/global/endpoints/openapi/chat/completions"
			So(url, ShouldEqual, expectedURL)
		})

		Convey("openai models without project ID should return error", func() {
			meta := &meta.Meta{
				ActualModelName: "openai/gpt-oss-20b-maas",
				IsStream:        false,
				Config:          model.ChannelConfig{
					// Missing VertexAI project ID
				},
			}

			url, err := adaptor.GetRequestURL(meta)
			So(err, ShouldNotBeNil)
			So(url, ShouldEqual, "")
			So(err.Error(), ShouldContainSubstring, "VertexAI project ID is required")
		})
	})
}

func TestGetQwenEndpointConfig(t *testing.T) {
	Convey("getQwenEndpointConfig", t, func() {
		testCases := []struct {
			model            string
			expectedHost     string
			expectedLocation string
		}{
			{
				model:            "qwen/qwen3-next-80b-a3b-instruct-maas",
				expectedHost:     "aiplatform.googleapis.com",
				expectedLocation: "global",
			},
			{
				model:            "qwen/qwen3-coder-480b-a35b-instruct-maas",
				expectedHost:     "us-south1-aiplatform.googleapis.com",
				expectedLocation: "us-central1",
			},
			{
				model:            "qwen/qwen3-235b-a22b-instruct-2507-maas",
				expectedHost:     "us-south1-aiplatform.googleapis.com",
				expectedLocation: "us-central1",
			},
			{
				model:            "qwen/unknown-model",
				expectedHost:     "us-south1-aiplatform.googleapis.com",
				expectedLocation: "us-central1",
			},
		}

		for _, tc := range testCases {
			Convey("model "+tc.model, func() {
				host, location := getQwenEndpointConfig(tc.model)
				So(host, ShouldEqual, tc.expectedHost)
				So(location, ShouldEqual, tc.expectedLocation)
			})
		}
	})
}
