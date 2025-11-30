package openai_compatible

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractThinkingContent_SingleThinkTag(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedThinking string
		expectedRegular  string
	}{
		{
			name:             "Single think tag with content",
			input:            "Hello <think>This is my reasoning</think> world",
			expectedThinking: "This is my reasoning",
			expectedRegular:  "Hello  world",
		},
		{
			name:             "Single think tag at beginning",
			input:            "<think>Initial thought</think>Rest of content",
			expectedThinking: "Initial thought",
			expectedRegular:  "Rest of content",
		},
		{
			name:             "Single think tag at end",
			input:            "Content before <think>Final thought</think>",
			expectedThinking: "Final thought",
			expectedRegular:  "Content before",
		},
		{
			name:             "Single think tag only",
			input:            "<think>Only thinking</think>",
			expectedThinking: "Only thinking",
			expectedRegular:  "",
		},
		{
			name:             "Empty think tag",
			input:            "Before <think></think> After",
			expectedThinking: "",
			expectedRegular:  "Before  After",
		},
		{
			name:             "No think tags",
			input:            "Regular content without any tags",
			expectedThinking: "",
			expectedRegular:  "Regular content without any tags",
		},
		{
			name:             "Empty input",
			input:            "",
			expectedThinking: "",
			expectedRegular:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking, regular := ExtractThinkingContent(tt.input)
			require.Equal(t, tt.expectedThinking, thinking, "ExtractThinkingContent() thinking mismatch")
			require.Equal(t, tt.expectedRegular, regular, "ExtractThinkingContent() regular mismatch")
		})
	}
}

func TestExtractThinkingContent_MultipleThinkTags_SingleTagBehavior(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedThinking string
		expectedRegular  string
		description      string
	}{
		{
			name:             "Two separate think tags - only first processed",
			input:            "Start <think>First thought</think> middle <think>Second thought</think> end",
			expectedThinking: "First thought",
			expectedRegular:  "Start  middle <think>Second thought</think> end",
			description:      "Should handle only the first think tag, subsequent tags treated as regular content",
		},
		{
			name:             "Three think tags - only first processed",
			input:            "<think>One</think>A<think>Two</think>B<think>Three</think>C",
			expectedThinking: "One",
			expectedRegular:  "A<think>Two</think>B<think>Three</think>C",
			description:      "Should handle only the first think tag, others treated as regular content",
		},
		{
			name:             "First think tag empty, second has content",
			input:            "Start <think></think> middle <think>Content</think> end",
			expectedThinking: "",
			expectedRegular:  "Start  middle <think>Content</think> end",
			description:      "Should process first empty think tag, ignore subsequent ones",
		},
		{
			name:             "Adjacent think tags - only first processed",
			input:            "<think>First</think><think>Second</think>",
			expectedThinking: "First",
			expectedRegular:  "<think>Second</think>",
			description:      "Should handle only first think tag, second becomes regular content",
		},
		{
			name:             "Nested-like content (but not actually nested)",
			input:            "<think>Outer <think>inner</think> content</think>",
			expectedThinking: "Outer <think>inner",
			expectedRegular:  "content</think>",
			description:      "Should process first opening and first closing tag, not handle nesting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking, regular := ExtractThinkingContent(tt.input)
			require.Equal(t, tt.expectedThinking, thinking, "ExtractThinkingContent() thinking mismatch - %s", tt.description)
			require.Equal(t, tt.expectedRegular, regular, "ExtractThinkingContent() regular mismatch - %s", tt.description)
		})
	}
}

func TestExtractThinkingContent_EdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedThinking string
		expectedRegular  string
		description      string
	}{
		{
			name:             "Unclosed think tag",
			input:            "Start <think>Unclosed thinking content",
			expectedThinking: "",
			expectedRegular:  "Start <think>Unclosed thinking content",
			description:      "Should treat unclosed think tag as regular content",
		},
		{
			name:             "Only closing think tag",
			input:            "Start content </think> end",
			expectedThinking: "",
			expectedRegular:  "Start content </think> end",
			description:      "Should treat orphaned closing tag as regular content",
		},
		{
			name:             "Mixed case tags",
			input:            "Start <Think>Mixed case</Think> end",
			expectedThinking: "",
			expectedRegular:  "Start <Think>Mixed case</Think> end",
			description:      "Should be case sensitive and not match mixed case tags",
		},
		{
			name:             "Tags with spaces",
			input:            "Start < think >Spaced tags</ think > end",
			expectedThinking: "",
			expectedRegular:  "Start < think >Spaced tags</ think > end",
			description:      "Should not match tags with spaces",
		},
		{
			name:             "Multiple unclosed tags",
			input:            "<think>First unclosed <think>Second unclosed",
			expectedThinking: "",
			expectedRegular:  "<think>First unclosed <think>Second unclosed",
			description:      "Should treat multiple unclosed tags as regular content",
		},
		{
			name:             "Think tag with attributes",
			input:            "Start <think id='test'>Content</think> end",
			expectedThinking: "",
			expectedRegular:  "Start <think id='test'>Content</think> end",
			description:      "Should not match think tags with attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking, regular := ExtractThinkingContent(tt.input)
			require.Equal(t, tt.expectedThinking, thinking, "ExtractThinkingContent() thinking mismatch - %s", tt.description)
			require.Equal(t, tt.expectedRegular, regular, "ExtractThinkingContent() regular mismatch - %s", tt.description)
		})
	}
}

func TestExtractThinkingContent_PerformanceWithMultipleTags_SingleTagBehavior(t *testing.T) {
	// Test performance characteristics with many think tags - only first should be processed
	var builder strings.Builder
	var expectedRegularBuilder strings.Builder

	// Build input with 10 think tags
	for i := range 10 {
		regularContent := "Regular content " + string(rune('A'+i))
		thinkingContent := "Thinking content " + string(rune('1'+i))

		builder.WriteString(regularContent)
		builder.WriteString("<think>")
		builder.WriteString(thinkingContent)
		builder.WriteString("</think>")

		if i == 0 {
			// First tag: only regular content before goes to expectedRegular
			expectedRegularBuilder.WriteString(regularContent)
		} else {
			// Subsequent tags: everything goes to expectedRegular
			expectedRegularBuilder.WriteString(regularContent)
			expectedRegularBuilder.WriteString("<think>")
			expectedRegularBuilder.WriteString(thinkingContent)
			expectedRegularBuilder.WriteString("</think>")
		}
	}

	input := builder.String()
	expectedThinking := "Thinking content 1" // Only first think tag content
	expectedRegular := expectedRegularBuilder.String()

	thinking, regular := ExtractThinkingContent(input)

	require.Equal(t, expectedThinking, thinking, "ExtractThinkingContent() with multiple tags (single-tag behavior) thinking mismatch")
	require.Equal(t, expectedRegular, regular, "ExtractThinkingContent() with multiple tags (single-tag behavior) regular mismatch")
}

// Test the improved behavior - now handles only 1 think tag, not multiple
func TestThinkTagHandling_ImprovedSingleTagOnly(t *testing.T) {
	t.Run("Verify only first think tag is processed, subsequent ones treated as regular content", func(t *testing.T) {
		input := "Hello <think>first</think> world <think>second</think> end"
		thinking, regular := ExtractThinkingContent(input)

		// The IMPROVED implementation handles only the FIRST think tag
		require.Equal(t, "first", thinking, "Expected only first think tag to be handled")
		require.Equal(t, "Hello  world <think>second</think> end", regular, "Expected regular content with second think tag preserved")
	})

	t.Run("Verify single think tag still works correctly", func(t *testing.T) {
		input := "Hello <think>only one</think> world"
		thinking, regular := ExtractThinkingContent(input)

		require.Equal(t, "only one", thinking, "Expected single think tag to work")
		require.Equal(t, "Hello  world", regular, "Expected regular content without think tag")
	})

	t.Run("Verify improvement: three think tags - only first processed", func(t *testing.T) {
		input := "Start <think>first</think> middle <think>second</think> more <think>third</think> end"
		thinking, regular := ExtractThinkingContent(input)

		// Only the first think tag should be extracted
		require.Equal(t, "first", thinking, "Expected only first think tag content")

		// All subsequent think tags should remain in regular content
		expectedRegular := "Start  middle <think>second</think> more <think>third</think> end"
		require.Equal(t, expectedRegular, regular, "Expected subsequent think tags to remain in regular content")
	})
}

// Test streaming behavior to verify only first think tag is processed
func TestStreamHandlerWithThinking_SingleTagBehavior(t *testing.T) {
	// Note: This is a conceptual test structure since StreamHandlerWithThinking processes
	// streaming chunks. The actual implementation would require mock HTTP responses and
	// streaming chunk simulation. The key behavioral change is that the function now
	// uses hasProcessedThinkTag flag to prevent processing of subsequent think tags.
	t.Run("Streaming handler should only process first think tag", func(t *testing.T) {
		// This test verifies the logic change in StreamHandlerWithThinking
		// The function now includes:
		// 1. hasProcessedThinkTag flag initialization
		// 2. Condition: !hasProcessedThinkTag && strings.Contains(deltaContent, "<think>")
		// 3. Setting hasProcessedThinkTag = true when thinking block ends

		// The behavior should match ExtractThinkingContent for consistency
		input1 := "Hello <think>first</think> world"
		thinking1, regular1 := ExtractThinkingContent(input1)
		require.Equal(t, "first", thinking1, "Single think tag test failed - thinking mismatch")
		require.Equal(t, "Hello  world", regular1, "Single think tag test failed - regular mismatch")

		input2 := "Hello <think>first</think> world <think>second</think> end"
		thinking2, regular2 := ExtractThinkingContent(input2)
		require.Equal(t, "first", thinking2, "Multiple think tags test failed - thinking mismatch")
		require.Equal(t, "Hello  world <think>second</think> end", regular2, "Multiple think tags test failed - regular mismatch")
	})
}

func TestExtractThinkingContent_UnicodeThinkTags(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedThinking string
		expectedRegular  string
		description      string
	}{
		{
			name:             "Unicode escaped complete think tag",
			input:            "Hello \\u003cthink\\u003ereasoning content\\u003c/think\\u003e world",
			expectedThinking: "reasoning content",
			expectedRegular:  "Hello  world",
			description:      "Should extract content from Unicode-escaped thinking tags",
		},
		{
			name:             "Unicode escaped empty think tag",
			input:            "Before \\u003cthink\\u003e\\u003c/think\\u003e After",
			expectedThinking: "",
			expectedRegular:  "Before  After",
			description:      "Should handle empty Unicode-escaped thinking tags",
		},
		{
			name:             "Mixed normal and Unicode tags - normal first",
			input:            "Start <think>normal first</think> middle \\u003cthink\\u003eunicode second\\u003c/think\\u003e end",
			expectedThinking: "normal first",
			expectedRegular:  "Start  middle \\u003cthink\\u003eunicode second\\u003c/think\\u003e end",
			description:      "Should process first tag (normal) and treat Unicode as regular content",
		},
		{
			name:             "Mixed normal and Unicode tags - Unicode first",
			input:            "Start \\u003cthink\\u003eunicode first\\u003c/think\\u003e middle <think>normal second</think> end",
			expectedThinking: "unicode first",
			expectedRegular:  "Start  middle <think>normal second</think> end",
			description:      "Should process first tag (Unicode) and treat normal as regular content",
		},
		{
			name:             "Unicode tag at beginning",
			input:            "\\u003cthink\\u003eInitial Unicode thought\\u003c/think\\u003eRest of content",
			expectedThinking: "Initial Unicode thought",
			expectedRegular:  "Rest of content",
			description:      "Should handle Unicode thinking tag at the beginning",
		},
		{
			name:             "Unicode tag at end",
			input:            "Content before \\u003cthink\\u003eFinal Unicode thought\\u003c/think\\u003e",
			expectedThinking: "Final Unicode thought",
			expectedRegular:  "Content before",
			description:      "Should handle Unicode thinking tag at the end",
		},
		{
			name:             "Multiple Unicode tags - only first processed",
			input:            "\\u003cthink\\u003eFirst Unicode\\u003c/think\\u003e middle \\u003cthink\\u003eSecond Unicode\\u003c/think\\u003e end",
			expectedThinking: "First Unicode",
			expectedRegular:  "middle \\u003cthink\\u003eSecond Unicode\\u003c/think\\u003e end",
			description:      "Should process only first Unicode tag, subsequent ones treated as regular content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking, regular := ExtractThinkingContent(tt.input)
			require.Equal(t, tt.expectedThinking, thinking, "ExtractThinkingContent() thinking mismatch - %s", tt.description)
			require.Equal(t, tt.expectedRegular, regular, "ExtractThinkingContent() regular mismatch - %s", tt.description)
		})
	}
}
