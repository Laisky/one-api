package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// codexSearchToolCallParams mirrors Codex's SearchToolCallParams
// (codex-rs protocol/src/models.rs): query is a required string and limit an
// optional unsigned integer; unknown keys are ignored.
type codexSearchToolCallParams struct {
	Query string  `json:"query"`
	Limit *uint64 `json:"limit"`
}

// decodeCodexSearchToolCallParams decodes raw the way Codex parses
// tool_search_call arguments: a JSON object with a required string query, an
// optional non-negative integer limit (floats such as 8.0 and strings are
// rejected), no repeated query or limit member, and unknown keys ignored.
// Parameters: raw is the rendered arguments. Returns the parameters or the
// first contract violation.
func decodeCodexSearchToolCallParams(raw []byte) (codexSearchToolCallParams, error) {
	var params codexSearchToolCallParams
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return params, err
	}
	if token != json.Delim('{') {
		return params, io.ErrUnexpectedEOF
	}
	seen := map[string]int{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return params, err
		}
		seen[key.(string)]++
		var skip json.RawMessage
		if err := decoder.Decode(&skip); err != nil {
			return params, err
		}
	}
	if seen["query"] != 1 || seen["limit"] > 1 {
		return params, io.ErrUnexpectedEOF
	}
	// Strict types: DisallowUnknownFields stays off, as serde ignores them.
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&params); err != nil {
		return params, err
	}
	return params, nil
}

// TestResponseToolSearchArgumentsCodexContract proves every rendered
// tool_search_call carries arguments Codex can parse: query is always a
// string, limit is either a positive JSON integer or absent, non-object input
// becomes {"query":""}, and unrelated members keep their exact bytes.
func TestResponseToolSearchArgumentsCodexContract(t *testing.T) {
	t.Parallel()
	uint64Ptr := func(value uint64) *uint64 { return &value }
	cases := map[string]struct {
		in        string
		wantQuery string
		wantLimit *uint64
		wantRaw   string // exact rendered bytes when non-empty
	}{
		"fractional_limit":     {in: `{"query":"docs","limit":2.5}`, wantQuery: "docs", wantRaw: `{"query":"docs"}`},
		"float_integral_limit": {in: `{"query":"docs","limit":8.0}`, wantQuery: "docs", wantLimit: uint64Ptr(8), wantRaw: `{"limit":8,"query":"docs"}`},
		"exponent_limit":       {in: `{"query":"docs","limit":1e2}`, wantQuery: "docs", wantLimit: uint64Ptr(100)},
		"fraction_exponent":    {in: `{"query":"docs","limit":2.50e1}`, wantQuery: "docs", wantLimit: uint64Ptr(25)},
		"tiny_exponent":        {in: `{"query":"docs","limit":5e-1}`, wantQuery: "docs"},
		"string_limit":         {in: `{"query":"docs","limit":"8"}`, wantQuery: "docs", wantLimit: uint64Ptr(8), wantRaw: `{"limit":8,"query":"docs"}`},
		"string_float_limit":   {in: `{"query":"docs","limit":"8.0"}`, wantQuery: "docs"},
		"string_word_limit":    {in: `{"query":"docs","limit":"eight"}`, wantQuery: "docs"},
		"string_padded_limit":  {in: `{"query":"docs","limit":" 8"}`, wantQuery: "docs"},
		"string_negative":      {in: `{"query":"docs","limit":"-2"}`, wantQuery: "docs"},
		"negative_limit":       {in: `{"query":"docs","limit":-1}`, wantQuery: "docs", wantRaw: `{"query":"docs"}`},
		"negative_float_limit": {in: `{"query":"docs","limit":-3.0}`, wantQuery: "docs"},
		"zero_limit":           {in: `{"query":"docs","limit":0}`, wantQuery: "docs"},
		"negative_zero_limit":  {in: `{"query":"docs","limit":-0.0}`, wantQuery: "docs"},
		"int64_max_limit":      {in: `{"query":"docs","limit":9223372036854775807}`, wantQuery: "docs", wantLimit: uint64Ptr(9223372036854775807)},
		"beyond_int64_limit":   {in: `{"query":"docs","limit":9223372036854775808}`, wantQuery: "docs"},
		"beyond_float_limit":   {in: `{"query":"docs","limit":9007199254740993}`, wantQuery: "docs", wantLimit: uint64Ptr(9007199254740993)},
		"overflow_limit":       {in: `{"query":"docs","limit":1e300}`, wantQuery: "docs"},
		"huge_exponent_limit":  {in: `{"query":"docs","limit":1e99999999999999999999}`, wantQuery: "docs"},
		"null_limit":           {in: `{"query":"docs","limit":null}`, wantQuery: "docs"},
		"bool_limit":           {in: `{"query":"docs","limit":true}`, wantQuery: "docs"},
		"array_limit":          {in: `{"query":"docs","limit":[8]}`, wantQuery: "docs"},
		"object_limit":         {in: `{"query":"docs","limit":{"n":8}}`, wantQuery: "docs"},
		"number_query":         {in: `{"query":5,"limit":3}`, wantQuery: "5", wantLimit: uint64Ptr(3), wantRaw: `{"limit":3,"query":"5"}`},
		"decimal_query":        {in: `{"query":1.50}`, wantQuery: "1.50", wantRaw: `{"query":"1.50"}`},
		"bool_query":           {in: `{"query":false}`, wantQuery: "false"},
		"null_query":           {in: `{"query":null}`, wantQuery: "", wantRaw: `{"query":""}`},
		"array_query":          {in: `{"query":["docs"]}`, wantQuery: ""},
		"object_query":         {in: `{"query":{"text":"docs"}}`, wantQuery: ""},
		"missing_query":        {in: `{"limit":3}`, wantQuery: "", wantLimit: uint64Ptr(3), wantRaw: `{"limit":3,"query":""}`},
		"empty_object":         {in: `{}`, wantQuery: "", wantRaw: `{"query":""}`},
		"duplicate_query":      {in: `{"query":"a","query":"b"}`, wantQuery: "b", wantRaw: `{"query":"b"}`},
		"array_arguments":      {in: `["docs"]`, wantQuery: "", wantRaw: `{"query":""}`},
		"string_arguments":     {in: `"docs"`, wantQuery: "", wantRaw: `{"query":""}`},
		"number_arguments":     {in: `8`, wantQuery: "", wantRaw: `{"query":""}`},
		"null_arguments":       {in: `null`, wantQuery: "", wantRaw: `{"query":""}`},
		"empty_arguments":      {in: ``, wantQuery: "", wantRaw: `{"query":""}`},
		"invalid_json":         {in: `{"query":"docs"`, wantQuery: "", wantRaw: `{"query":""}`},
		"trailing_value":       {in: `{"query":"docs"} {"query":"x"}`, wantQuery: "", wantRaw: `{"query":""}`},
		"valid_canonical":      {in: " {\"query\": \"docs\", \"cursor\": 1.50}\n", wantQuery: "docs", wantRaw: `{"cursor":1.50,"query":"docs"}`},
		"unknown_members_kept": {
			in:        `{"query":"a<b","limit":2.5,"cursor":9007199254740993,"nested":{ "x" : [1, 2.0] },"note":"q\"<&>","odd\u0001key":true}`,
			wantQuery: "a<b",
			wantRaw:   `{"cursor":9007199254740993,"nested":{"x":[1,2.0]},"note":"q\"<&>","odd\u0001key":true,"query":"a<b"}`,
		},
	}
	catalog, _ := fallbackRenderCatalog(t)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			normalized := normalizeResponseToolSearchArguments(tc.in)
			if tc.wantRaw != "" {
				require.Equal(t, tc.wantRaw, string(normalized))
			}

			// The rendered item travels on the wire; decode its arguments the
			// way Codex does.
			item := catalog.ResponseFallbackToolCallItem("call_1", "tool_search", tc.in)
			require.Equal(t, "tool_search_call", item.Type)
			encoded, err := json.Marshal(item)
			require.NoError(t, err)
			var wire struct {
				Type      string          `json:"type"`
				CallID    string          `json:"call_id"`
				Arguments json.RawMessage `json:"arguments"`
			}
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Equal(t, "tool_search_call", wire.Type)
			require.Equal(t, "call_1", wire.CallID)
			params, err := decodeCodexSearchToolCallParams(wire.Arguments)
			require.NoError(t, err, "Codex must parse arguments %s", string(wire.Arguments))
			require.Equal(t, tc.wantQuery, params.Query)
			require.Equal(t, tc.wantLimit, params.Limit)
		})
	}
}

// TestResponseToolSearchArgumentsKeepsValidInput proves arguments that already
// satisfy the client contract render exactly as before the contract change.
func TestResponseToolSearchArgumentsKeepsValidInput(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`{"query":"docs","limit":8}`:                                `{"limit":8,"query":"docs"}`,
		`{"query":"docs"}`:                                          `{"query":"docs"}`,
		`{"query": "x", "n": 1.50}`:                                 `{"n":1.50,"query":"x"}`,
		`{"query":"docs","limit":4,"large":9007199254740993}`:       `{"large":9007199254740993,"limit":4,"query":"docs"}`,
		"\t{\"query\":\"\"}\n":                                      `{"query":""}`,
		`{"query":"docs","limit":8,"exact":"keep","z":[1,{"a":2}]}`: `{"exact":"keep","limit":8,"query":"docs","z":[1,{"a":2}]}`,
	} {
		require.Equal(t, want, string(normalizeResponseToolSearchArguments(in)), in)
	}
}
