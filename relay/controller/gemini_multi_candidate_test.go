package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGeminiMultipleCandidatesHTTP verifies distinct streamed choices, response items and exact billed outputs.
func TestGeminiMultipleCandidatesHTTP(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"candidates":[{"index":0,"content":{"parts":[{"text":"alpha"},{"inlineData":{"mimeType":"image/png","data":"YQ=="}}]}},{"index":1,"content":{"parts":[{"text":"beta"},{"inlineData":{"mimeType":"image/png","data":"Yg=="}}]}}]}`+"\n\n"+`data: {"candidates":[{"index":1,"content":{"parts":[{"text":"-tail"}]}}]}`+"\n\n"+`data: {"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2242,"totalTokenCount":2243,"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":2},{"modality":"IMAGE","tokenCount":2240}]}}`+"\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"draw"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"draw","max_output_tokens":1,"stream":true}`
			}
			c, rec, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", path, body, server.URL, balance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 100, CompletionRatio: 1})
			var apiErr *model.ErrorWithStatusCode
			if fallback {
				apiErr = RelayResponseAPIHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			output := rec.Body.String()
			require.Contains(t, output, "YQ==")
			require.Contains(t, output, "Yg==")
			require.Contains(t, output, "alpha")
			require.Contains(t, output, "beta")
			require.Contains(t, output, "-tail")
			if fallback {
				var completed struct {
					Response struct {
						Output []struct {
							ID      string `json:"id"`
							Type    string `json:"type"`
							Content []struct {
								Text string `json:"text"`
							} `json:"content"`
						} `json:"output"`
					} `json:"response"`
				}
				for _, line := range strings.Split(output, "\n") {
					if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"response.completed"`) {
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &completed))
					}
				}
				require.Len(t, completed.Response.Output, 2)
				a, b := completed.Response.Output[0], completed.Response.Output[1]
				require.NotEmpty(t, a.ID)
				require.NotEmpty(t, b.ID)
				require.NotEqual(t, a.ID, b.ID)
				require.Contains(t, a.Content[0].Text, "alpha")
				require.NotContains(t, a.Content[0].Text, "beta")
				require.Contains(t, b.Content[0].Text, "beta")
				require.Contains(t, b.Content[0].Text, "-tail")
			} else {
				require.Contains(t, output, `"index":1`)
			}
			expected := quota.Compute(quota.ComputeInput{Usage: &model.Usage{PromptTokens: 1, CompletionTokens: 2242, ImageTokensBilledSeparately: 2240, ToolsCost: calculateImageBaseQuota(0.0336, 0, 1, 1, 2)}, ModelName: "gemini-nano-banana-2.1", ModelRatio: 100, GroupRatio: 1, ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{"gemini-nano-banana-2.1": {Ratio: 100, CompletionRatio: 1}}})
			require.Equal(t, balance-expected.TotalQuota, reloadUserQuota(t))
		})
	}
}

// disconnectedGeminiWriter rejects all response writes to model a disconnected client.
type disconnectedGeminiWriter struct{ gin.ResponseWriter }

// Write returns a connection error without accepting the provided bytes.
func (w disconnectedGeminiWriter) Write([]byte) (int, error) {
	return 0, errors.New("client disconnected")
}

// WriteString returns a connection error without accepting the provided string.
func (w disconnectedGeminiWriter) WriteString(string) (int, error) {
	return 0, errors.New("client disconnected")
}

// TestGeminiStreamDisconnectDoesNotBillUndelivered verifies failed output cannot be counted as delivered.
func TestGeminiStreamDisconnectDoesNotBillUndelivered(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Writer = disconnectedGeminiWriter{c.Writer}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if fallback {
				c.Set(ctxkey.ResponseStreamRewriteHandler, newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: "gemini-nano-banana-2.1"}, &openai.ResponseAPIRequest{Model: "gemini-nano-banana-2.1"}))
			}
			body := `data: {"candidates":[{"index":0,"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"YQ=="}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1120,"totalTokenCount":1121,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}}` + "\n\ndata: [DONE]\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			apiErr, _, _ := gemini.StreamHandler(c, resp)
			require.NotNil(t, apiErr)
			require.Equal(t, 0, c.GetInt(ctxkey.OutputImageCount))
		})
	}
}

// secondImageShortWriter models a truncated frame containing the second image.
type secondImageShortWriter struct{ gin.ResponseWriter }

func (w secondImageShortWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Yg==") {
		return len(p) - 1, nil
	}
	return w.ResponseWriter.Write(p)
}
func (w secondImageShortWriter) WriteString(p string) (int, error) {
	if strings.Contains(p, "Yg==") {
		return len(p) - 1, nil
	}
	return w.ResponseWriter.WriteString(p)
}

// TestGeminiStreamShortWriteCountsOnlyCompleteFrames checks the observable delivery boundary.
func TestGeminiStreamShortWriteCountsOnlyCompleteFrames(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Writer = secondImageShortWriter{c.Writer}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if fallback {
				c.Set(ctxkey.ResponseStreamRewriteHandler, newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: "gemini-nano-banana-2.1"}, &openai.ResponseAPIRequest{Model: "gemini-nano-banana-2.1"}))
			}
			body := `data: {"candidates":[{"index":0,"content":{"parts":[{"text":"data:image/png;base64,text-only"},{"inlineData":{"mimeType":"image/png","data":"YQ=="}},{"inlineData":{"mimeType":"image/png","data":"Yg=="}}]}}]}` + "\n\ndata: [DONE]\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			apiErr, _, _ := gemini.StreamHandler(c, resp)
			require.NotNil(t, apiErr)
			if fallback {
				require.Equal(t, 1, c.GetInt(ctxkey.OutputImageCount))
			} else {
				require.Equal(t, 0, c.GetInt(ctxkey.OutputImageCount))
			}
			require.True(t, c.GetBool(ctxkey.GeminiOutputIncomplete))
			require.NotContains(t, rec.Body.String(), "response.completed")
		})
	}
}

// TestGeminiIncompleteOutputReceipt verifies aggregate tokens are never allocated to partial output.
func TestGeminiIncompleteOutputReceipt(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		t.Run(fmt.Sprint(incomplete), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ChannelRatio, 1.0)
			c.Set(ctxkey.OutputImageCount, 1)
			c.Set(ctxkey.GeminiOutputIncomplete, incomplete)
			usage := &model.Usage{CompletionTokens: 2242, CandidateImageTokens: 2240}
			meta := &metalib.Meta{ActualModelName: "gemini-nano-banana-2.1", ChannelType: channeltype.VertextAI, APIType: apitype.VertexAI}
			applyOutputImageCharges(c, &usage, meta)
			require.Equal(t, calculateImageBaseQuota(0.0336, 0, 1, 1, 1), usage.ToolsCost)
			if incomplete {
				require.Zero(t, usage.ImageTokensBilledSeparately)
			} else {
				require.Equal(t, 2240, usage.ImageTokensBilledSeparately)
			}
			require.Equal(t, 2242, usage.CompletionTokens)
		})
	}
}

// TestGeminiOutOfOrderCandidateFrames checks stable choice identity across independently arriving frames.
func TestGeminiOutOfOrderCandidateFrames(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if fallback {
				c.Set(ctxkey.ResponseStreamRewriteHandler, newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: "gemini-nano-banana-2.1"}, &openai.ResponseAPIRequest{Model: "gemini-nano-banana-2.1"}))
			}
			body := `data: {"candidates":[{"index":1,"content":{"parts":[{"text":"B"}]}}]}` + "\n\n" + `data: {"candidates":[{"index":0,"content":{"parts":[{"text":"A"}]}}]}` + "\n\n" + `data: {"candidates":[{"index":1,"content":{"parts":[{"text":"C"}]}}]}` + "\n\ndata: [DONE]\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			apiErr, _, _ := gemini.StreamHandler(c, resp)
			require.Nil(t, apiErr)
			var byIndex = map[int]string{}
			var byItem = map[string]string{}
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: {") {
					continue
				}
				var event struct {
					Type    string `json:"type"`
					ItemID  string `json:"item_id"`
					Delta   string `json:"delta"`
					Choices []struct {
						Index int `json:"index"`
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
				for _, choice := range event.Choices {
					byIndex[choice.Index] += choice.Delta.Content
				}
				if event.Type == "response.output_text.delta" {
					require.NotEmpty(t, event.ItemID)
					byItem[event.ItemID] += event.Delta
				}
			}
			if fallback {
				require.Len(t, byItem, 2)
				require.ElementsMatch(t, []string{"A", "BC"}, func() []string {
					v := []string{}
					for _, text := range byItem {
						v = append(v, text)
					}
					return v
				}())
			} else {
				require.Equal(t, "A", byIndex[0])
				require.Equal(t, "BC", byIndex[1])
			}
		})
	}
}
