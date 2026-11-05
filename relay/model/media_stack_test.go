package model

import (
	stderrors "errors"
	"fmt"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestMediaValidationErrorsCarryStacks verifies the message, cause and return-site stack for media validation failures.
func TestMediaValidationErrorsCarryStacks(t *testing.T) {
	type stackTracer interface{ StackTrace() stackerrors.StackTrace }
	tests := []struct {
		name   string
		cause  error
		frame  string
		invoke func() error
	}{
		{"claude source", errClaudeMediaSourceNotObject, "DecodeClaudeMediaPart", func() error { _, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document"}); return err }},
		{"claude base64", errClaudeMediaBase64Shape, "DecodeClaudeMediaPart", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document", "source": map[string]any{"type": "base64"}})
			return err
		}},
		{"claude url", errClaudeMediaURLShape, "DecodeClaudeMediaPart", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document", "source": map[string]any{"type": "url"}})
			return err
		}},
		{"claude file", errClaudeMediaFileShape, "DecodeClaudeMediaPart", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document", "source": map[string]any{"type": "file"}})
			return err
		}},
		{"claude type", errClaudeMediaSourceType, "DecodeClaudeMediaPart", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document", "source": map[string]any{"type": "other"}})
			return err
		}},
		{"claude mime", errClaudeMediaTypeMismatch, "DecodeClaudeMediaPart", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "audio", "source": map[string]any{"type": "url", "url": "x"}})
			return err
		}},
		{"claude field", errMediaFieldType, "readMediaStrings", func() error {
			_, _, err := DecodeClaudeMediaPart(map[string]any{"type": "document", "source": map[string]any{"type": 42}})
			return err
		}},
		{"video object", errVideoURLNotObject, "decodeMediaPart", func() error { _, _, err := decodeMediaPart(map[string]any{"type": "video_url"}); return err }},
		{"video fields", errVideoURLFields, "decodeMediaPart", func() error {
			_, _, err := decodeMediaPart(map[string]any{"type": "video_url", "video_url": map[string]any{}})
			return err
		}},
		{"file object", errFileNotObject, "decodeMediaPart", func() error { _, _, err := decodeMediaPart(map[string]any{"type": "file", "file": 42}); return err }},
		{"file mixed", errFileMixedShape, "decodeMediaPart", func() error {
			_, _, err := decodeMediaPart(map[string]any{"type": "file", "file": map[string]any{}, "file_id": "x"})
			return err
		}},
		{"file fields", errFileFieldsNotStrings, "decodeMediaPart", func() error { _, _, err := decodeMediaPart(map[string]any{"type": "file", "file_id": 42}); return err }},
		{"file choice", errFileSourceChoice, "decodeMediaPart", func() error { _, _, err := decodeMediaPart(map[string]any{"type": "file"}); return err }},
		{"content type", errContentNotTextOrParts, "ParseContentChecked", func() error { _, err := (Message{Content: 42}).ParseContentChecked(); return err }},
		{"text", errTextNotString, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: ContentTypeText}) }},
		{"image", errImageURLEmpty, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: ContentTypeImageURL}) }},
		{"video", errVideoURLEmpty, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: ContentTypeVideoURL}) }},
		{"audio", errInputAudioIncomplete, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: ContentTypeInputAudio}) }},
		{"file source", errFileSourceCount, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: ContentTypeFile}) }},
		{"unsupported", errUnsupportedContentPartType, "validateMessagePart", func() error { return validateMessagePart(MessageContent{Type: "unknown"}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.invoke()
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, tc.cause.Error(), err.Error())
			var trace stackTracer
			require.True(t, stderrors.As(err, &trace))
			require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), tc.frame)
		})
	}
}

// TestParseContentCheckedRetainsNestedErrorStack verifies indexed errors retain their original validation cause and stack.
func TestParseContentCheckedRetainsNestedErrorStack(t *testing.T) {
	type stackTracer interface{ StackTrace() stackerrors.StackTrace }
	tests := []struct {
		content any
		cause   error
		frame   string
	}{
		{[]MessageContent{{Type: ContentTypeText}}, errTextNotString, "validateMessagePart"},
		{[]any{map[string]any{"type": "file"}}, errFileSourceChoice, "decodeMediaPart"},
	}
	for _, tc := range tests {
		t.Run(tc.frame, func(t *testing.T) {
			_, err := (Message{Content: tc.content}).ParseContentChecked()
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, "content part 0: "+tc.cause.Error(), err.Error())
			var trace stackTracer
			require.True(t, stderrors.As(err, &trace))
			require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), tc.frame)
			require.Contains(t, fmt.Sprintf("%+v", err), tc.frame)
		})
	}
}

// TestParseContentCheckedIndexedErrorsCarryStacks verifies malformed part errors preserve their indexed messages and originating frames.
func TestParseContentCheckedIndexedErrorsCarryStacks(t *testing.T) {
	type stackTracer interface{ StackTrace() stackerrors.StackTrace }
	tests := []struct {
		content any
		message string
	}{
		{[]any{42}, "content part 0 must be an object"},
		{[]any{map[string]any{"type": "unknown"}}, "content part 0 has an unsupported type or malformed payload"},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			_, err := (Message{Content: tc.content}).ParseContentChecked()
			require.EqualError(t, err, tc.message)
			var trace stackTracer
			require.True(t, stderrors.As(err, &trace))
			require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), "ParseContentChecked")
		})
	}
}
