package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// responseAgentMessageReviewCanary is planted in agent_message content that
// must be rejected, so tests can prove errors never echo prompt content.
const responseAgentMessageReviewCanary = "agent-content-canary-7f3a"

// TestLowerResponseAgentMessageContentShapes proves every agent_message
// content shape is either lowered with its text intact or rejected with a
// wrapped ErrResponseFallbackNotRepresentable: a string is the message text,
// an array keeps the text-part join, and missing, null, number, bool, object
// or encrypted content is rejected without echoing the content.
func TestLowerResponseAgentMessageContentShapes(t *testing.T) {
	t.Parallel()
	const header = "[Agent message from /root to /root/worker]\n"
	cases := map[string]struct {
		content  string // raw JSON for the content member; "" omits it
		wantText string
		wantErr  bool
	}{
		"string":         {content: `"hello ` + responseAgentMessageReviewCanary + `"`, wantText: header + "hello " + responseAgentMessageReviewCanary},
		"empty_string":   {content: `""`, wantText: header},
		"multiline":      {content: `"line one\nline two"`, wantText: header + "line one\nline two"},
		"array":          {content: `[{"type":"input_text","text":"one"},{"type":"output_text","text":"two"}]`, wantText: header + "one\ntwo"},
		"empty_array":    {content: `[]`, wantText: header},
		"null":           {content: `null`, wantErr: true},
		"missing":        {wantErr: true},
		"object":         {content: `{"type":"input_text","text":"` + responseAgentMessageReviewCanary + `"}`, wantErr: true},
		"number":         {content: `7031`, wantErr: true},
		"bool":           {content: `true`, wantErr: true},
		"encrypted":      {content: `[{"type":"encrypted_content","encrypted_content":"` + responseAgentMessageReviewCanary + `"}]`, wantErr: true},
		"array_scalar":   {content: `["` + responseAgentMessageReviewCanary + `"]`, wantErr: true},
		"array_of_image": {content: `[{"type":"input_image","image_url":"https://example.invalid/` + responseAgentMessageReviewCanary + `.png"}]`, wantErr: true},
		// A text part whose text is not a string carries content the lowering
		// cannot represent; it must be rejected like a tool-output text part,
		// not silently lowered as an empty string.
		"part_object_text": {content: `[{"type":"input_text","text":{"value":"` + responseAgentMessageReviewCanary + `"}}]`, wantErr: true},
		"part_number_text": {content: `[{"type":"input_text","text":7031}]`, wantErr: true},
		"part_null_text":   {content: `[{"type":"output_text","text":null}]`, wantErr: true},
		"part_array_text":  {content: `[{"type":"text","text":["` + responseAgentMessageReviewCanary + `"]}]`, wantErr: true},
		"part_absent_text": {content: `[{"type":"input_text"},{"type":"input_text","text":"two"}]`, wantText: header + "\ntwo"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := `{"type":"agent_message","author":"/root","recipient":"/root/worker"`
			if tc.content != "" {
				item += `,"content":` + tc.content
			}
			item += `}`
			raw := `{"model":"fixture","input":[` + item + `]}`

			lowered, _, lowerErr := fallbackLower(t, raw)
			fidelityErr := ValidateResponseAPIFallbackFidelity(fallbackToolRequest(t, raw).Input, true)
			if tc.wantErr {
				for _, err := range []error{lowerErr, fidelityErr} {
					require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
					require.NotErrorIs(t, err, ErrResponseFallbackStateNotPortable, "content shape errors are 400s, not state portability conflicts")
					require.NotContains(t, err.Error(), responseAgentMessageReviewCanary, "errors must not echo agent message content")
					require.NotContains(t, err.Error(), "7031", "errors must not echo agent message content")
				}
				require.Nil(t, lowered)
				return
			}
			require.NoError(t, lowerErr)
			require.NoError(t, fidelityErr)
			require.Equal(t, ResponseAPIInput{map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": tc.wantText}},
			}}, lowered)
		})
	}
}

// TestConvertResponseAPIAgentMessageAdmission proves the fallback converter
// admits string agent_message content with its text delivered to the upstream
// Chat request, and that fidelity admission rejects unrepresentable content
// before any catalog building or lowering runs.
func TestConvertResponseAPIAgentMessageAdmission(t *testing.T) {
	t.Parallel()
	converted, err := ConvertResponseAPIToChatCompletionRequest(fallbackToolRequest(t,
		`{"model":"fixture","input":[{"type":"agent_message","author":"/root/worker","recipient":"/root","content":"task finished: 3 files"}]}`))
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "[Agent message from /root/worker to /root]\ntask finished: 3 files", converted.Messages[0].StringContent())

	for name, content := range map[string]string{
		"null":    `,"content":null`,
		"missing": ``,
		"object":  `,"content":{"text":"` + responseAgentMessageReviewCanary + `"}`,
		"number":  `,"content":42`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := fallbackToolRequest(t, `{"model":"fixture","input":[{"type":"agent_message","author":"/root"`+content+`},{"type":"message","role":"user","content":"next"}]}`)
			// Both admission passes the controller runs (before and after
			// hydration) reject the item, so no quota or upstream work starts.
			for _, allowReferences := range []bool{true, false} {
				err := ValidateResponseAPIFallbackFidelity(request.Input, allowReferences)
				require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
				require.NotErrorIs(t, err, ErrResponseFallbackStateNotPortable)
				require.NotContains(t, err.Error(), responseAgentMessageReviewCanary)
			}
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
			require.ErrorContains(t, err, "validate response fallback fidelity", "rejection happens at admission, before lowering")
			require.Nil(t, converted)
		})
	}
}
