package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestClaudeImageBlocks checks generated image representation without changing text-only output.
func TestClaudeImageBlocks(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			name := "image_only"
			if mixed {
				name = "mixed"
			}
			if stream {
				name += "_stream"
			}
			t.Run(name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				parts := []Part{{InlineData: &InlineData{MimeType: "image/png", Data: "aW1hZ2U="}}}
				if mixed {
					parts = append([]Part{{Text: "before"}}, parts...)
					parts = append(parts, Part{Text: "after"})
				}
				raw, err := json.Marshal(ChatResponse{Candidates: []ChatCandidate{{Content: ChatContent{Parts: parts}}}})
				require.NoError(t, err)
				resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
				a := &Adaptor{}
				var got *http.Response
				var failure *model.ErrorWithStatusCode
				if stream {
					got, failure = a.convertStreamingToClaudeResponse(c, resp, []byte("data: "+string(raw)+"\n\ndata: [DONE]\n\n"), &meta.Meta{ActualModelName: "gemini-nano-banana-2.1"})
				} else {
					got, failure = a.convertNonStreamingToClaudeResponse(c, resp, raw, &meta.Meta{ActualModelName: "gemini-nano-banana-2.1"})
				}
				require.Nil(t, failure)
				body, err := io.ReadAll(got.Body)
				require.NoError(t, err)
				require.Contains(t, string(body), "data:image/png;base64,aW1hZ2U=")
				if mixed {
					require.Less(t, bytes.Index(body, []byte("before")), bytes.Index(body, []byte("data:image")))
					require.Less(t, bytes.Index(body, []byte("data:image")), bytes.Index(body, []byte("after")))
				}
				if stream {
					require.True(t, strings.Contains(string(body), "text_delta"))
				}
			})
		}
	}
}

// TestClaudeNanoHiddenStreamCandidateKeepsReceiptConservative checks split candidate frames.
func TestClaudeNanoHiddenStreamCandidateKeepsReceiptConservative(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	first := `{"candidates":[{"index":1,"content":{"parts":[{"text":"hidden"},{"inlineData":{"mimeType":"image/png","data":"aGlkZGVu"}}]}}]}`
	second := `{"candidates":[{"index":0,"content":{"parts":[{"text":"visible"},{"inlineData":{"mimeType":"image/png","data":"dmlzaWJsZQ=="}}]}}]}`
	got, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, resp, []byte("data: "+first+"\n\ndata: "+second+"\n\ndata: [DONE]\n\n"), &meta.Meta{ActualModelName: "gemini-nano-banana-2.1"})
	require.Nil(t, failure)
	body, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "visible")
	require.NotContains(t, string(body), "hidden")
	receipt, ok := c.Get("claude_gemini_image_receipt")
	require.True(t, ok)
	info := receipt.(*model.ClaudeImageReceipt)
	require.True(t, info.Incomplete)
	require.Len(t, info.ImageEndOffsets, 1)
}

// TestClaudeNanoMultipleImageStreamBlocks verifies distinct output and delivery boundaries.
func TestClaudeNanoMultipleImageStreamBlocks(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	payload := `{"candidates":[{"index":0,"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"b25l"}},{"text":"between"},{"inlineData":{"mimeType":"image/png","data":"dHdv"}}]}}]}`
	got, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, resp, []byte("data: "+payload+"\n\ndata: [DONE]\n\n"), &meta.Meta{ActualModelName: "gemini-nano-banana-2.1"})
	require.Nil(t, failure)
	body, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	text := string(body)
	require.Contains(t, text, "base64,b25l")
	require.Contains(t, text, "between")
	require.Contains(t, text, "base64,dHdv")
	require.Less(t, strings.Index(text, "base64,b25l"), strings.Index(text, "between"))
	require.Less(t, strings.Index(text, "between"), strings.Index(text, "base64,dHdv"))
	require.Contains(t, text, `"index":0`)
	require.Contains(t, text, `"index":1`)
	require.Contains(t, text, `"index":2`)
	raw, ok := c.Get("claude_gemini_image_receipt")
	require.True(t, ok)
	receipt := raw.(*model.ClaudeImageReceipt)
	require.False(t, receipt.Incomplete)
	require.Len(t, receipt.ImageEndOffsets, 2)
	require.Less(t, receipt.ImageEndOffsets[0], receipt.ImageEndOffsets[1])
}
