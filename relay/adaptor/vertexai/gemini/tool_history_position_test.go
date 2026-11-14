package vertexai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestVertexToolHistoryPositionBinding preserves each call's identity when native positions differ from source array order.
func TestVertexToolHistoryPositionBinding(t *testing.T) {
	for _, tc := range []struct {
		name              string
		reverse, sameName bool
	}{{"ordinary_control", false, false}, {"restored_order", true, false}, {"same_name_restored_order", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			names := []string{"alpha", "beta"}
			if tc.sameName {
				names = []string{"lookup", "lookup"}
			}
			positions := []int{0, 1}
			if tc.reverse {
				positions = []int{1, 0}
			}
			request := vertexContinuationRequest()
			request.Messages[1].ToolCalls = []model.Tool{
				{Id: "call_a", Type: "function", Function: &model.Function{Name: names[0], Arguments: `{"value":"A","number":9007199254740993}`}, ExtraContent: model.WithGeminiPartIndex(model.GoogleThoughtSignature("sig-A"), positions[0])},
				{Id: "call_b", Type: "function", Function: &model.Function{Name: names[1], Arguments: `{"value":"B","number":9007199254740995}`}, ExtraContent: model.WithGeminiPartIndex(model.GoogleThoughtSignature("sig-B"), positions[1])},
			}
			request.Messages[2].Content = `{"result":"B"}`
			request.Messages[3].Content = `{"result":"A"}`
			before, err := json.Marshal(request)
			require.NoError(t, err)
			var wire struct {
				Contents []struct {
					Parts []map[string]json.RawMessage `json:"parts"`
				} `json:"contents"`
			}
			raw := vertexSchemaWire(t, request)
			require.NoError(t, json.Unmarshal(raw, &wire))
			require.Len(t, wire.Contents, 3)
			require.Len(t, wire.Contents[1].Parts, 2)
			require.Len(t, wire.Contents[2].Parts, 2)
			order := []int{0, 1}
			if tc.reverse {
				order = []int{1, 0}
			}
			args := []string{`{"value":"A","number":9007199254740993}`, `{"value":"B","number":9007199254740995}`}
			labels := []string{"A", "B"}
			for index, source := range order {
				part := wire.Contents[1].Parts[index]
				var call struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				}
				require.NoError(t, json.Unmarshal(part["functionCall"], &call))
				require.Equal(t, names[source], call.Name)
				require.JSONEq(t, args[source], string(call.Args), "argument object must follow its original call identity")
				require.Contains(t, string(call.Args), []string{"9007199254740993", "9007199254740995"}[source], "number literals must not lose precision")
				require.JSONEq(t, `"sig-`+labels[source]+`"`, string(part["thoughtSignature"]))
				require.NotContains(t, string(part["functionCall"]), `"id"`)
				expected := `{"name":"` + names[source] + `","response":{"result":"` + labels[source] + `"}}`
				require.JSONEq(t, expected, string(wire.Contents[2].Parts[index]["functionResponse"]), "results must follow restored native call order")
			}
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
			require.NotContains(t, string(raw), "sourceToolCallID")
		})
	}
}
