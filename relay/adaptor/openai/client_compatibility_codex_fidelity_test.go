package openai

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// clientCompatibilityCodexRequest decodes client wire data without losing absent fields.
func clientCompatibilityCodexRequest(t *testing.T, input string) *ResponseAPIRequest {
	t.Helper()
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(input), &request))
	return &request
}

// TestClientCompatibilityCodexJSONSchemaStrict preserves explicit strict semantics and legacy absent-field sanitization.
func TestClientCompatibilityCodexJSONSchemaStrict(t *testing.T) {
	t.Parallel()
	for _, strict := range []string{"true", "false", "absent"} {
		t.Run(strict, func(t *testing.T) {
			field := ""
			if strict != "absent" {
				field = `,"strict":` + strict
			}
			request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":"answer","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`+field+`}}}`)
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.NoError(t, err)
			require.NotNil(t, converted.ResponseFormat)
			require.Equal(t, "json_schema", converted.ResponseFormat.Type)
			require.NotNil(t, converted.ResponseFormat.JsonSchema)
			require.Equal(t, "answer", converted.ResponseFormat.JsonSchema.Name)
			encoded, err := json.Marshal(converted.ResponseFormat)
			require.NoError(t, err)
			var wire struct {
				JSONSchema map[string]json.RawMessage `json:"json_schema"`
			}
			require.NoError(t, json.Unmarshal(encoded, &wire))
			if strict == "absent" {
				require.NotContains(t, wire.JSONSchema, "strict")
				require.JSONEq(t, `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`, string(wire.JSONSchema["schema"]), "absent strict retains the established legacy schema sanitizer")
			} else {
				require.Equal(t, request.Text.Format.Schema, converted.ResponseFormat.JsonSchema.Schema, "explicit strict preserves the complete requested schema")
				require.NotNil(t, converted.ResponseFormat.JsonSchema.Strict)
				require.Equal(t, strict == "true", *converted.ResponseFormat.JsonSchema.Strict)
				require.JSONEq(t, strict, string(wire.JSONSchema["strict"]))
			}
			require.Equal(t, false, request.Text.Format.Schema["additionalProperties"], "conversion must not mutate the caller schema")
			if strict != "absent" {
				copied := converted.ResponseFormat.JsonSchema.Schema
				copied["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "integer"
				copied["required"].([]any)[0] = "changed"
				*converted.ResponseFormat.JsonSchema.Strict = !*converted.ResponseFormat.JsonSchema.Strict
				originalAnswer := request.Text.Format.Schema["properties"].(map[string]any)["answer"].(map[string]any)
				require.Equal(t, "string", originalAnswer["type"], "nested maps must be detached before downstream normalization")
				require.Equal(t, "answer", request.Text.Format.Schema["required"].([]any)[0], "schema arrays must be detached")
				require.Equal(t, strict == "true", *request.Text.Format.Strict, "strict pointers must be detached")
			}
		})
	}
}

// TestClientCompatibilityCodexNamespaceHistory rejects ambiguous same-leaf calls even without Tools.
func TestClientCompatibilityCodexNamespaceHistory(t *testing.T) {
	t.Parallel()
	for _, namespaced := range []bool{false, true} {
		t.Run(fmt.Sprintf("namespaced=%t", namespaced), func(t *testing.T) {
			alpha, beta := "", ""
			if namespaced {
				alpha, beta = `,"namespace":"alpha"`, `,"namespace":"beta"`
			}
			request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":[{"type":"message","role":"user","content":"lookup"},{"type":"function_call","id":"fc_alpha","call_id":"call_alpha","name":"lookup","arguments":"{}"`+alpha+`},{"type":"function_call","id":"fc_beta","call_id":"call_beta","name":"lookup","arguments":"{}"`+beta+`},{"type":"function_call_output","call_id":"call_alpha","output":"alpha-result"},{"type":"function_call_output","call_id":"call_beta","output":"beta-result"}]}`)
			require.Empty(t, request.Tools)
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			if namespaced {
				require.Error(t, err, "same leaf names cannot silently lose namespace identity")
				require.Nil(t, converted)
				return
			}
			require.NoError(t, err)
			require.Len(t, converted.Messages, 4)
			calls := converted.Messages[1].ToolCalls
			require.Len(t, calls, 2)
			require.NotEqual(t, calls[0].Id, calls[1].Id)
			require.Equal(t, calls[0].Id, converted.Messages[2].ToolCallId)
			require.Equal(t, calls[1].Id, converted.Messages[3].ToolCallId)
			require.Equal(t, "alpha-result", converted.Messages[2].StringContent())
			require.Equal(t, "beta-result", converted.Messages[3].StringContent())
		})
	}
}

// TestClientCompatibilityCodexMixedContent forbids successful conversion that silently loses parts.
func TestClientCompatibilityCodexMixedContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, part string
		file       bool
	}{
		{"file", `{"type":"input_file","file_id":"file_fixture","filename":"report.pdf"}`, true},
		{"unknown", `{"type":"future_binary","opaque_data":"opaque-canary"}`, false},
		{"unknown_with_text", `{"type":"future_binary","text":"not-a-text-part","opaque_data":"opaque-canary"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect attachment"},`+tc.part+`]}]}`)
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			if err != nil {
				require.Nil(t, converted)
				return
			}
			require.True(t, tc.file, "unrepresentable parts must fail explicitly rather than become text")
			require.Len(t, converted.Messages, 1)
			parts := converted.Messages[0].ParseContent()
			require.Len(t, parts, 2, "keep text and attachment or explicitly reject the request")
			require.Equal(t, relaymodel.ContentTypeText, parts[0].Type)
			require.Equal(t, "inspect attachment", *parts[0].Text)
			require.Equal(t, relaymodel.ContentTypeFile, parts[1].Type)
			require.Equal(t, "file_fixture", parts[1].FileID)
			require.Equal(t, "report.pdf", parts[1].Filename)
		})
	}
}

// TestClientCompatibilityCodexCompaction refuses JSON-text or empty-message lowering of unknown opaque state.
func TestClientCompatibilityCodexCompaction(t *testing.T) {
	t.Parallel()
	request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":[{"type":"compaction","id":"cmp_fixture","encrypted_content":"opaque-compaction-canary"},{"type":"message","role":"user","content":"continue"}]}`)
	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.Error(t, err, "unknown compaction state has no sanctioned Chat lowering")
	require.Nil(t, converted)
}

// TestClientCompatibilityCodexReasoningSidecar preserves the existing lossy portability policy explicitly.
func TestClientCompatibilityCodexReasoningSidecar(t *testing.T) {
	t.Parallel()
	for _, summary := range []string{`[]`, `[{"type":"summary_text","text":"visible summary"}]`} {
		t.Run(summary, func(t *testing.T) {
			continuation := `{"type":"message","role":"user","content":"continue"}`
			if summary != "[]" {
				continuation = `{"type":"function_call","id":"fc_read","call_id":"call_read","name":"read_file","arguments":"{}"},{"type":"function_call_output","call_id":"call_read","output":"file contents"},` + continuation
			}
			request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":[{"type":"reasoning","id":"rs_fixture","encrypted_content":"opaque-reasoning-canary","summary":`+summary+`},`+continuation+`]}`)
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.NoError(t, err, "reasoning sidecars intentionally degrade on stateless fallback")
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			require.NotContains(t, string(wire), "opaque-reasoning-canary")
			require.NotContains(t, string(wire), "encrypted_content")
			if summary == "[]" {
				require.Len(t, converted.Messages, 1, "opaque-only reasoning sidecar intentionally drops")
			} else {
				require.Len(t, converted.Messages, 3)
				assistant := converted.Messages[0]
				require.Equal(t, "assistant", assistant.Role)
				require.Len(t, assistant.ToolCalls, 1)
				require.NotNil(t, assistant.ReasoningContent)
				require.Equal(t, "visible summary", *assistant.ReasoningContent)
				require.Equal(t, "tool", converted.Messages[1].Role)
				require.Equal(t, assistant.ToolCalls[0].Id, converted.Messages[1].ToolCallId)
				require.Nil(t, converted.Messages[1].ReasoningContent)
			}
			followup := converted.Messages[len(converted.Messages)-1]
			require.Equal(t, "user", followup.Role)
			require.Equal(t, "continue", followup.StringContent())
			require.Nil(t, followup.ReasoningContent, "assistant reasoning must not leak into a later user turn")
		})
	}
}

// TestClientCompatibilityCodexKnownContent retains established readable representations and rejects unsupported file URLs.
func TestClientCompatibilityCodexKnownContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, part string
		rejected   bool
	}{
		{"refusal", `{"type":"refusal","refusal":"cannot complete this request"}`, false},
		{"text_alias", `{"type":"text","text":"legacy text alias"}`, false},
		{"inline_file", `{"type":"input_file","file_data":"data:application/pdf;base64,ZmFrZQ==","filename":"fixture.pdf"}`, true},
		{"file_url", `{"type":"input_file","file_url":"https://example.invalid/fixture.pdf"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := clientCompatibilityCodexRequest(t, `{"model":"fixture","input":[{"type":"message","role":"assistant","content":[`+tc.part+`]}]}`)
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			if tc.rejected {
				require.Error(t, err)
				require.Nil(t, converted)
				return
			}
			require.NoError(t, err)
			require.Len(t, converted.Messages, 1)
			switch tc.name {
			case "refusal":
				require.Equal(t, "cannot complete this request", converted.Messages[0].StringContent())
			case "text_alias":
				require.Equal(t, "legacy text alias", converted.Messages[0].StringContent())
			}
		})
	}
}
