package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestStructuredOutputCostCalculation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		request           *model.GeneralOpenAIRequest
		expectedToolsCost int64
		completionTokens  int
	}{
		{
			name: "Request with JSON schema should have no additional cost",
			request: &model.GeneralOpenAIRequest{
				Model: "gpt-4o",
				ResponseFormat: &model.ResponseFormat{
					Type: "json_schema",
					JsonSchema: &model.JSONSchema{
						Name: "test_schema",
						Schema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"result": map[string]any{
									"type": "string",
								},
							},
						},
					},
				},
			},
			completionTokens:  1000,
			expectedToolsCost: 0,
		},
		{
			name: "Request without response format should have no additional cost",
			request: &model.GeneralOpenAIRequest{
				Model: "gpt-4o",
			},
			completionTokens:  1000,
			expectedToolsCost: 0,
		},
		{
			name: "Request with text response format should have no additional cost",
			request: &model.GeneralOpenAIRequest{
				Model: "gpt-4o",
				ResponseFormat: &model.ResponseFormat{
					Type: "text",
				},
			},
			completionTokens:  1000,
			expectedToolsCost: 0,
		},
		{
			name: "Request with json_schema but no schema should have no additional cost",
			request: &model.GeneralOpenAIRequest{
				Model: "gpt-4o",
				ResponseFormat: &model.ResponseFormat{
					Type: "json_schema",
				},
			},
			completionTokens:  1000,
			expectedToolsCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gin context with proper HTTP request
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Create mock response
			mockResponse := &SlimTextResponse{
				Usage: model.Usage{
					PromptTokens:     100,
					CompletionTokens: tt.completionTokens,
					TotalTokens:      100 + tt.completionTokens,
				},
			}

			responseBody, _ := json.Marshal(mockResponse)
			resp := &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "application/json")

			// Set up context with request
			c.Set(ctxkey.ConvertedRequest, tt.request)

			// Create meta
			meta := &meta.Meta{
				ActualModelName: tt.request.Model,
				ChannelType:     channeltype.OpenAI,
				PromptTokens:    100,
				Mode:            relaymode.ChatCompletions,
			}

			// Create adaptor and call DoResponse
			adaptor := &Adaptor{
				ChannelType: channeltype.OpenAI,
			}

			usage, err := adaptor.DoResponse(c, resp, meta)

			if err != nil {
				t.Fatalf("DoResponse failed: %v", err)
			}

			if usage == nil {
				t.Fatal("Usage should not be nil")
			}

			// Check completion tokens
			if usage.CompletionTokens != tt.completionTokens {
				t.Errorf("Expected completion tokens %d, got %d", tt.completionTokens, usage.CompletionTokens)
			}

			// No structured output surcharge should be applied
			if usage.ToolsCost != tt.expectedToolsCost {
				t.Errorf("Expected ToolsCost %d, got %d", tt.expectedToolsCost, usage.ToolsCost)
			}
		})
	}
}

func TestStructuredOutputCostWithOriginalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test case where the request is stored in RequestModel context key
	request := &model.GeneralOpenAIRequest{
		Model: "gpt-4o",
		ResponseFormat: &model.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &model.JSONSchema{
				Name: "test_schema",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"result": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
	}

	// Setup gin context with proper HTTP request
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Create mock response
	completionTokens := 500
	mockResponse := &SlimTextResponse{
		Usage: model.Usage{
			PromptTokens:     100,
			CompletionTokens: completionTokens,
			TotalTokens:      600,
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "application/json")

	// Set up context with request in RequestModel key (not ConvertedRequest)
	c.Set(ctxkey.RequestModel, request)

	// Create meta
	meta := &meta.Meta{
		ActualModelName: request.Model,
		ChannelType:     channeltype.OpenAI,
		PromptTokens:    100,
		Mode:            relaymode.ChatCompletions,
	}

	// Create adaptor and call DoResponse
	adaptor := &Adaptor{
		ChannelType: channeltype.OpenAI,
	}

	usage, err := adaptor.DoResponse(c, resp, meta)

	if err != nil {
		t.Fatalf("DoResponse failed: %v", err)
	}

	if usage == nil {
		t.Fatal("Usage should not be nil")
	}

	// No structured output surcharge should be applied even when original request is used
	if usage.ToolsCost != 0 {
		t.Errorf("Expected no structured output cost from RequestModel context, but got %d", usage.ToolsCost)
	}
}
