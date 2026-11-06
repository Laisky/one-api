package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestConvertedWebSearchToolType pins the platform and model version table.
func TestConvertedWebSearchToolType(t *testing.T) {
	cases := []struct {
		name    string
		channel int
		model   string
		want    string
	}{
		{"anthropic_opus_4_6_dynamic", channeltype.Anthropic, "claude-opus-4-6", WebSearchToolTypeDynamic},
		{"anthropic_sonnet_4_6_dynamic", channeltype.Anthropic, "claude-sonnet-4-6", WebSearchToolTypeDynamic},
		{"anthropic_opus_5_5_dynamic", channeltype.Anthropic, "claude-opus-5-5-20260922", WebSearchToolTypeDynamic},
		{"anthropic_haiku_5_5_dynamic", channeltype.Anthropic, "claude-haiku-5-5", WebSearchToolTypeDynamic},
		{"anthropic_mythos_preview_dynamic", channeltype.Anthropic, "claude-mythos-preview", WebSearchToolTypeDynamic},
		{"anthropic_mythos_previewish_basic", channeltype.Anthropic, "claude-mythos-previewish", WebSearchToolTypeBasic},
		{"anthropic_sonnet_4_5_basic", channeltype.Anthropic, "claude-sonnet-4-5-20250929", WebSearchToolTypeBasic},
		{"anthropic_haiku_4_5_basic", channeltype.Anthropic, "claude-haiku-4-5", WebSearchToolTypeBasic},
		{"anthropic_unknown_alias_basic", channeltype.Anthropic, "house-model", WebSearchToolTypeBasic},
		{"azure_claude_basic", channeltype.Azure, "claude-opus-4-6", WebSearchToolTypeBasic},
		{"azure_gpt_none", channeltype.Azure, "gpt-5", ""},
		{"vertex_claude_basic", channeltype.VertextAI, "claude-opus-4-6@20260101", WebSearchToolTypeBasic},
		{"vertex_gemini_none", channeltype.VertextAI, "gemini-2.5-pro", ""},
		{"bedrock_none", channeltype.AwsClaude, "claude-opus-4-6", ""},
		{"claude_compatible_none", channeltype.ClaudeCompatible, "claude-opus-4-6", ""},
		{"openai_none", channeltype.OpenAI, "claude-opus-4-6", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.model, OriginModelName: tc.model}
			require.Equal(t, tc.want, ConvertedWebSearchToolType(info))
		})
	}
	require.Empty(t, ConvertedWebSearchToolType(nil))
}

// TestConvertedWebSearchToolFromResponsesCodex maps the Codex live declaration.
func TestConvertedWebSearchToolFromResponsesCodex(t *testing.T) {
	var tool openai.ResponseAPITool
	require.NoError(t, json.Unmarshal([]byte(`{"type":"web_search","external_web_access":true,"search_context_size":"high",
		"search_content_types":["text","image"],"filters":{"allowed_domains":[" example.com ","","docs.example.com/blog"]},
		"user_location":{"type":"approximate","country":"US","region":"California","city":"San Francisco","timezone":"America/Los_Angeles"}}`), &tool))

	converted, err := ConvertedWebSearchToolFromResponses(tool, WebSearchToolTypeDynamic)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"web_search_20260209","name":"web_search","allowed_domains":["example.com","docs.example.com/blog"],
		"user_location":{"type":"approximate","city":"San Francisco","region":"California","country":"US","timezone":"America/Los_Angeles"}}`, string(encoded))

	var bare openai.ResponseAPITool
	require.NoError(t, json.Unmarshal([]byte(`{"type":"web_search","external_web_access":false}`), &bare))
	converted, err = ConvertedWebSearchToolFromResponses(bare, WebSearchToolTypeBasic)
	require.NoError(t, err)
	encoded, err = json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"web_search_20250305","name":"web_search"}`, string(encoded))
}

// TestConvertedWebSearchToolFromResponsesRejects bounds untrusted options.
func TestConvertedWebSearchToolFromResponsesRejects(t *testing.T) {
	many := make([]string, maxConvertedWebSearchDomains+1)
	for i := range many {
		many[i] = "example.com"
	}
	manyJSON, err := json.Marshal(many)
	require.NoError(t, err)
	cases := map[string]string{
		"both_filters":    `{"type":"web_search","filters":{"allowed_domains":["a.com"],"disallowed_domains":["b.com"]}}`,
		"too_many":        `{"type":"web_search","filters":{"allowed_domains":` + string(manyJSON) + `}}`,
		"long_domain":     `{"type":"web_search","filters":{"allowed_domains":["` + strings.Repeat("a", maxConvertedWebSearchFieldBytes+1) + `"]}}`,
		"exact_location":  `{"type":"web_search","user_location":{"type":"exact","city":"Paris"}}`,
		"long_location":   `{"type":"web_search","user_location":{"type":"approximate","city":"` + strings.Repeat("c", maxConvertedWebSearchFieldBytes+1) + `"}}`,
		"unknown_version": "",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var tool openai.ResponseAPITool
			version := WebSearchToolTypeBasic
			if raw == "" {
				raw, version = `{"type":"web_search"}`, "web_search_20990101"
			}
			require.NoError(t, json.Unmarshal([]byte(raw), &tool))
			_, err := ConvertedWebSearchToolFromResponses(tool, version)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrConvertedWebSearchNotRepresentable))
		})
	}
}

// TestConvertRequestAppendsConvertedWebSearch proves the converter appends the
// recorded server tool without input_schema and leaves caller tools unchanged.
func TestConvertRequestAppendsConvertedWebSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		gmw.SetLogger(c, glog.Shared.Named("anthropic-web-search-test"))
		return c
	}
	request := func(name string) model.GeneralOpenAIRequest {
		return model.GeneralOpenAIRequest{
			Model:    "claude-opus-4-6",
			Messages: []model.Message{{Role: "user", Content: "news?"}},
			Tools: []model.Tool{{Type: "function", Function: &model.Function{
				Name: name, Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
			}}},
		}
	}
	search := &Tool{Type: WebSearchToolTypeDynamic, Name: "web_search", AllowedDomains: []string{"example.com"}}

	c := newContext()
	SetConvertedWebSearchTool(c, search)
	converted, err := ConvertRequest(c, request("lookup"))
	require.NoError(t, err)
	encoded, err := json.Marshal(converted.Tools)
	require.NoError(t, err)
	require.JSONEq(t, `[{"name":"lookup","input_schema":{"type":"object","properties":{}}},
		{"type":"web_search_20260209","name":"web_search","allowed_domains":["example.com"]}]`, string(encoded))

	// The recorded declaration is copied, so converter output cannot alias it.
	converted.Tools[1].AllowedDomains[0] = "mutated.example"
	require.Equal(t, "example.com", search.AllowedDomains[0])

	plain, err := ConvertRequest(newContext(), request("lookup"))
	require.NoError(t, err)
	require.Len(t, plain.Tools, 1)

	clash := newContext()
	SetConvertedWebSearchTool(clash, search)
	skipped, err := ConvertRequest(clash, request("web_search"))
	require.NoError(t, err)
	require.Len(t, skipped.Tools, 1, "a caller tool named web_search keeps the name")
	require.Empty(t, skipped.Tools[0].Type)
}
