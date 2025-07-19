package anthropic

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
)

func setupTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request and reasoning_format parameter
	req := httptest.NewRequest("POST", "/v1/chat/completions?reasoning_format=reasoning_content&thinking=true", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)
	return c
}

func TestStreamResponseClaude2OpenAI_ThinkingDelta(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Test thinking_delta event
	thinkingText := "Let me think about this..."
	claudeResponse := &StreamResponse{
		Type: "thinking_delta",
		Delta: &Delta{
			Thinking: &thinkingText,
		},
	}

	openaiResponse, response := StreamResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	if response != nil {
		t.Fatal("Expected nil response for thinking_delta")
	}

	// Check that reasoning content is set
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	if choice.Delta.ReasoningContent == nil {
		t.Fatal("Expected reasoning content to be set")
	}

	if *choice.Delta.ReasoningContent != thinkingText {
		t.Fatalf("Expected reasoning content %s, got %s", thinkingText, *choice.Delta.ReasoningContent)
	}
}

func TestStreamResponseClaude2OpenAI_SignatureDelta(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Test signature_delta event
	signatureText := "signature_abc123"
	claudeResponse := &StreamResponse{
		Type: "signature_delta",
		Delta: &Delta{
			Signature: &signatureText,
		},
	}

	openaiResponse, response := StreamResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	if response != nil {
		t.Fatal("Expected nil response for signature_delta")
	}

	// Signature should be cached but not exposed to client
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	// Signature should not be exposed in the OpenAI response
	if choice.Delta.Signature != nil {
		t.Fatal("Signature should not be exposed to OpenAI client")
	}
}

func TestStreamResponseClaude2OpenAI_ContentBlockWithSignature(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Test content_block_start with thinking and signature
	thinkingText := "Let me analyze this..."
	signatureText := "signature_def456"
	claudeResponse := &StreamResponse{
		Type: "content_block_start",
		ContentBlock: &Content{
			Type:      "thinking",
			Thinking:  &thinkingText,
			Signature: &signatureText,
		},
	}

	openaiResponse, response := StreamResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	if response != nil {
		t.Fatal("Expected nil response for content_block_start")
	}

	// Check reasoning content
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	if choice.Delta.ReasoningContent == nil || *choice.Delta.ReasoningContent != thinkingText {
		t.Fatal("Expected reasoning content to match thinking text")
	}

	// Signature should not be exposed
	if choice.Delta.Signature != nil {
		t.Fatal("Signature should not be exposed to OpenAI client")
	}
}

func TestResponseClaude2OpenAI_ThinkingWithSignature(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Test non-streaming response with thinking and signature
	thinkingText := "I need to consider several factors..."
	signatureText := "signature_ghi789"
	responseText := "Based on my analysis, the answer is..."

	claudeResponse := &Response{
		Content: []Content{
			{
				Type:      "thinking",
				Thinking:  &thinkingText,
				Signature: &signatureText,
			},
			{
				Type: "text",
				Text: responseText,
			},
		},
	}

	openaiResponse := ResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	// Check that response text is correct
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	if choice.Message.StringContent() != responseText {
		t.Fatalf("Expected response text %s, got %s", responseText, choice.Message.StringContent())
	}

	// Check that reasoning content is set
	if choice.Message.ReasoningContent == nil || *choice.Message.ReasoningContent != thinkingText {
		t.Fatal("Expected reasoning content to match thinking text")
	}

	// Signature should not be exposed
	if choice.Message.Signature != nil {
		t.Fatal("Signature should not be exposed to OpenAI client")
	}
}

func TestResponseClaude2OpenAI_RedactedThinking(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Test redacted_thinking support
	redactedThinkingText := "[REDACTED THINKING CONTENT]"
	signatureText := "signature_redacted_123"
	responseText := "Here's my response."

	claudeResponse := &Response{
		Content: []Content{
			{
				Type:      "redacted_thinking",
				Thinking:  &redactedThinkingText,
				Signature: &signatureText,
			},
			{
				Type: "text",
				Text: responseText,
			},
		},
	}

	openaiResponse := ResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	// Check that redacted thinking is handled like regular thinking
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	if choice.Message.ReasoningContent == nil || *choice.Message.ReasoningContent != redactedThinkingText {
		t.Fatal("Expected reasoning content to match redacted thinking text")
	}
}

func TestConvertRequest_ThinkingBlockOrdering(t *testing.T) {
	// Initialize signature cache
	InitSignatureCache(time.Hour)

	c := setupTestContext()
	tokenIDStr := getTokenIDFromRequest(12345)

	// Create a request with thinking content
	reasoningText := "Let me think step by step..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "What is 2+2?",
			},
			{
				Role:             "assistant",
				Content:          "The answer is 4.",
				ReasoningContent: &reasoningText,
			},
		},
	}

	// Pre-cache a signature to ensure thinking block is created (not fallback)
	conversationID := generateConversationID(textRequest.Messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 for assistant message
	testSignature := "test_signature_123"
	GetSignatureCache().Store(cacheKey, testSignature)

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that the assistant message has thinking block first
	if len(claudeRequest.Messages) < 2 {
		t.Fatal("Expected at least 2 messages")
	}

	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) < 2 {
		t.Fatal("Expected assistant message to have at least 2 content blocks")
	}

	// First content block should be thinking
	firstContent := assistantMessage.Content[0]
	if firstContent.Type != "thinking" {
		t.Fatalf("Expected first content block to be thinking, got %s", firstContent.Type)
	}

	if firstContent.Thinking == nil || *firstContent.Thinking != reasoningText {
		t.Fatal("Expected thinking content to match reasoning text")
	}

	// Second content block should be text
	secondContent := assistantMessage.Content[1]
	if secondContent.Type != "text" {
		t.Fatalf("Expected second content block to be text, got %s", secondContent.Type)
	}
}

func TestBackwardCompatibility_WithoutThinking(t *testing.T) {
	// Test that requests without thinking content work unchanged
	gin.SetMode(gin.TestMode)

	// Create a test context WITHOUT thinking parameter for backward compatibility test
	req := httptest.NewRequest("POST", "/v1/chat/completions?reasoning_format=reasoning_content", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)

	textRequest := model.GeneralOpenAIRequest{
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:    "assistant",
				Content: "Hi there!",
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Should work normally without thinking blocks
	if len(claudeRequest.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(claudeRequest.Messages))
	}

	// Assistant message should have only text content
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(assistantMessage.Content))
	}

	if assistantMessage.Content[0].Type != "text" {
		t.Fatalf("Expected text content, got %s", assistantMessage.Content[0].Type)
	}
}

func TestBackwardCompatibility_StreamingWithoutThinking(t *testing.T) {
	// Test that streaming responses without thinking work unchanged
	c := setupTestContext()

	claudeResponse := &StreamResponse{
		Type: "content_block_delta",
		Delta: &Delta{
			Text: "Hello world",
		},
	}

	openaiResponse, response := StreamResponseClaude2OpenAI(c, claudeResponse)

	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response, got nil")
	}

	if response != nil {
		t.Fatal("Expected nil response for content_block_delta")
	}

	// Should work normally
	if len(openaiResponse.Choices) == 0 {
		t.Fatal("Expected at least one choice")
	}

	choice := openaiResponse.Choices[0]
	if choice.Delta.Content != "Hello world" {
		t.Fatalf("Expected content 'Hello world', got %s", choice.Delta.Content)
	}

	// Should not have reasoning content
	if choice.Delta.ReasoningContent != nil {
		t.Fatalf("Expected no reasoning content for non-thinking response, got: %q", *choice.Delta.ReasoningContent)
	}
}

func TestSignatureCachingIntegration(t *testing.T) {
	// Test full signature caching flow: response -> cache -> request
	InitSignatureCache(time.Hour)

	c := setupTestContext()
	tokenIDStr := getTokenIDFromRequest(12345)

	// Step 1: Process a Claude response with thinking and signature
	thinkingText := "I need to analyze this carefully..."
	signatureText := "signature_integration_test"
	responseText := "Based on my analysis, here's the answer."

	claudeResponse := &Response{
		Content: []Content{
			{
				Type:      "thinking",
				Thinking:  &thinkingText,
				Signature: &signatureText,
			},
			{
				Type: "text",
				Text: responseText,
			},
		},
	}

	// Set up conversation context
	messages := []model.Message{
		{Role: "user", Content: "Test question"},
	}
	conversationID := generateConversationID(messages)
	c.Set(ctxkey.ConversationId, conversationID)

	// Process the response (this should cache the signature)
	openaiResponse := ResponseClaude2OpenAI(c, claudeResponse)
	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response")
	}

	// Step 2: Manually cache the signature with the correct message index for the follow-up request
	// In the follow-up request, the assistant message will be at index 1
	followUpConversationID := generateConversationID([]model.Message{
		{Role: "user", Content: "Test question"},
		{Role: "assistant", Content: responseText, ReasoningContent: &thinkingText},
		{Role: "user", Content: "Follow-up question"},
	})
	cacheKey := generateSignatureKey(tokenIDStr, followUpConversationID, 1, 0) // messageIndex=1 for assistant message
	GetSignatureCache().Store(cacheKey, signatureText)

	// Step 3: Create a follow-up request that includes the thinking content
	followUpRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "Test question"},
			{
				Role:             "assistant",
				Content:          responseText,
				ReasoningContent: &thinkingText,
			},
			{Role: "user", Content: "Follow-up question"},
		},
	}

	// Step 4: Convert the request (this should restore the signature)
	claudeRequest, err := ConvertRequest(c, followUpRequest)
	if err != nil {
		t.Fatalf("Failed to convert follow-up request: %v", err)
	}

	// Step 5: Verify signature is restored in the thinking block
	if len(claudeRequest.Messages) < 2 {
		t.Fatal("Expected at least 2 messages in Claude request")
	}

	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) == 0 {
		t.Fatal("Expected assistant message to have content")
	}

	// First content should be thinking block with restored signature
	thinkingBlock := assistantMessage.Content[0]
	if thinkingBlock.Type != "thinking" {
		t.Fatalf("Expected thinking block, got %s", thinkingBlock.Type)
	}

	if thinkingBlock.Signature == nil {
		t.Fatal("Expected signature to be restored in thinking block")
	}

	if *thinkingBlock.Signature != signatureText {
		t.Fatalf("Expected restored signature %s, got %s", signatureText, *thinkingBlock.Signature)
	}
}

func TestMultipleThinkingBlocks(t *testing.T) {
	// Test handling of multiple thinking blocks in a single response
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	thinking1 := "First thought..."
	signature1 := "sig1"
	thinking2 := "Second thought..."
	signature2 := "sig2"
	responseText := "Final answer."

	claudeResponse := &Response{
		Content: []Content{
			{
				Type:      "thinking",
				Thinking:  &thinking1,
				Signature: &signature1,
			},
			{
				Type:      "thinking",
				Thinking:  &thinking2,
				Signature: &signature2,
			},
			{
				Type: "text",
				Text: responseText,
			},
		},
	}

	openaiResponse := ResponseClaude2OpenAI(c, claudeResponse)
	if openaiResponse == nil {
		t.Fatal("Expected OpenAI response")
	}

	// Should combine thinking content
	choice := openaiResponse.Choices[0]
	if choice.Message.ReasoningContent == nil {
		t.Fatal("Expected reasoning content")
	}

	expectedReasoning := thinking1 + thinking2
	if *choice.Message.ReasoningContent != expectedReasoning {
		t.Fatalf("Expected combined reasoning %s, got %s", expectedReasoning, *choice.Message.ReasoningContent)
	}
}

func TestCacheExpiration(t *testing.T) {
	// Test that expired signatures are not restored
	InitSignatureCache(50 * time.Millisecond) // Very short TTL

	c := setupTestContext()
	tokenIDStr := getTokenIDFromRequest(12345)

	// Cache a signature
	conversationID := "test_conv"
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 0, 0)
	GetSignatureCache().Store(cacheKey, "expired_signature")

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Try to restore signature
	reasoningText := "Some thinking..."
	followUpRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "Test"},
			{
				Role:             "assistant",
				Content:          "Response",
				ReasoningContent: &reasoningText,
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, followUpRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Signature should not be restored due to expiration
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) > 0 {
		thinkingBlock := assistantMessage.Content[0]
		if thinkingBlock.Type == "thinking" && thinkingBlock.Signature != nil {
			t.Fatal("Expired signature should not be restored")
		}
	}
}

func TestSignatureFallbackMechanism(t *testing.T) {
	// Test that thinking content is converted to <think> format when signature is not cached
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Create a request with thinking content but no cached signature
	reasoningText := "Let me analyze this step by step..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "What is the solution?"},
			{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: &reasoningText,
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that the assistant message uses fallback format
	if len(claudeRequest.Messages) < 2 {
		t.Fatal("Expected at least 2 messages")
	}

	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) == 0 {
		t.Fatal("Expected assistant message to have content")
	}

	// Should have text content with <think> prefix, not a thinking block
	textContent := assistantMessage.Content[0]
	if textContent.Type != "text" {
		t.Fatalf("Expected text content, got %s", textContent.Type)
	}

	expectedPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningText)
	if !strings.HasPrefix(textContent.Text, expectedPrefix) {
		t.Fatalf("Expected text to start with %s, got %s", expectedPrefix, textContent.Text)
	}

	// Should contain the original response text after the thinking
	if !strings.Contains(textContent.Text, "The answer is 42.") {
		t.Fatal("Expected original response text to be preserved")
	}
}

func TestSignatureFallbackWithCachedSignature(t *testing.T) {
	// Test that proper thinking blocks are used when signature is cached
	InitSignatureCache(time.Hour)

	c := setupTestContext()
	tokenIDStr := getTokenIDFromRequest(12345)

	// Pre-cache a signature
	messages := []model.Message{
		{Role: "user", Content: "What is the solution?"},
	}
	conversationID := generateConversationID(messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 for assistant message
	testSignature := "cached_signature_123"
	GetSignatureCache().Store(cacheKey, testSignature)

	// Create a request with thinking content
	reasoningText := "Let me analyze this step by step..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "What is the solution?"},
			{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: &reasoningText,
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that the assistant message uses proper thinking block
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) < 2 {
		t.Fatal("Expected at least 2 content blocks")
	}

	// First content should be thinking block with signature
	thinkingBlock := assistantMessage.Content[0]
	if thinkingBlock.Type != "thinking" {
		t.Fatalf("Expected thinking block, got %s", thinkingBlock.Type)
	}

	if thinkingBlock.Signature == nil {
		t.Fatal("Expected signature to be restored")
	}

	if *thinkingBlock.Signature != testSignature {
		t.Fatalf("Expected signature %s, got %s", testSignature, *thinkingBlock.Signature)
	}

	// Second content should be text without <think> prefix
	textContent := assistantMessage.Content[1]
	if textContent.Type != "text" {
		t.Fatalf("Expected text content, got %s", textContent.Type)
	}

	if strings.Contains(textContent.Text, "<think>") {
		t.Fatal("Text content should not contain <think> tags when signature is cached")
	}
}

func TestSignatureFallbackEmptyTextContent(t *testing.T) {
	// Test fallback when there's no existing text content
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Create a request with only thinking content, no regular text
	reasoningText := "Pure thinking without response text..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "Think about this"},
			{
				Role:             "assistant",
				ReasoningContent: &reasoningText,
				// No Content field - only thinking
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Should create text content with thinking prefix
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) == 0 {
		t.Fatal("Expected assistant message to have content")
	}

	textContent := assistantMessage.Content[0]
	if textContent.Type != "text" {
		t.Fatalf("Expected text content, got %s", textContent.Type)
	}

	expectedText := fmt.Sprintf("<think>%s</think>\n\n", reasoningText)
	if textContent.Text != expectedText {
		t.Fatalf("Expected text %s, got %s", expectedText, textContent.Text)
	}
}

func TestSignatureFallbackMultipleTextBlocks(t *testing.T) {
	// Test that fallback prepends to the first text block when multiple exist
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	reasoningText := "My reasoning process..."

	// Create assistant message with multiple content blocks
	firstText := "First text block"
	secondText := "Second text block"
	assistantMessage := model.Message{
		Role: "assistant",
		Content: []model.MessageContent{
			{Type: "text", Text: &firstText},
			{Type: "text", Text: &secondText},
		},
		ReasoningContent: &reasoningText,
	}

	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet", // Model that supports thinking
		MaxTokens: 2048,              // Must be > 1024 for thinking
		Messages: []model.Message{
			{Role: "user", Content: "Test question"},
			assistantMessage,
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that thinking is prepended to first text block
	assistantMsg := claudeRequest.Messages[1]
	if len(assistantMsg.Content) < 2 {
		t.Fatal("Expected at least 2 content blocks")
	}

	firstTextBlock := assistantMsg.Content[0]
	if firstTextBlock.Type != "text" {
		t.Fatalf("Expected first block to be text, got %s", firstTextBlock.Type)
	}

	expectedPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningText)
	if !strings.HasPrefix(firstTextBlock.Text, expectedPrefix) {
		t.Fatalf("Expected first text block to start with thinking prefix")
	}

	if !strings.Contains(firstTextBlock.Text, "First text block") {
		t.Fatal("Expected original first text block content to be preserved")
	}

	// Second text block should be unchanged
	secondTextBlock := assistantMsg.Content[1]
	if secondTextBlock.Text != "Second text block" {
		t.Fatalf("Expected second text block to be unchanged, got %s", secondTextBlock.Text)
	}
}

func TestBackwardCompatibilityWithFallback(t *testing.T) {
	// Test that the fallback mechanism doesn't break existing functionality
	InitSignatureCache(time.Hour)

	gin.SetMode(gin.TestMode)

	// Create a test context WITHOUT thinking parameter for backward compatibility test
	req := httptest.NewRequest("POST", "/v1/chat/completions?reasoning_format=reasoning_content", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)

	// Test normal request without thinking content
	normalRequest := model.GeneralOpenAIRequest{
		Messages: []model.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
		},
	}

	claudeRequest, err := ConvertRequest(c, normalRequest)
	if err != nil {
		t.Fatalf("Failed to convert normal request: %v", err)
	}

	// Should work exactly as before
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(assistantMessage.Content))
	}

	if assistantMessage.Content[0].Type != "text" {
		t.Fatalf("Expected text content, got %s", assistantMessage.Content[0].Type)
	}

	if assistantMessage.Content[0].Text != "Hi there!" {
		t.Fatalf("Expected 'Hi there!', got %s", assistantMessage.Content[0].Text)
	}

	// Should not contain any <think> tags
	if strings.Contains(assistantMessage.Content[0].Text, "<think>") {
		t.Fatal("Normal messages should not contain <think> tags")
	}
}

func TestSignatureFallbackDisablesThinking(t *testing.T) {
	// Test that thinking parameter is disabled when fallback mode is used
	InitSignatureCache(time.Hour)

	c := setupTestContext()

	// Create a request with thinking enabled and reasoning content but no cached signature
	reasoningText := "Let me analyze this step by step..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet",
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 512,
		},
		Messages: []model.Message{
			{Role: "user", Content: "What is the solution?"},
			{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: &reasoningText,
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that thinking parameter is disabled due to fallback mode
	if claudeRequest.Thinking != nil {
		t.Fatal("Expected thinking parameter to be disabled when using fallback mode")
	}

	// Check that the assistant message uses fallback format
	if len(claudeRequest.Messages) < 2 {
		t.Fatal("Expected at least 2 messages")
	}

	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) == 0 {
		t.Fatal("Expected assistant message to have content")
	}

	// Should have text content with <think> prefix, not a thinking block
	textContent := assistantMessage.Content[0]
	if textContent.Type != "text" {
		t.Fatalf("Expected text content, got %s", textContent.Type)
	}

	expectedPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningText)
	if !strings.HasPrefix(textContent.Text, expectedPrefix) {
		t.Fatalf("Expected text to start with %s, got %s", expectedPrefix, textContent.Text)
	}
}

func TestSignatureFallbackKeepsThinkingWhenSignatureRestored(t *testing.T) {
	// Test that thinking parameter is kept when signature is successfully restored
	InitSignatureCache(time.Hour)

	c := setupTestContext()
	tokenIDStr := getTokenIDFromRequest(12345)

	// Pre-cache a signature
	messages := []model.Message{
		{Role: "user", Content: "What is the solution?"},
	}
	conversationID := generateConversationID(messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 for assistant message
	testSignature := "cached_signature_123"
	GetSignatureCache().Store(cacheKey, testSignature)

	// Create a request with thinking enabled and reasoning content
	reasoningText := "Let me analyze this step by step..."
	textRequest := model.GeneralOpenAIRequest{
		Model:     "claude-4-sonnet",
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 512,
		},
		Messages: []model.Message{
			{Role: "user", Content: "What is the solution?"},
			{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: &reasoningText,
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, textRequest)
	if err != nil {
		t.Fatalf("Failed to convert request: %v", err)
	}

	// Check that thinking parameter is preserved when signature is restored
	if claudeRequest.Thinking == nil {
		t.Fatal("Expected thinking parameter to be preserved when signature is restored")
	}

	if claudeRequest.Thinking.Type != "enabled" {
		t.Fatalf("Expected thinking type 'enabled', got %s", claudeRequest.Thinking.Type)
	}

	if claudeRequest.Thinking.BudgetTokens != 512 {
		t.Fatalf("Expected thinking budget tokens 512, got %d", claudeRequest.Thinking.BudgetTokens)
	}

	// Check that the assistant message uses proper thinking block
	assistantMessage := claudeRequest.Messages[1]
	if len(assistantMessage.Content) < 2 {
		t.Fatal("Expected at least 2 content blocks")
	}

	// First content should be thinking block with signature
	thinkingBlock := assistantMessage.Content[0]
	if thinkingBlock.Type != "thinking" {
		t.Fatalf("Expected thinking block, got %s", thinkingBlock.Type)
	}

	if thinkingBlock.Signature == nil {
		t.Fatal("Expected signature to be restored")
	}

	if *thinkingBlock.Signature != testSignature {
		t.Fatalf("Expected signature %s, got %s", testSignature, *thinkingBlock.Signature)
	}
}
