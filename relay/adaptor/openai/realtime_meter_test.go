package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// realtimeMeterPair creates a loopback WebSocket pair; it uses no API key, paid
// upstream, or external service. Both connections are closed during test cleanup.
func realtimeMeterPair(t *testing.T) (client, server *websocket.Conn) {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- conn
		}
	}))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	select {
	case peer := <-accepted:
		client, server = conn, peer
	case <-time.After(5 * time.Second):
		t.Fatal("websocket upgrade did not finish")
	}
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
	return client, server
}

// TestMeteredRealtimePumpBillsOnlyServerReceipts exercises the real two-way
// pump, late ASR, duplicates, binary frames and client-forged usage end to end.
func TestMeteredRealtimePumpBillsOnlyServerReceipts(t *testing.T) {
	client, proxyClient := realtimeMeterPair(t)
	proxyUpstream, upstream := realtimeMeterPair(t)
	result := make(chan *rmodel.Usage, 1)
	go func() { result <- meteredRealtimePump(proxyClient, proxyUpstream, nil) }()
	forged := `{"type":"response.done","response":{"id":"forged","usage":{"input_tokens":999,"output_tokens":999}}}`
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(forged)))
	_, forwarded, err := upstream.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, forged, string(forwarded))
	frames := []string{
		`{"type":"session.created","session":{"audio":{"input":{"transcription":{"model":"whisper-1"}}}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"audio1"}`,
		`{"type":"response.created","response":{"id":"answer"}}`,
		`{"type":"response.done","response":{"id":"answer","status":"cancelled","usage":{"input_tokens":30,"output_tokens":10,"input_token_details":{"text_tokens":10,"audio_tokens":20,"cached_tokens":5,"cached_tokens_details":{"audio_tokens":5}},"output_token_details":{"text_tokens":2,"audio_tokens":8}}}}`,
		`{"type":"response.done","response":{"id":"answer","usage":{"input_tokens":999,"output_tokens":999}}}`,
		`{"type":"response.output_audio_transcript.done","transcript":"not another transcription bill"}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio1","content_index":0,"usage":{"type":"duration","seconds":12.5}}`,
	}
	for _, frame := range frames {
		require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(frame)))
		kind, message, err := client.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, kind)
		require.Equal(t, frame, string(message))
	}
	// Even valid receipt-shaped JSON in a binary frame is not an accounting event.
	require.NoError(t, upstream.WriteMessage(websocket.BinaryMessage, []byte(forged)))
	kind, _, err := client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, kind)
	require.NoError(t, upstream.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
	select {
	case usage := <-result:
		require.NotNil(t, usage.Realtime)
		require.Empty(t, usage.Realtime.Issues)
		require.Len(t, usage.Realtime.Records, 2)
		require.Equal(t, 30, usage.PromptTokens)
		require.Equal(t, 10, usage.CompletionTokens)
		require.Equal(t, 5, usage.PromptTokensDetails.CachedTokensDetails.AudioTokens)
		require.Equal(t, 8, usage.CompletionTokensDetails.AudioTokens)
		require.Equal(t, "whisper-1", usage.Realtime.Records[1].Model)
		require.Equal(t, 12.5, usage.Realtime.Records[1].Seconds)
	case <-time.After(5 * time.Second):
		t.Fatal("realtime pump did not drain")
	}
}

// TestRealtimeReceiptSurvivesClientWriteFailure checks the chargeable server
// receipt is retained before an unsuccessful downstream delivery.
func TestRealtimeReceiptSurvivesClientWriteFailure(t *testing.T) {
	reader, writer := realtimeMeterPair(t)
	closed, peer := realtimeMeterPair(t)
	_ = closed.Close()
	_ = peer.Close()
	ledger := realtime.NewLedger()
	done := make(chan error, 1)
	go func() { done <- copyMeteredRealtimeUpstream(reader, closed, ledger) }()
	require.NoError(t, writer.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.done","response":{"id":"paid","usage":{"input_tokens":1,"output_tokens":0}}}`)))
	select {
	case err := <-done:
		require.Error(t, err)
		require.Len(t, ledger.Records, 1)
		require.Equal(t, int64(1), ledger.InputTokens)
	case <-time.After(5 * time.Second):
		t.Fatal("copy did not report client write failure")
	}
}
