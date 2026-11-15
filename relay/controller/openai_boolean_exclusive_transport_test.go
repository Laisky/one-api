package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// validateModernExclusiveSchema checks exclusive-keyword types only at real JSON Schema nodes.
// The local transport fixture uses this bounded contract check before issuing any usage receipt.
func validateModernExclusiveSchema(node any) error {
	switch value := node.(type) {
	case map[string]any:
		for name, child := range value {
			switch name {
			case "exclusiveMinimum", "exclusiveMaximum":
				if _, ok := child.(json.Number); !ok {
					return errors.Errorf("modern schema keyword %s must be numeric", name)
				}
			case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies":
				if dictionary, ok := child.(map[string]any); ok {
					for _, schema := range dictionary {
						if err := validateModernExclusiveSchema(schema); err != nil {
							return errors.Wrap(err, "validate schema dictionary")
						}
					}
				}
			case "items", "prefixItems", "additionalItems", "contains", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "propertyNames", "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "contentSchema":
				if err := validateModernExclusiveSchema(child); err != nil {
					return errors.Wrap(err, "validate nested schema")
				}
			}
		}
	case []any:
		for _, schema := range value {
			if err := validateModernExclusiveSchema(schema); err != nil {
				return errors.Wrap(err, "validate schema list")
			}
		}
	}
	return nil
}

// TestModernExclusiveFixtureSchemaContexts prevents property names and literal JSON objects from becoming false validation failures.
func TestModernExclusiveFixtureSchemaContexts(t *testing.T) {
	for _, raw := range []string{
		`{"properties":{"exclusiveMinimum":{"type":"boolean"},"exclusiveMaximum":{"type":"boolean"}}}`,
		`{"const":{"exclusiveMinimum":true},"enum":[{"exclusiveMaximum":false}],"default":{"exclusiveMinimum":true},"examples":[{"exclusiveMaximum":false}]}`,
		`{"properties":{"n":{"exclusiveMinimum":1e3,"exclusiveMaximum":1001}}}`,
	} {
		var schema any
		require.NoError(t, relaymodel.DecodeSchemaJSON([]byte(raw), &schema))
		require.NoError(t, validateModernExclusiveSchema(schema))
	}
	for _, raw := range []string{
		`{"exclusiveMinimum":true}`, `{"exclusiveMaximum":false}`,
		`{"properties":{"n":{"exclusiveMinimum":false}}}`,
		`{"allOf":[{"properties":{"items":{"items":{"exclusiveMaximum":true}}}}]}`,
	} {
		var schema any
		require.NoError(t, relaymodel.DecodeSchemaJSON([]byte(raw), &schema))
		require.ErrorContains(t, validateModernExclusiveSchema(schema), "must be numeric")
	}
}

// openAIBooleanExclusiveSchema returns inclusive integer and fractional limits with legacy booleans or modern numeric exclusivity.
func openAIBooleanExclusiveSchema(variant string) string {
	lower, upper := variant, variant
	fractionLower, fractionUpper := variant, variant
	if variant == "numeric" {
		lower, upper = "9007199254740993", "9007199254740997"
		fractionLower, fractionUpper = "0.123456789012345678901234567890", "0.987654321098765432109876543210"
	}
	return `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740997,"exclusiveMinimum":` + lower + `,"exclusiveMaximum":` + upper + `},"exclusiveMinimum":{"type":"string"},"nested":{"type":"array","items":{"type":"object","properties":{"fraction":{"type":"number","minimum":0.123456789012345678901234567890,"maximum":0.987654321098765432109876543210,"exclusiveMinimum":` + fractionLower + `,"exclusiveMaximum":` + fractionUpper + `}},"required":["fraction"],"additionalProperties":false}}},"required":["count","exclusiveMinimum","nested"],"additionalProperties":false}`
}

// TestOpenAILegacyExclusiveBoundsTransport qualifies legacy boolean lowering against a local modern-schema validator through real controller HTTP dispatch.
func TestOpenAILegacyExclusiveBoundsTransport(t *testing.T) {
	for _, surface := range []string{"chat", "responses"} {
		for _, variant := range []string{"true", "false", "numeric"} {
			t.Run(surface+"/"+variant, func(t *testing.T) {
				const balance int64 = 1_000_000
				xaiVideoSetup(t, balance, false)
				var calls, receipts atomic.Int32
				observed := make(chan struct {
					body       []byte
					path       string
					err        error
					validation error
					remaining  int64
				}, 1)
				provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if calls.Add(1) > 1 {
						http.Error(w, "unexpected local provider retry", http.StatusBadRequest)
						return
					}
					body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
					var payload struct {
						Text struct {
							Format struct {
								Schema json.RawMessage `json:"schema"`
							} `json:"format"`
						} `json:"text"`
					}
					if err == nil {
						err = json.Unmarshal(body, &payload)
					}
					var schema any
					if err == nil {
						err = relaymodel.DecodeSchemaJSON(payload.Text.Format.Schema, &schema)
					}
					var user model.User
					if err == nil {
						err = model.DB.First(&user, fallbackUserID).Error
					}
					var validation error
					if err == nil {
						validation = validateModernExclusiveSchema(schema)
					}
					observed <- struct {
						body       []byte
						path       string
						err        error
						validation error
						remaining  int64
					}{body: body, path: r.URL.Path, err: err, validation: validation, remaining: user.Quota}
					w.Header().Set("Content-Type", "application/json")
					if err != nil || validation != nil {
						w.WriteHeader(http.StatusBadRequest)
						if _, writeErr := io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"invalid_json_schema","message":"modern exclusiveMinimum and exclusiveMaximum must be numeric"}}`); writeErr != nil {
							return
						}
						return
					}
					receipts.Add(1)
					result := map[string]any{
						"id": "resp_exclusive_fixture", "object": "response", "created_at": 1, "status": "completed", "model": "gpt-6.1-sol",
						"output": []any{map[string]any{"id": "msg_exclusive", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": `{"count":9007199254740995,"exclusiveMinimum":"property","nested":[{"fraction":0.5}]}`, "annotations": []any{}}}}},
						"usage":  map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
					}
					if err := json.NewEncoder(w).Encode(result); err != nil {
						return
					}
				}))
				t.Cleanup(provider.Close)
				priorClient := client.HTTPClient
				localClient := provider.Client()
				target, err := url.Parse(provider.URL)
				require.NoError(t, err)
				localClient.Transport = openAINumericBoundsTransport{transport: localClient.Transport, target: target}
				client.HTTPClient = localClient
				t.Cleanup(func() { client.HTTPClient = priorClient })
				schema := openAIBooleanExclusiveSchema(variant)
				path, relay := "/v1/chat/completions", RelayTextHelper
				body := `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"Extract values."}],"response_format":{"type":"json_schema","json_schema":{"name":"exclusive_fixture","strict":true,"schema":` + schema + `}}}`
				if surface == "responses" {
					path, relay = "/v1/responses", RelayResponseAPIHelper
					body = `{"model":"alias","store":false,"max_output_tokens":32,"input":"Extract values.","text":{"format":{"type":"json_schema","name":"exclusive_fixture","strict":true,"schema":` + schema + `}}}`
				}
				c, recorder, id := protocolContext(t, channeltype.OpenAI, "gpt-6.1-sol", path, body, "https://api.openai.com", balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				responseNamespaceCleanLedger(t, id)
				apiErr := relay(c)
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				got := <-observed
				require.NoError(t, got.err)
				require.Equal(t, "/v1/responses", got.path)
				source, err := common.GetRequestBody(c)
				require.NoError(t, err)
				require.Equal(t, body, string(source), "caller bytes and numeric tokens remain unchanged")
				if got.validation != nil {
					require.NotNil(t, apiErr)
					require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
					require.Zero(t, receipts.Load(), "invalid schemas are rejected before a provider usage receipt")
					require.Equal(t, got.remaining, reloadUserQuota(t), "a rejected schema must not add a charge to the existing conservative dispatch hold")
					t.Logf("local schema rejection: status=%d receipts=%d validation=%v", apiErr.StatusCode, receipts.Load(), got.validation)
				}
				require.Nil(t, apiErr, "valid legacy or modern numeric intent must reach a modern-schema upstream successfully")
				require.NoError(t, got.validation)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.EqualValues(t, 1, receipts.Load())
				require.EqualValues(t, 5, requestCostQuota(t, id))
				require.Equal(t, balance-5, reloadUserQuota(t))
				var wire struct {
					Model string `json:"model"`
					Text  struct {
						Format struct {
							Strict *bool `json:"strict"`
							Schema struct {
								Properties map[string]json.RawMessage `json:"properties"`
							} `json:"schema"`
						} `json:"format"`
					} `json:"text"`
				}
				require.NoError(t, json.Unmarshal(got.body, &wire))
				require.Equal(t, "gpt-6.1-sol", wire.Model)
				require.NotNil(t, wire.Text.Format.Strict)
				require.True(t, *wire.Text.Format.Strict)
				require.JSONEq(t, `{"type":"string"}`, string(wire.Text.Format.Schema.Properties["exclusiveMinimum"]))
				var nested struct {
					Items struct {
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"items"`
				}
				require.NoError(t, json.Unmarshal(wire.Text.Format.Schema.Properties["nested"], &nested))
				for _, entry := range []struct {
					raw          json.RawMessage
					lower, upper string
				}{
					{wire.Text.Format.Schema.Properties["count"], "9007199254740993", "9007199254740997"},
					{nested.Items.Properties["fraction"], "0.123456789012345678901234567890", "0.987654321098765432109876543210"},
				} {
					var node map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(entry.raw, &node))
					if variant == "false" {
						require.NotContains(t, node, "exclusiveMinimum")
						require.NotContains(t, node, "exclusiveMaximum")
						require.Equal(t, entry.lower, string(node["minimum"]))
						require.Equal(t, entry.upper, string(node["maximum"]))
					} else {
						require.Equal(t, entry.lower, string(node["exclusiveMinimum"]))
						require.Equal(t, entry.upper, string(node["exclusiveMaximum"]))
					}
				}
			})
		}
	}
}
