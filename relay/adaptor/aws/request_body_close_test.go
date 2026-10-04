package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	channelmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/meta"
	awsdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/require"
)

// requestCloseBarrier preserves the real SDK body's optional WriterTo method,
// but schedules the transport's final exhaustion check after Smithy's early
// request-body Close. It returns the SDK's original result without injecting errors.
type requestCloseBarrier struct {
	io.ReadCloser
	writer   io.WriterTo
	closed   chan struct{}
	once     sync.Once
	ctx      context.Context
	fastPath atomic.Bool
}

// WriteTo controls only the interleaving of the optional exhaustion fast path.
func (b *requestCloseBarrier) WriteTo(w io.Writer) (int64, error) {
	b.fastPath.Store(true)
	select {
	case <-b.closed:
		return b.writer.WriteTo(w)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

// Close forwards the original SDK close before releasing the delayed check.
func (b *requestCloseBarrier) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { close(b.closed) })
	return err
}

// requestCloseBarrierClient installs the barrier before the production client
// and observes the actual net/http writer result on one local SDK invocation.
type requestCloseBarrierClient struct {
	next    bedrockruntime.HTTPClient
	wrote   chan error
	barrier *requestCloseBarrier
}

// Do preserves request bytes and waits only at the optional final WriteTo path.
func (c *requestCloseBarrierClient) Do(req *http.Request) (*http.Response, error) {
	writer, ok := req.Body.(io.WriterTo)
	if !ok {
		return nil, io.ErrNoProgress
	}
	c.barrier = &requestCloseBarrier{ReadCloser: req.Body, writer: writer, closed: make(chan struct{}), ctx: req.Context()}
	req.Body = c.barrier
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		select {
		case c.wrote <- info.Err:
		default:
		}
	}}
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	return c.next.Do(req)
}

// requestCloseFrame encodes genuine SDK events without mocking its decoder.
func requestCloseFrame(t *testing.T, kind string, payload any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var headers eventstream.Headers
	headers.Set(":message-type", eventstream.StringValue("event"))
	headers.Set(":event-type", eventstream.StringValue(kind))
	headers.Set(":content-type", eventstream.StringValue("application/json"))
	var data bytes.Buffer
	require.NoError(t, eventstream.NewEncoder().Encode(&data, eventstream.Message{Headers: headers, Payload: raw}))
	return data.Bytes()
}

// TestSDKRequestCloseDoesNotAbortResponse schedules a valid early HTTP response
// before the transport's last request-body exhaustion check. The real SDK closes
// its request body when Do returns; this must not abort the paid response stream.
func TestSDKRequestCloseDoesNotAbortResponse(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wrote := make(chan error, 1)
	writerResult := make(chan error, 1)
	serverClosed := make(chan bool, 1)
	var calls atomic.Int32
	start := requestCloseFrame(t, "messageStart", map[string]any{"role": "assistant"})
	tail := requestCloseFrame(t, "messageStop", map[string]any{"stopReason": "end_turn"})
	tail = append(tail, requestCloseFrame(t, "metadata", map[string]any{"usage": map[string]any{"inputTokens": 11, "outputTokens": 7, "totalTokens": 18}})...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// A large request bypasses the small write buffer. All declared bytes
		// reach this real server before the final WriterTo exhaustion check.
		_, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 128<<10))
		if err != nil {
			writerResult <- err
			return
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		_, _ = w.Write(start)
		w.(http.Flusher).Flush()
		select {
		case err = <-wrote:
		case <-ctx.Done():
			writerResult <- ctx.Err()
			return
		}
		writerResult <- err
		if err != nil {
			select {
			case <-r.Context().Done():
				serverClosed <- true
			case <-ctx.Done():
				serverClosed <- false
			}
			return
		}
		_, _ = w.Write(tail)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL", server.URL)
	var adaptor Adaptor
	adaptor.Init(&meta.Meta{Config: channelmodel.ChannelConfig{Region: "us-east-1", AK: "synthetic-access", SK: "synthetic-secret"}})
	require.NotNil(t, adaptor.AwsClient)
	barrierClient := &requestCloseBarrierClient{next: adaptor.AwsClient.Options().HTTPClient, wrote: wrote}
	sdk := bedrockruntime.New(adaptor.AwsClient.Options(), func(o *bedrockruntime.Options) { o.HTTPClient = barrierClient })
	result, err := sdk.ConverseStream(ctx, &bedrockruntime.ConverseStreamInput{
		ModelId:  awsdk.String("qwen3-coder-480b"),
		Messages: []types.Message{{Role: types.ConversationRoleUser, Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: strings.Repeat("p", 64<<10)}}}},
	}, utils.NoInferenceRetry)
	require.NoError(t, err, "headers and initial SDK event are valid")
	stream := result.GetStream()
	defer stream.Close()
	var receipt *types.TokenUsage
	for ev := range stream.Events() {
		if metadata, ok := ev.(*types.ConverseStreamOutputMemberMetadata); ok {
			receipt = metadata.Value.Usage
		}
	}
	select {
	case err = <-writerResult:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	t.Logf("REQUEST_CLOSE_TRANSPORT fast_path=%v writer_error=%v stream_error=%v", barrierClient.barrier.fastPath.Load(), err, stream.Err())
	if err != nil {
		require.True(t, <-serverClosed, "the premature transport failure must physically close the provider connection")
	}
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, err, "closing a fully transmitted SDK body must not turn normal exhaustion into a transport error")
	require.NoError(t, stream.Err(), "request cleanup must preserve the response stream")
	require.NotNil(t, receipt)
	require.EqualValues(t, 18, *receipt.TotalTokens)
}
