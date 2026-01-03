package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// Test data constants
const (
	testModelName    = "gpt-4-turbo"
	testContentType  = "application/json"
	testPromptTokens = 100
)

// Helper function to create a test logger
func createTestLogger() *log.LoggerT {
	logger, _ := log.New(log.WithLevel("debug"))
	return logger
}

// Helper function to create test streaming response
func createTestStreamResponse(id string, content string, usage *model.Usage) *ChatCompletionsStreamResponse {
	return &ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   testModelName,
		Choices: []ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: model.Message{Content: content},
			},
		},
		Usage: usage,
	}
}

// Helper function to create test streaming response with tool calls
func createTestStreamResponseWithToolCalls(id string, toolArgs any) *ChatCompletionsStreamResponse {
	toolCall := model.Tool{
		Function: &model.Function{
			Arguments: toolArgs,
		},
	}

	return &ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   testModelName,
		Choices: []ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: model.Message{
					ToolCalls: []model.Tool{toolCall},
				},
			},
		},
	}
}

// TestThinkingProcessor_ProcessThinkingContent tests the ThinkingProcessor functionality
func TestThinkingProcessor_ProcessThinkingContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		input             string
		expectedContent   string
		expectedReasoning *string
		expectedModified  bool
		setupFunc         func(*ThinkingProcessor)
	}{
		{
			name:              "Empty input",
			input:             "",
			expectedContent:   "",
			expectedReasoning: nil,
			expectedModified:  false,
		},
		{
			name:              "No thinking tag",
			input:             "Hello world",
			expectedContent:   "Hello world",
			expectedReasoning: nil,
			expectedModified:  false,
		},
		{
			name:              "Complete thinking block in single chunk",
			input:             "Hello <think>reasoning here</think> world",
			expectedContent:   "Hello  world",
			expectedReasoning: stringPtr("reasoning here"),
			expectedModified:  true,
		},
		{
			name:              "Thinking block at start",
			input:             "<think>reasoning</think>content",
			expectedContent:   "content",
			expectedReasoning: stringPtr("reasoning"),
			expectedModified:  true,
		},
		{
			name:              "Thinking block at end",
			input:             "content<think>reasoning</think>",
			expectedContent:   "content",
			expectedReasoning: stringPtr("reasoning"),
			expectedModified:  true,
		},
		{
			name:              "Empty thinking block",
			input:             "Hello <think></think> world",
			expectedContent:   "Hello  world",
			expectedReasoning: nil,
			expectedModified:  true,
		},
		{
			name:              "Incomplete thinking block - opening tag only",
			input:             "Hello <think>partial reasoning",
			expectedContent:   "Hello ",
			expectedReasoning: stringPtr("partial reasoning"),
			expectedModified:  true,
		},
		{
			name:              "Continue inside thinking block",
			input:             " more reasoning",
			expectedContent:   "",
			expectedReasoning: stringPtr(" more reasoning"),
			expectedModified:  true,
			setupFunc: func(tp *ThinkingProcessor) {
				tp.isInThinkingBlock = true
			},
		},
		{
			name:              "Close thinking block",
			input:             " final reasoning</think> world",
			expectedContent:   " world",
			expectedReasoning: stringPtr(" final reasoning"),
			expectedModified:  true,
			setupFunc: func(tp *ThinkingProcessor) {
				tp.isInThinkingBlock = true
			},
		},
		{
			name:              "Already processed thinking tag",
			input:             "Hello <think>should be ignored</think> world",
			expectedContent:   "Hello <think>should be ignored</think> world",
			expectedReasoning: nil,
			expectedModified:  false,
			setupFunc: func(tp *ThinkingProcessor) {
				tp.hasProcessedThinkTag = true
			},
		},
		{
			name:              "Multiple thinking blocks - only first processed",
			input:             "Hello <think>first</think> middle <think>second</think> world",
			expectedContent:   "Hello  middle <think>second</think> world",
			expectedReasoning: stringPtr("first"),
			expectedModified:  true,
		},
		{
			name:              "JSON-escaped Unicode thinking block (complete)",
			input:             "Hello \\u003cthink\\u003ereasoning\\u003c/think\\u003e world",
			expectedContent:   "Hello  world",
			expectedReasoning: stringPtr("reasoning"),
			expectedModified:  true,
		},
		{
			name:              "JSON-escaped Unicode thinking block (opening only)",
			input:             "Hello \\u003cthink\\u003epartial reasoning",
			expectedContent:   "Hello ",
			expectedReasoning: stringPtr("partial reasoning"),
			expectedModified:  true,
		},
		{
			name:              "Mixed normal and Unicode thinking blocks",
			input:             "Hello \\u003cthink\\u003eunicode first\\u003c/think\\u003e <think>normal second</think>",
			expectedContent:   "Hello  <think>normal second</think>",
			expectedReasoning: stringPtr("unicode first"),
			expectedModified:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := &ThinkingProcessor{}
			if tt.setupFunc != nil {
				tt.setupFunc(tp)
			}

			content, reasoning, modified := tp.ProcessThinkingContent(tt.input)

			require.Equal(t, tt.expectedContent, content, "content mismatch")

			if tt.expectedReasoning == nil {
				require.Nil(t, reasoning, "expected nil reasoning")
			} else {
				require.NotNil(t, reasoning, "expected non-nil reasoning")
				require.Equal(t, *tt.expectedReasoning, *reasoning, "reasoning mismatch")
			}

			require.Equal(t, tt.expectedModified, modified, "modified mismatch")
		})
	}
}

// TestStreamingContext_NewStreamingContext tests context initialization
func TestStreamingContext_NewStreamingContext(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()

	tests := []struct {
		name           string
		enableThinking bool
		expectThinking bool
	}{
		{
			name:           "With thinking processor",
			enableThinking: true,
			expectThinking: true,
		},
		{
			name:           "Without thinking processor",
			enableThinking: false,
			expectThinking: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := NewStreamingContext(logger, tt.enableThinking)

			require.NotNil(t, ctx, "expected non-nil context")
			require.Equal(t, logger, ctx.logger, "logger not properly set")

			if tt.expectThinking {
				require.NotNil(t, ctx.thinkingProcessor, "expected thinking processor to be created")
			} else {
				require.Nil(t, ctx.thinkingProcessor, "expected thinking processor to be nil")
			}

			// Check buffer pre-allocation
			require.NotZero(t, ctx.responseTextBuilder.Cap(), "expected responseTextBuilder to be pre-allocated")
			require.NotZero(t, ctx.toolArgsTextBuilder.Cap(), "expected toolArgsTextBuilder to be pre-allocated")

			// Verify initial state
			require.Zero(t, ctx.chunksProcessed, "expected chunksProcessed to be 0")
			require.False(t, ctx.doneRendered, "expected doneRendered to be false")
		})
	}
}

// TestStreamingContext_ProcessStreamChunk tests chunk processing
func TestStreamingContext_ProcessStreamChunk(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()

	tests := []struct {
		name           string
		enableThinking bool
		response       *ChatCompletionsStreamResponse
		expectModified bool
	}{
		{
			name:           "Basic content chunk",
			enableThinking: false,
			response:       createTestStreamResponse("test-1", "Hello world", nil),
			expectModified: true,
		},
		{
			name:           "Content with thinking block",
			enableThinking: true,
			response:       createTestStreamResponse("test-2", "Hello <think>reasoning</think> world", nil),
			expectModified: true,
		},
		{
			name:           "Chunk with usage",
			enableThinking: false,
			response: createTestStreamResponse("test-3", "Hello", &model.Usage{
				PromptTokens:     50,
				CompletionTokens: 25,
				TotalTokens:      75,
			}),
			expectModified: true,
		},
		{
			name:           "String tool call arguments",
			enableThinking: false,
			response:       createTestStreamResponseWithToolCalls("test-4", `{"arg": "value"}`),
			expectModified: true,
		},
		{
			name:           "Object tool call arguments",
			enableThinking: false,
			response:       createTestStreamResponseWithToolCalls("test-5", map[string]any{"arg": "value"}),
			expectModified: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := NewStreamingContext(logger, tt.enableThinking)
			originalContent := ""
			if len(tt.response.Choices) > 0 {
				originalContent = tt.response.Choices[0].Delta.StringContent()
			}

			modified := ctx.ProcessStreamChunk(tt.response)

			require.Equal(t, tt.expectModified, modified, "modified mismatch")

			// Verify content accumulation
			responseText := ctx.responseTextBuilder.String()
			// Note: responseTextBuilder always contains original content (before thinking processing)
			// The thinking processing modifies the response.Choices[].Delta.Content but not the builder
			if originalContent != "" {
				require.Contains(t, responseText, originalContent, "expected original content to be preserved in responseTextBuilder")
			}

			// Verify usage accumulation
			if tt.response.Usage != nil {
				require.Equal(t, tt.response.Usage, ctx.usage, "expected usage to be accumulated")
			}

			// Verify counter increment
			require.Equal(t, 1, ctx.chunksProcessed, "expected chunksProcessed to be 1")
		})
	}
}

// TestStreamingContext_ManageBufferCapacity tests buffer management
func TestStreamingContext_ManageBufferCapacity(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()
	ctx := NewStreamingContext(logger, false)

	// Test functional behavior rather than exact capacity values
	// Force buffer to significantly exceed max capacity
	largeContent := strings.Repeat("x", MaxBuilderCapacity+100000) // Much larger to ensure management kicks in
	ctx.responseTextBuilder.WriteString(largeContent)
	ctx.toolArgsTextBuilder.WriteString(largeContent)

	// Check capacity before management
	responseCapBefore := ctx.responseTextBuilder.Cap()

	if responseCapBefore <= MaxBuilderCapacity {
		t.Skip("Buffer didn't exceed MaxBuilderCapacity, skipping capacity management test")
	}

	// Trigger capacity management
	ctx.ManageBufferCapacity()

	// Verify content was preserved (this is the most important functional test)
	require.Equal(t, largeContent, ctx.responseTextBuilder.String(), "response content not preserved during capacity management")
	require.Equal(t, largeContent, ctx.toolArgsTextBuilder.String(), "tool args content not preserved during capacity management")

	// Check that ManageBufferCapacity method runs without panicking
	// Additional management calls should be safe
	ctx.ManageBufferCapacity()
	ctx.ManageBufferCapacity()

	// Verify content is still preserved after multiple management calls
	require.Equal(t, largeContent, ctx.responseTextBuilder.String(), "response content not preserved after multiple capacity management calls")
	require.Equal(t, largeContent, ctx.toolArgsTextBuilder.String(), "tool args content not preserved after multiple capacity management calls")
}

// TestStreamingContext_CalculateUsage tests usage calculation
func TestStreamingContext_CalculateUsage(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()

	tests := []struct {
		name                     string
		setupFunc                func(*StreamingContext)
		promptTokens             int
		expectedPromptTokens     int
		expectedCompletionTokens int
		expectedTotalTokens      int
		expectFallback           bool
	}{
		{
			name: "No upstream usage - fallback calculation",
			setupFunc: func(ctx *StreamingContext) {
				ctx.responseTextBuilder.WriteString("Hello world")
				ctx.toolArgsTextBuilder.WriteString(`{"arg": "value"}`)
			},
			promptTokens:             testPromptTokens,
			expectedPromptTokens:     testPromptTokens,
			expectedCompletionTokens: (len("Hello world") + len(`{"arg": "value"}`)) / 4, // CountTokenText estimation
			expectedTotalTokens:      testPromptTokens + (len("Hello world")+len(`{"arg": "value"}`))/4,
			expectFallback:           true,
		},
		{
			name: "Complete upstream usage",
			setupFunc: func(ctx *StreamingContext) {
				ctx.usage = &model.Usage{
					PromptTokens:     testPromptTokens,
					CompletionTokens: 50,
					TotalTokens:      testPromptTokens + 50,
				}
			},
			promptTokens:             testPromptTokens,
			expectedPromptTokens:     testPromptTokens,
			expectedCompletionTokens: 50,
			expectedTotalTokens:      testPromptTokens + 50,
			expectFallback:           false,
		},
		{
			name: "Partial upstream usage - missing completion tokens",
			setupFunc: func(ctx *StreamingContext) {
				ctx.responseTextBuilder.WriteString("Hello world")
				ctx.usage = &model.Usage{
					PromptTokens: testPromptTokens,
					TotalTokens:  0, // Will be calculated
				}
			},
			promptTokens:             testPromptTokens,
			expectedPromptTokens:     testPromptTokens,
			expectedCompletionTokens: len("Hello world") / 4,
			expectedTotalTokens:      testPromptTokens + len("Hello world")/4,
			expectFallback:           false,
		},
		{
			name: "Partial upstream usage - missing prompt tokens",
			setupFunc: func(ctx *StreamingContext) {
				ctx.usage = &model.Usage{
					CompletionTokens: 50,
					TotalTokens:      0, // Will be calculated
				}
			},
			promptTokens:             testPromptTokens,
			expectedPromptTokens:     testPromptTokens,
			expectedCompletionTokens: 50,
			expectedTotalTokens:      testPromptTokens + 50,
			expectFallback:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := NewStreamingContext(logger, false)
			if tt.setupFunc != nil {
				tt.setupFunc(ctx)
			}

			usage := ctx.CalculateUsage(tt.promptTokens, testModelName)

			require.NotNil(t, usage, "expected non-nil usage")
			require.Equal(t, tt.expectedPromptTokens, usage.PromptTokens, "prompt tokens mismatch")
			require.Equal(t, tt.expectedCompletionTokens, usage.CompletionTokens, "completion tokens mismatch")
			require.Equal(t, tt.expectedTotalTokens, usage.TotalTokens, "total tokens mismatch")
		})
	}
}

// TestStreamingContext_ValidateStreamCompletion tests stream validation
func TestStreamingContext_ValidateStreamCompletion(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()

	tests := []struct {
		name        string
		setupFunc   func(*StreamingContext)
		expectValid bool
		expectError bool
	}{
		{
			name: "Valid stream - chunks processed",
			setupFunc: func(ctx *StreamingContext) {
				ctx.chunksProcessed = 5
			},
			expectValid: true,
			expectError: false,
		},
		{
			name: "Valid stream - content present",
			setupFunc: func(ctx *StreamingContext) {
				ctx.responseTextBuilder.WriteString("Hello world")
			},
			expectValid: true,
			expectError: false,
		},
		{
			name: "Valid stream - both chunks and content",
			setupFunc: func(ctx *StreamingContext) {
				ctx.chunksProcessed = 3
				ctx.responseTextBuilder.WriteString("Hello world")
			},
			expectValid: true,
			expectError: false,
		},
		{
			name:        "Invalid stream - no chunks or content",
			setupFunc:   nil,
			expectValid: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := NewStreamingContext(logger, false)
			if tt.setupFunc != nil {
				tt.setupFunc(ctx)
			}

			err, valid := ctx.ValidateStreamCompletion(testModelName, testContentType)

			require.Equal(t, tt.expectValid, valid, "valid mismatch")

			if tt.expectError {
				require.NotNil(t, err, "expected error, got nil")
			} else {
				require.Nil(t, err, "expected no error")
			}
		})
	}
}

// TestStreamingContext_Integration tests complete streaming workflow
func TestStreamingContext_Integration(t *testing.T) {
	t.Parallel()
	logger := createTestLogger()
	ctx := NewStreamingContext(logger, true) // Enable thinking

	// Simulate streaming workflow
	responses := []*ChatCompletionsStreamResponse{
		createTestStreamResponse("test-1", "Hello <think>", nil),
		createTestStreamResponse("test-2", "I need to think about this", nil),
		createTestStreamResponse("test-3", "</think> world!", nil),
		createTestStreamResponse("test-4", " How are you?", &model.Usage{
			PromptTokens:     testPromptTokens,
			CompletionTokens: 25,
			TotalTokens:      testPromptTokens + 25,
		}),
	}

	// Process all chunks
	for _, response := range responses {
		modified := ctx.ProcessStreamChunk(response)
		require.True(t, modified, "expected all chunks to be modified")
	}

	// Verify final state
	require.Equal(t, 4, ctx.chunksProcessed, "expected 4 chunks processed")

	// Calculate final usage
	finalUsage := ctx.CalculateUsage(testPromptTokens, testModelName)
	require.Equal(t, testPromptTokens, finalUsage.PromptTokens, "prompt tokens mismatch")

	// Validate completion
	err, valid := ctx.ValidateStreamCompletion(testModelName, testContentType)
	require.True(t, valid, "expected valid completion")
	require.Nil(t, err, "expected no error")

	// Verify content accumulation in responseTextBuilder
	// Note: responseTextBuilder contains original content before thinking processing
	finalContent := ctx.responseTextBuilder.String()
	require.Contains(t, finalContent, "Hello", "expected content to be preserved")
	require.Contains(t, finalContent, "world!", "expected content to be preserved")
	// The thinking tags remain in responseTextBuilder as it stores original content
	// The actual thinking processing modifies the response.Choices[].Delta.Content fields
}

// TestBufferCapacityConstants tests the capacity constants
func TestBufferCapacityConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, 4096, DefaultBuilderCapacity, "DefaultBuilderCapacity mismatch")
	require.Equal(t, 65536, LargeBuilderCapacity, "LargeBuilderCapacity mismatch")
	require.Equal(t, 1048576, MaxBuilderCapacity, "MaxBuilderCapacity mismatch")

	// Verify logical ordering
	require.Less(t, DefaultBuilderCapacity, LargeBuilderCapacity, "DefaultBuilderCapacity should be less than LargeBuilderCapacity")
	require.Less(t, LargeBuilderCapacity, MaxBuilderCapacity, "LargeBuilderCapacity should be less than MaxBuilderCapacity")
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// TestUnifiedStreamProcessing_ThinkingMapping verifies that when thinking is enabled and reasoning_format
// is set, the streamed chunks contain the reasoning in the requested field and avoid duplicates.
func TestUnifiedStreamProcessing_ThinkingMapping(t *testing.T) {
	t.Parallel()
	// Prepare a gin test context with query params
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/chat/completions?thinking=true&reasoning_format=thinking", nil)
	c.Request = req

	// Build a single SSE chunk where delta content includes a think block
	chunk := ChatCompletionsStreamResponse{
		Id:      "test-id",
		Object:  "chat.completion.chunk",
		Created: 123,
		Model:   testModelName,
		Choices: []ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: model.Message{Content: "hello <think>abc</think> world"},
			},
		},
	}
	b, _ := json.Marshal(chunk)
	sse := "data: " + string(b) + "\n\n" + "data: [DONE]\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	// Run unified processing with thinking enabled
	errResp, _ := UnifiedStreamProcessing(c, resp, 0, testModelName, true)
	require.Nil(t, errResp, "unexpected error")

	// Parse the first emitted chunk from the recorder
	body := w.Body.String()
	// Extract the JSON after "data: "
	lines := strings.Split(body, "\n")
	var jsonLine string
	for _, ln := range lines {
		if strings.HasPrefix(ln, "data: {") {
			jsonLine = strings.TrimPrefix(ln, "data: ")
			break
		}
	}
	require.NotEmpty(t, jsonLine, "no JSON chunk found in response body: %q", body)

	var out ChatCompletionsStreamResponse
	err := json.Unmarshal([]byte(jsonLine), &out)
	require.NoError(t, err, "failed to unmarshal emitted chunk")
	require.NotEmpty(t, out.Choices, "no choices in emitted chunk: %v", out)

	got := out.Choices[0].Delta
	// Expect content without think tags
	require.Equal(t, "hello  world", got.StringContent(), "unexpected content")
	// Expect thinking field to be set as per reasoning_format=thinking
	require.NotNil(t, got.Thinking, "expected thinking to be set")
	require.Equal(t, "abc", *got.Thinking, "thinking content mismatch")
	// And ReasoningContent should be cleared to avoid duplicates
	require.Nil(t, got.ReasoningContent, "expected ReasoningContent to be nil when reasoning_format=thinking")
}
