package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyClaudeFixtureNamed returns the legacy fixture called name.
// Parameters: t owns assertions and name is the fixture name. Returns: it.
func legacyClaudeFixtureNamed(t *testing.T, name string) legacyClaudeFixture {
	t.Helper()
	for _, fx := range legacyClaudeFixtures() {
		if fx.name == name {
			return fx
		}
	}
	require.FailNow(t, "unknown legacy fixture", name)
	return legacyClaudeFixture{}
}

// requireClaudeStreamFailed asserts the client saw the streamed text and
// then a Claude error event, never a completed message. Parameters: t owns
// assertions and raw is the client SSE body.
func requireClaudeStreamFailed(t *testing.T, raw string) {
	t.Helper()
	events := parseClaudeSSE(t, raw)
	require.NotEmpty(t, events, "the client received nothing: %s", raw)
	require.Equal(t, "message_start", events[0].Event)
	require.Equal(t, "error", events[len(events)-1].Event, "an interrupted stream must end with a Claude error event: %s", raw)
	for _, event := range events {
		require.NotEqual(t, "message_stop", event.Event, "an interrupted stream must not complete: %s", raw)
		require.NotEqual(t, "message_delta", event.Event, "an interrupted stream must not report a stop reason: %s", raw)
	}
	require.Contains(t, raw, `"text":"ack"`, "content streamed before the interruption still reaches the client")
}

// TestLegacyClaudeStreamInterruptionContract drives Claude Messages streams
// whose provider connection fails or ends before the provider's own end
// event. These handlers log the failure and finalize the chat stream, so a
// Chat client sees [DONE]; a Claude client must instead get a Claude error
// event and the controller an error result, while the work already
// delivered stays billed exactly as the handler measured it.
func TestLegacyClaudeStreamInterruptionContract(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		// stream is the provider stream up to the interruption.
		stream    string
		truncate  bool
		charge    int64
		chatCheck func(t *testing.T, raw string)
	}{
		{
			// Baidu reports cumulative usage per chunk: 11 input and 2 output
			// tokens settle at 11*2 + 2*6.
			name: "baidu read error", fixture: "baidu", truncate: true, charge: 34,
			stream: "data: {\"id\":\"as-1\",\"result\":\"ack\",\"is_end\":false,\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}}\n\n",
		},
		{
			// Coze reports no usage; the adaptor counts the delivered text
			// against the 9-token prompt estimate: 9*2 + 1*6.
			name: "coze read error", fixture: "coze", truncate: true, charge: 24,
			stream: "data:{\"event\":\"message\",\"message\":{\"role\":\"assistant\",\"type\":\"answer\",\"content\":\"ack\",\"content_type\":\"text\"},\"is_finish\":false,\"index\":0,\"conversation_id\":\"c1\"}\n\n",
		},
		{
			// A clean EOF before Cohere's stream-end event.
			name: "cohere eof before stream-end", fixture: "cohere", truncate: false,
			stream: "{\"is_finished\":false,\"event_type\":\"text-generation\",\"text\":\"ack\"}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := legacyClaudeFixtureNamed(t, tc.fixture)
			native := fx.reply
			fx.reply = func(stream bool, r *http.Request) legacyWireReply {
				answer := native(stream, r)
				if r.URL.Path == fx.chatPath && stream {
					answer.Body, answer.Truncate = tc.stream, tc.truncate
				}
				return answer
			}

			claude := runLegacyClaudeFixture(t, fx, "messages", true)
			require.NotNil(t, claude.apiErr, "an interrupted stream must be an error result")
			require.Equal(t, "upstream_stream_incomplete", claude.apiErr.Error.Code)
			requireClaudeStreamFailed(t, claude.writer.Body.String())
			require.Positive(t, claude.charge, "delivered work stays billed")
			if tc.charge > 0 {
				require.Equal(t, tc.charge, claude.charge, "settlement keeps the usage the handler measured")
			}
			require.Equal(t, claude.charge, claude.consume, "the consume log must record the charge")
			require.Equal(t, claude.charge, claude.costQuota, "the request cost must reconcile to the charge")

			// Control: the Chat Completions output of the same interruption is unchanged.
			chat := runLegacyClaudeFixture(t, fx, "chat", true)
			require.Nil(t, chat.apiErr, "control: the chat handler keeps swallowing the interruption")
			require.Contains(t, chat.writer.Body.String(), "data: [DONE]", "control: Chat clients still get [DONE]")
		})
	}
}

// TestLegacyClaudeStreamJSONErrorBody answers a Claude Messages stream
// request with a 200 JSON error body, which parses into no stream chunk. The
// client must not receive an empty but well-formed message, the controller
// must get an error result, and the provider estimate must not be settled as
// a successful request.
func TestLegacyClaudeStreamJSONErrorBody(t *testing.T) {
	for _, tc := range []struct {
		fixture, body string
	}{
		{fixture: "baidu", body: `{"error_code":336003,"error_msg":"invalid argument"}`},
		{fixture: "coze", body: `{"code":4000,"msg":"bot not found"}`},
		{fixture: "aiproxy_library", body: `{"success":false,"errCode":1001,"message":"library not found"}`},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			fx := legacyClaudeFixtureNamed(t, tc.fixture)
			native := fx.reply
			fx.reply = func(stream bool, r *http.Request) legacyWireReply {
				answer := native(stream, r)
				if r.URL.Path == fx.chatPath && stream {
					answer = legacyWireReply{ContentType: "application/json", Body: tc.body}
				}
				return answer
			}
			claude := runLegacyClaudeFixture(t, fx, "messages", true)
			require.NotNil(t, claude.apiErr, "a JSON error body must not be reported as success")
			require.Equal(t, "upstream_stream_incomplete", claude.apiErr.Error.Code)
			raw := claude.writer.Body.String()
			require.NotContains(t, raw, "message_stop", "no fake completed message")
			require.NotContains(t, raw, "message_delta", "no fake stop reason")
			require.NotContains(t, raw, "content_block", "no fabricated content")
			if events := parseClaudeSSE(t, raw); len(events) > 0 {
				// AIProxy renders a synthetic closing chunk even for an error body,
				// so its Claude stream opens; it must then end in an error event.
				require.Equal(t, "error", events[len(events)-1].Event, "an opened stream must end with a Claude error event: %s", raw)
			}
			require.Zero(t, claude.consume, "no consume log may settle a failed request as success")
			t.Logf("charge=%d log=%d cost=%d", claude.charge, claude.logQuota, claude.costQuota)
		})
	}
}
