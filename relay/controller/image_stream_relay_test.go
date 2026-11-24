package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// strictImageStreamBalance is the starting user and token quota of the
// streamed image cases.
const strictImageStreamBalance = int64(10_000_000)

// strictImageStreamUsage is a complete GPT Image 2.5 receipt: 50 text input
// tokens at $5/M and 1,756 output tokens (one 1024x1024 high image) at $30/M.
const strictImageStreamUsage = `{"input_tokens":50,"output_tokens":1756,"total_tokens":1806,"input_tokens_details":{"text_tokens":50,"image_tokens":0}}`

// imageStreamFrames renders a provider event stream: partials partial-image
// events, then one final-image event per usage, using the generation or edit
// event names. Parameters: edit selects image_edit.* names, partials the
// partial-image count, and usages the final events' usage JSON. Returns: the
// SSE body.
func imageStreamFrames(edit bool, partials int, usages ...string) string {
	prefix := "image_generation"
	if edit {
		prefix = "image_edit"
	}
	var b strings.Builder
	for i := range partials {
		fmt.Fprintf(&b, "event: %s.partial_image\ndata: {\"type\":\"%s.partial_image\",\"b64_json\":\"AQ==\",\"partial_image_index\":%d}\n\n", prefix, prefix, i)
	}
	for _, usage := range usages {
		fmt.Fprintf(&b, "event: %s.completed\ndata: {\"type\":\"%s.completed\",\"b64_json\":\"AQID\",\"size\":\"1024x1024\",\"quality\":\"high\",\"output_format\":\"png\",\"usage\":%s}\n\n", prefix, prefix, usage)
	}
	return b.String()
}

// streamImageResponse returns a provider responder that answers with body as
// a 200 event stream and, when reset is set, then drops the connection without
// finishing the response. Parameters: body is the SSE payload and reset
// whether the transport fails afterwards. Returns: the responder.
func streamImageResponse(body string, reset bool) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
		w.(http.Flusher).Flush()
		if !reset {
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
}

// TestStrictGPTImageStreamedResponses drives stream=true GPT Image 2.5
// requests through RelayImageHelper, the SQLite ledger, and a provider that
// answers with an event stream. The client must receive the provider's events
// as an event stream, the native re-encoded body must keep stream and
// partial_images, and settlement must follow the streamed receipts: a
// complete stream settles its exact token price plus 100 output tokens per
// requested partial image, several receipts are summed and flagged, and a
// stream that may omit paid work (interrupted, failed, or short of receipts)
// never settles below the hold. Parameters: t runs the cases. Returns: none.
func TestStrictGPTImageStreamedResponses(t *testing.T) {
	const prompt = "A lighthouse"
	genBody := func(extra string) []byte {
		return []byte(`{"model":"gpt-image-2.5-sunburst","prompt":"` + prompt + `","size":"1024x1024","quality":"high","stream":true` + extra + `}`)
	}
	// The derived default holds the calculator's 1,756 output tokens plus 100
	// per requested partial image, and the prompt bytes as text input.
	holdWithPartials := func(partials, count int) int64 {
		return expectedDefaultImageHold(1756+100*partials, 30, 5, len(prompt), count)
	}
	// One complete receipt plus partial images: 50 x $5 + (1,756 + partial
	// tokens) x $30, at 500,000 quota per USD.
	charge := func(partials int) int64 {
		return int64(float64(50)*5/1e6*500_000 + float64(1756+100*partials)*30/1e6*500_000)
	}

	t.Run("native_generation_streams_and_settles_exactly", func(t *testing.T) {
		stream := imageStreamFrames(false, 2, strictImageStreamUsage)
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-stream-native", model: "gpt-image-2.5-sunburst",
			path: "/v1/images/generations", contentType: "application/json", body: genBody(`,"partial_images":2`),
			balance: strictImageStreamBalance, respond: streamImageResponse(stream, false),
		})
		var sent map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(res.sentBody, &sent))
		require.JSONEq(t, `true`, string(sent["stream"]), "the re-encoded body keeps stream")
		require.JSONEq(t, `2`, string(sent["partial_images"]), "the re-encoded body keeps partial_images")
		require.Equal(t, http.StatusOK, res.clientStatus)
		require.Equal(t, "text/event-stream", res.clientContentType)
		require.Equal(t, stream, string(res.clientBody), "the client receives the provider's events")
		requireStrictImageSettled(t, res, strictImageStreamBalance, holdWithPartials(2, 1), charge(2), "provider_tokens")
		require.NotContains(t, res.consume.Metadata, "billing_estimated", "an exact receipt is not an estimate")
	})

	t.Run("verbatim_generation_streams_and_settles_exactly", func(t *testing.T) {
		stream := imageStreamFrames(false, 1, strictImageStreamUsage)
		body := genBody(`,"partial_images":1`)
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-stream-verbatim", channelType: channeltype.OpenAICompatible, model: "gpt-image-2.5-sunburst",
			path: "/v1/images/generations", contentType: "application/json", body: body,
			balance: strictImageStreamBalance, respond: streamImageResponse(stream, false),
		})
		require.Equal(t, body, res.sentBody, "a verbatim body reaches the provider unchanged")
		require.Equal(t, stream, string(res.clientBody))
		requireStrictImageSettled(t, res, strictImageStreamBalance, holdWithPartials(1, 1), charge(1), "provider_tokens")
	})

	t.Run("edit_stream_settles_exactly", func(t *testing.T) {
		body, contentType := strictImageMultipartEdit(t, "gpt-image-2.5-sunburst", 1, 1, 64, false)
		body = []byte(strings.Replace(string(body), "Content-Disposition: form-data; name=\"prompt\"",
			"Content-Disposition: form-data; name=\"stream\"\r\n\r\ntrue\r\n--"+testMultipartBoundary(contentType)+"\r\n"+
				"Content-Disposition: form-data; name=\"partial_images\"\r\n\r\n1\r\n--"+testMultipartBoundary(contentType)+"\r\n"+
				"Content-Disposition: form-data; name=\"prompt\"", 1))
		stream := imageStreamFrames(true, 1, strictImageStreamUsage)
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-stream-edit", model: "gpt-image-2.5-sunburst", path: "/v1/images/edits",
			contentType: contentType, body: body, configs: `{"gpt-image-2.5-sunburst":{"image":{"price_per_image_usd":0.2}}}`,
			balance: strictImageStreamBalance, respond: streamImageResponse(stream, false),
		})
		require.Equal(t, body, res.sentBody, "a native edit is forwarded verbatim")
		require.Equal(t, stream, string(res.clientBody))
		requireStrictImageSettled(t, res, strictImageStreamBalance, 100_000, charge(1), "provider_tokens")
	})

	t.Run("several_receipts_are_summed_and_flagged", func(t *testing.T) {
		stream := imageStreamFrames(false, 0, strictImageStreamUsage, strictImageStreamUsage)
		res := runStrictImageRelay(t, strictImageRelayCase{
			requestID: "strict-image-stream-summed", channelType: channeltype.OpenAICompatible, model: "gpt-image-2.5-sunburst",
			path: "/v1/images/generations", contentType: "application/json", body: genBody(`,"n":2`),
			balance: strictImageStreamBalance, respond: streamImageResponse(stream, false),
		})
		requireStrictImageSettled(t, res, strictImageStreamBalance, holdWithPartials(0, 2), 2*charge(0), "provider_tokens_upper_bound")
		require.Equal(t, imageStreamReceiptsSummedReason, res.consume.Metadata["billing_estimate_reason"])
	})

	for _, tc := range []struct {
		name, stream, extra, source string
		reset                       bool
		partials, count             int
		chargeFn                    func(hold int64) int64
	}{
		{
			// No final image arrived: the hold settles.
			name: "partials_only_keep_the_hold", stream: imageStreamFrames(false, 2), extra: `,"partial_images":2`,
			partials: 2, count: 1, source: "default_fallback", chargeFn: func(hold int64) int64 { return hold },
		},
		{
			// An error event voids the receipt: the hold settles.
			name: "error_event_keeps_the_hold", partials: 1, count: 1, extra: `,"partial_images":1`, source: "default_fallback",
			stream:   imageStreamFrames(false, 1) + "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"server_error\"}}\n\n",
			chargeFn: func(hold int64) int64 { return hold },
		},
		{
			// A receipt arrived but the transport then failed: never below the
			// hold, and at least the receipt's own ceiling (every input token
			// at the $8 image rate): 50 x $8 + 1,856 x $30.
			name: "transport_failure_after_receipt_charges_the_ceiling", reset: true, partials: 1, count: 1, extra: `,"partial_images":1`,
			stream: imageStreamFrames(false, 1, strictImageStreamUsage), source: "provider_tokens_upper_bound",
			chargeFn: func(hold int64) int64 {
				return max(hold, int64(float64(50)*8/1e6*500_000+float64(1856)*30/1e6*500_000))
			},
		},
		{
			// Two images requested, one receipt: the receipt may cover only
			// one image, so the two-image hold settles.
			name: "missing_receipt_for_a_requested_image_keeps_the_hold", count: 2, extra: `,"n":2`,
			stream: imageStreamFrames(false, 0, strictImageStreamUsage), source: "default_fallback",
			chargeFn: func(hold int64) int64 { return hold },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "strict-image-stream-" + tc.name, channelType: channeltype.OpenAICompatible, model: "gpt-image-2.5-sunburst",
				path: "/v1/images/generations", contentType: "application/json", body: genBody(tc.extra),
				balance: strictImageStreamBalance, respond: streamImageResponse(tc.stream, tc.reset),
			})
			hold := holdWithPartials(tc.partials, tc.count)
			require.Equal(t, http.StatusOK, res.clientStatus, "the stream already reached the client")
			require.True(t, strings.HasPrefix(string(res.clientBody), tc.stream[:20]), "the provider's events reach the client")
			if tc.reset {
				// The relay reports the interrupted stream; billing still settles.
				require.NotNil(t, res.err)
				res.err = nil
			}
			requireStrictImageSettled(t, res, strictImageStreamBalance, hold, tc.chargeFn(hold), tc.source)
			require.Equal(t, true, res.consume.Metadata["billing_estimated"], "a charge without a complete stream receipt is flagged")
		})
	}
}

// testMultipartBoundary returns the boundary parameter of a multipart
// Content-Type. Parameters: contentType is the header value. Returns: the
// boundary.
func testMultipartBoundary(contentType string) string {
	_, boundary, _ := strings.Cut(contentType, "boundary=")
	return boundary
}

// TestLegacyGPTImageStreamedResponses covers a stream=true request for a GPT
// Image model that keeps additive per-image billing (gpt-image-2): the
// streamed receipt plus its partial image is added to the per-image charge,
// and an interrupted stream keeps that charge but is flagged as an estimate.
// Parameters: t runs the cases. Returns: none.
func TestLegacyGPTImageStreamedResponses(t *testing.T) {
	// gpt-image-2 high 1024x1024 reserves $0.211; the receipt adds 50 x $5 +
	// (1,756 + 100 partial) x $30 = $0.05593.
	const hold, charge = int64(105_500), int64(105_500 + 27_965)
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("reset_%v", reset), func(t *testing.T) {
			stream := imageStreamFrames(false, 1, strictImageStreamUsage)
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: fmt.Sprintf("legacy-image-stream-%v", reset), channelType: channeltype.OpenAICompatible, model: "gpt-image-2",
				path: "/v1/images/generations", contentType: "application/json",
				body:    []byte(`{"model":"gpt-image-2","prompt":"A lighthouse","size":"1024x1024","quality":"high","stream":true,"partial_images":1}`),
				balance: strictImageStreamBalance, respond: streamImageResponse(stream, reset),
			})
			require.Equal(t, stream, string(res.clientBody))
			require.Equal(t, strictImageStreamBalance-hold, res.userAtUpstream)
			require.Equal(t, strictImageStreamBalance-charge, res.userQuota)
			require.Equal(t, strictImageStreamBalance-charge, res.tokenQuota)
			require.NotNil(t, res.consume)
			require.Equal(t, int(charge), res.consume.Quota)
			if !reset {
				require.Nil(t, res.err)
				require.NotContains(t, res.consume.Metadata, "billing_estimated")
				return
			}
			require.NotNil(t, res.err, "the relay reports the interrupted stream")
			require.Equal(t, imageStreamReceiptIncompleteReason, res.consume.Metadata["billing_estimate_reason"])
		})
	}
}
