package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestProtocolAuditZhipuVideoLedger proves per-call and explicit free tariffs,
// both native auth variants, mapped JSON preservation, task persistence, polling
// affinity and failure refunds using real HTTP and physical account records.
func TestProtocolAuditZhipuVideoLedger(t *testing.T) {
	for _, tc := range []struct {
		name, actual               string
		channel, status            int
		cost                       int64
		group                      float64
		override                   *model.ModelConfigLocal
		failedEnvelope, disconnect bool
	}{
		{name: "cog3", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 200, cost: 71429, group: 1},
		{name: "cog2", actual: "cogvideox-2", channel: channeltype.Zhipu, status: 200, cost: 35715, group: 1},
		{name: "free_flash", actual: "cogvideox-flash", channel: channeltype.Zhipu, status: 200, cost: 0, group: 1},
		{name: "q1_text", actual: "viduq1-text", channel: channeltype.Zhipu, status: 200, cost: 178572, group: 1},
		{name: "q1_image", actual: "viduq1-image", channel: channeltype.Zhipu, status: 200, cost: 178572, group: 1},
		{name: "q1_frames", actual: "viduq1-start-end", channel: channeltype.Zhipu, status: 200, cost: 178572, group: 1},
		{name: "v2_image", actual: "vidu2-image", channel: channeltype.Zhipu, status: 200, cost: 89286, group: 1},
		{name: "v2_frames", actual: "vidu2-start-end", channel: channeltype.Zhipu, status: 200, cost: 89286, group: 1},
		{name: "v2_reference", actual: "vidu2-reference", channel: channeltype.Zhipu, status: 200, cost: 178572, group: 1},
		{name: "international", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, cost: 100000, group: 1},
		{name: "group", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, cost: 150000, group: 1.5},
		{name: "free_group", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, group: 0},
		{name: "metadata_only", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, cost: 100000, group: 1, override: &model.ModelConfigLocal{MaxTokens: 123}},
		{name: "operator_per_call", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, cost: 10000, group: 1, override: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 20}}},
		{name: "operator_free", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, group: 1, override: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{}}},
		{name: "legacy_ratio_override", actual: "cogvideox-3", channel: channeltype.Zai, status: 200, cost: 70, group: 2, override: &model.ModelConfigLocal{Ratio: 35}},
		{name: "refused", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 403, group: 1},
		{name: "limited", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 429, group: 1},
		{name: "overloaded", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 503, group: 1},
		{name: "failed_task", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 200, group: 1, failedEnvelope: true},
		{name: "disconnected", actual: "cogvideox-3", channel: channeltype.Zhipu, status: 200, cost: 71429, group: 1, disconnect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1000000)
			xaiVideoSetup(t, balance, false)
			seen := make(chan map[string]any, 1)
			var creates, polls atomic.Int32
			job := "protocol-" + tc.name
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					polls.Add(1)
					if r.URL.Path != "/api/paas/v4/async-result/"+job {
						w.WriteHeader(404)
						return
					}
					_, _ = io.WriteString(w, `{"task_status":"SUCCESS","video_result":[{"url":"https://example.com/result.mp4"}]}`)
					return
				}
				creates.Add(1)
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				var payload map[string]any
				err := decoder.Decode(&payload)
				select {
				case seen <- map[string]any{"path": r.URL.Path, "body": payload, "error": err, "auth": r.Header.Get("Authorization")}:
				default:
					// The atomic call count detects duplicates without blocking server shutdown.
				}
				w.WriteHeader(tc.status)
				if tc.status >= 400 {
					_, _ = io.WriteString(w, `{"error":{"code":1302,"message":"rejected"}}`)
					return
				}
				status := "PROCESSING"
				if tc.failedEnvelope {
					status = "FAIL"
				}
				_, _ = fmt.Fprintf(w, `{"id":%q,"task_status":%q,"request_id":"provider-request"}`, job, status)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			payload := `{"model":"alias","prompt":"Animate this scene.","n":1,"with_audio":false,"seed":123,"future_extension":9007199254740993}`
			var native map[string]any
			require.NoError(t, json.Unmarshal([]byte(payload), &native))
			switch tc.actual {
			case "viduq1-image", "vidu2-image":
				native["image_url"] = "https://example.com/first.png"
			case "viduq1-start-end", "vidu2-start-end":
				native["image_url"] = []string{"https://example.com/first.png", "https://example.com/last.png"}
			case "vidu2-reference":
				native["image_url"] = []string{"https://example.com/reference.png"}
			}
			// Raw extension precision is tested independently of provider seed limits.
			native["future_extension"] = json.Number("9007199254740993")
			body, encodeErr := json.Marshal(native)
			require.NoError(t, encodeErr)
			payload = string(body)
			c, _, id := protocolContext(t, tc.channel, tc.actual, "/v1/videos", payload, server.URL+"/api/paas/v4", balance, tc.group, false, tc.override)
			if tc.channel == channeltype.Zhipu {
				c.Request.Header.Set("Authorization", "Bearer fixture.secret")
			}
			if tc.disconnect {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayVideoHelper(c)
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.cost, reloadUserQuota(t))
			require.Equal(t, tc.cost, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, tc.cost, token.UsedQuota)
			var observed map[string]any
			select {
			case observed = <-seen:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream create request was not received")
			}
			require.Nil(t, observed["error"])
			require.Equal(t, "/api/paas/v4/videos/generations", observed["path"])
			require.Equal(t, tc.actual, observed["body"].(map[string]any)["model"])
			require.Equal(t, json.Number("9007199254740993"), observed["body"].(map[string]any)["future_extension"])
			require.Equal(t, false, observed["body"].(map[string]any)["with_audio"])
			if tc.channel == channeltype.Zai {
				require.Equal(t, "Bearer upstream-fixture-key", observed["auth"])
			} else {
				require.Len(t, strings.Split(observed["auth"].(string), "."), 3)
			}
			require.EqualValues(t, 1, creates.Load(), "exactly one creation even on failure")
			if tc.status >= 400 || tc.failedEnvelope {
				require.NotNil(t, apiErr)
				return
			}
			if tc.disconnect {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
			}
			binding, err := model.GetAsyncTaskBindingByTaskID(context.Background(), job)
			require.NoError(t, err)
			require.Equal(t, fallbackUserID, binding.UserID)
			require.Equal(t, tc.channel, binding.ChannelType)
			for range 2 {
				poll, response, _ := protocolContext(t, tc.channel, tc.actual, "/v1/videos/"+job, "", server.URL+"/api", balance, tc.group, false, tc.override)
				poll.Request.Method = http.MethodGet
				require.Nil(t, RelayVideoHelper(poll))
				require.Contains(t, response.Body.String(), "video_result")
				drainCriticalTasks(t)
				require.Equal(t, balance-tc.cost, reloadUserQuota(t))
			}
			require.EqualValues(t, 1, creates.Load())
			require.EqualValues(t, 2, polls.Load())
		})
	}
}

// TestProtocolAuditSiliconFlowImages verifies all five catalog image models,
// native wire conversion, large extension numbers, exact fixed-image charges,
// administrator tariffs and accepted-versus-rejected output accounting.
func TestProtocolAuditSiliconFlowImages(t *testing.T) {
	for _, tc := range []struct {
		name, actual          string
		status                int
		cost                  int64
		group                 float64
		override              *model.ModelConfigLocal
		malformed, disconnect bool
	}{
		{name: "schnell", actual: "black-forest-labs/FLUX.1-schnell", status: 200, cost: 700, group: 1},
		{name: "pro11", actual: "black-forest-labs/FLUX.1.1-pro", status: 200, cost: 20000, group: 1},
		{name: "pro11_canonical", actual: "black-forest-labs/FLUX-1.1-pro", status: 200, cost: 20000, group: 1},
		{name: "zimage_canonical", actual: "Tongyi-MAI/Z-Image-Turbo", status: 200, cost: 2500, group: 1},
		{name: "pro2", actual: "black-forest-labs/FLUX.2-pro", status: 200, cost: 15000, group: 1},
		{name: "flex2", actual: "black-forest-labs/FLUX.2-flex", status: 200, cost: 30000, group: 1},
		{name: "zimage", actual: "Bytedance/Z-Image-Turbo", status: 200, cost: 2500, group: 1},
		{name: "group", actual: "Bytedance/Z-Image-Turbo", status: 200, cost: 3750, group: 1.5},
		{name: "free_group", actual: "Bytedance/Z-Image-Turbo", status: 200, group: 0},
		{name: "override", actual: "Bytedance/Z-Image-Turbo", status: 200, cost: 1000, group: 1, override: &model.ModelConfigLocal{Image: &model.ImagePricingLocal{PricePerImageUsd: .002}}},
		{name: "refused", actual: "Bytedance/Z-Image-Turbo", status: 403, group: 1},
		{name: "limited", actual: "Bytedance/Z-Image-Turbo", status: 429, group: 1},
		{name: "overloaded", actual: "Bytedance/Z-Image-Turbo", status: 503, group: 1},
		{name: "malformed", actual: "Bytedance/Z-Image-Turbo", status: 200, group: 1, malformed: true},
		{name: "disconnected", actual: "Bytedance/Z-Image-Turbo", status: 200, cost: 2500, group: 1, disconnect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1000000)
			xaiVideoSetup(t, balance, false)
			seen := make(chan map[string]any, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				var payload map[string]any
				err := decoder.Decode(&payload)
				select {
				case seen <- map[string]any{"body": payload, "error": err, "path": r.URL.Path, "auth": r.Header.Get("Authorization"), "watermark": r.Header.Get("X-Enable-Watermark")}:
				default:
					// The atomic call count detects duplicates without blocking server shutdown.
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Siliconcloud-Trace-Id", "trace-fixture")
				w.WriteHeader(tc.status)
				if tc.status >= 400 {
					_, _ = io.WriteString(w, `{"code":50505,"message":"rejected"}`)
					return
				}
				if tc.malformed {
					_, _ = io.WriteString(w, `{"images":[]}`)
					return
				}
				_, _ = io.WriteString(w, `{"images":[{"url":"https://example.com/generated.png"}],"seed":123,"timings":{"inference":0.1}}`)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			size := "1024x1024"
			if tc.actual == "black-forest-labs/FLUX.2-pro" {
				size = "512x512"
			}
			body := fmt.Sprintf(`{"model":"alias","prompt":"A paper boat.","size":%q,"n":1,"response_format":"url","extra_body":{"seed":9999999999,"future_extension":9007199254740993}}`, size)
			c, w, id := protocolContext(t, channeltype.SiliconFlow, tc.actual, "/v1/images/generations", body, server.URL, balance, tc.group, false, tc.override)
			if tc.disconnect {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.cost, reloadUserQuota(t))
			require.Equal(t, tc.cost, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, tc.cost, token.UsedQuota)
			var observed map[string]any
			select {
			case observed = <-seen:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream create request was not received")
			}
			require.Nil(t, observed["error"])
			wire := observed["body"].(map[string]any)
			expectedModel := tc.actual
			if expectedModel == "Bytedance/Z-Image-Turbo" {
				expectedModel = "Tongyi-MAI/Z-Image-Turbo"
			}
			if expectedModel == "black-forest-labs/FLUX.1.1-pro" {
				expectedModel = "black-forest-labs/FLUX-1.1-pro"
			}
			require.Equal(t, expectedModel, wire["model"])
			require.Equal(t, size, wire["image_size"])
			require.Equal(t, json.Number("9007199254740993"), wire["future_extension"])
			require.Equal(t, json.Number("9999999999"), wire["seed"])
			require.NotContains(t, wire, "size")
			require.NotContains(t, wire, "n")
			require.NotContains(t, wire, "batch_size")
			require.NotContains(t, wire, "extra_body")
			require.Equal(t, "/v1/images/generations", observed["path"])
			require.Equal(t, "Bearer upstream-fixture-key", observed["auth"])
			require.Equal(t, "", observed["watermark"], "must not disable the provider watermark default")
			if tc.status >= 400 || tc.malformed || tc.disconnect {
				require.NotNil(t, apiErr)
				return
			}
			require.Nil(t, apiErr)
			var result struct {
				Data []struct {
					URL string `json:"url"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Len(t, result.Data, 1)
			require.Equal(t, "https://example.com/generated.png", result.Data[0].URL)
		})
	}
}

// TestProtocolAuditMediaAdmission rejects ambiguous or unrepresentable billed
// work before any provider call or physical quota change.
func TestProtocolAuditMediaAdmission(t *testing.T) {
	for _, tc := range []struct {
		channel             int
		actual, path, extra string
	}{
		{channeltype.Zhipu, "cogvideox-3", "/v1/videos", `"duration":5,"seconds":10`},
		{channeltype.Zhipu, "cogvideox-3", "/v1/videos", `"duration":3`},
		{channeltype.Zhipu, "vidu2-image", "/v1/videos", `"seconds":10`},
		{channeltype.Zhipu, "cogvideox-3", "/v1/videos", `"n":2`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"n":2`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"batch_size":2`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"size":"1024x1024","image_size":"2048x2048"`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"response_format":"b64_json"`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"extra_body":{"n":4}`},
		{channeltype.SiliconFlow, "Bytedance/Z-Image-Turbo", "/v1/images/generations", `"image_size":"999999x999999"`},
		{channeltype.SiliconFlow, "Tongyi-MAI/Z-Image-Turbo", "/v1/images/generations", `"num_images":2`},
		{channeltype.SiliconFlow, "Tongyi-MAI/Z-Image-Turbo", "/v1/images/generations", `"extra_body":{"num_outputs":2}`},
		{channeltype.SiliconFlow, "Tongyi-MAI/Z-Image-Turbo", "/v1/images/generations", `"image":"https://example.com/image.png"`},
		{channeltype.SiliconFlow, "Tongyi-MAI/Z-Image-Turbo", "/v1/images/generations", `"seed":9007199254740993`},
		{channeltype.SiliconFlow, "black-forest-labs/FLUX.2-pro", "/v1/images/generations", `"size":"1024x1024"`},
	} {
		t.Run(tc.actual+tc.extra, func(t *testing.T) {
			const balance = int64(1000000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer server.Close()
			c, _, _ := protocolContext(t, tc.channel, tc.actual, tc.path, `{"model":"alias","prompt":"A boat",`+tc.extra+`}`, server.URL, balance, 1, false, nil)
			if tc.channel == channeltype.Zhipu {
				require.NotNil(t, RelayVideoHelper(c))
			} else {
				require.NotNil(t, RelayImageHelper(c, relaymode.ImagesGenerations))
			}
			drainCriticalTasks(t)
			require.Zero(t, calls.Load())
			require.Equal(t, balance, reloadUserQuota(t))
		})
	}
}
