package typesafe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecodeRequestPreservesStructure covers all primitives and structured descriptions.
func TestDecodeRequestPreservesStructure(t *testing.T) {
	body := []byte(`{"model":"jev-latest","state":{"id":9007199254740993,"items":["text",{"active":true}]},"questions":{"yes":{"type":"noul","instructions":null,"criteria":{"true":{"rubric":"yes"},"false":null}},"pick":{"type":"choice","instructions":["classify"],"criteria":{"a":null,"b":{"nested":["description"]}}},"rate":{"type":"score","instructions":{"task":"rate"},"criteria":[null,{"level":"good"},["excellent"]]}}}`)
	request, err := DecodeRequest(body)
	require.NoError(t, err)
	require.Len(t, request.Questions, 3)
	require.Equal(t, json.RawMessage(`null`), request.Questions["yes"].Instructions)
	request.Model = "jev-1.13.0"
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `9007199254740993`)
	require.NotContains(t, string(encoded), `9007199254740992`)
	decoded, err := DecodeRequest(encoded)
	require.NoError(t, err)
	require.Equal(t, request, decoded)
}

// TestDecodeRequestRejectsLossyOrMalformedShapes checks validation before dispatch.
func TestDecodeRequestRejectsLossyOrMalformedShapes(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"model":" ","state":"x","questions":{}}`,
		`{"model":"jev-latest","state":true,"questions":{"q":{"type":"noul","instructions":"?"}}}`,
		`{"model":"jev-latest","state":null,"questions":{"q":{"type":"noul","instructions":"?"}}}`,
		`{"model":"jev-latest","state":"x","questions":{}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul"}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":42}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"chat","instructions":"?"}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"score","instructions":"?","criteria":["one"]}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"choice","instructions":"?","criteria":[]}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"choice","instructions":"?","criteria":{"a":true}}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?","criteria":{"yes":"not true"}}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?","extra":true}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?"}},"stream":true}`,
		`{"model":"jev-latest","model":"other","state":"x","questions":{"q":{"type":"noul","instructions":"?"}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?"},"q":{"type":"noul","instructions":"?"}}}`,
		`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?"}}} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, err := DecodeRequest([]byte(body))
			require.Error(t, err)
		})
	}
}

// TestDecodeRequestStateShapes allows text and structured state without coercion.
func TestDecodeRequestStateShapes(t *testing.T) {
	for _, state := range []string{`"plain text"`, `[]`, `{}`, `["text",{"field":"value"}]`} {
		_, err := DecodeRequest([]byte(`{"model":"jev-preview","state":` + state + `,"questions":{"":{"type":"noul","instructions":"?"}}}`))
		require.NoError(t, err)
	}
}
