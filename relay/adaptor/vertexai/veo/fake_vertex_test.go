package veo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
)

const (
	// fakeVertexToken is the bearer token the fake Vertex provider requires.
	fakeVertexToken = "test-vertex-token"
	// fakeCreateQuery is a query string carried by the create URL that the
	// derived poll URL must preserve.
	fakeCreateQuery = "trace=1"
)

// TestMain initializes the shared relay HTTP client once, before any parallel
// test resolves it, and runs the package tests. It accepts m and exits with
// the test status.
func TestMain(m *testing.M) {
	client.Init()
	os.Exit(m.Run())
}

// pollReply is one scripted fetchPredictOperation answer. hijack closes the
// connection after writing a truncated body instead of answering normally.
type pollReply struct {
	status int
	body   string
	hijack bool
}

// pollRecord captures what one fetchPredictOperation call carried.
type pollRecord struct {
	path          string
	rawQuery      string
	authorization string
	custom        string
	operationName string
}

// fakeVertex is a fake Vertex AI endpoint that serves predictLongRunning and
// scripted fetchPredictOperation replies, enforcing the bearer token on polls.
type fakeVertex struct {
	srv          *httptest.Server
	createStatus int
	createBody   string
	mu           sync.Mutex
	replies      []pollReply
	polls        []pollRecord
}

// newFakeVertex starts a fake provider whose create call answers with
// createStatus and createBody and whose polls answer with replies in order,
// repeating the last reply. It accepts t, the create status and body, and the
// poll replies, and returns the provider.
func newFakeVertex(t *testing.T, createStatus int, createBody string, replies ...pollReply) *fakeVertex {
	t.Helper()
	f := &fakeVertex{createStatus: createStatus, createBody: createBody, replies: replies}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// serve answers one fake provider request. It accepts the response writer and
// the request.
func (f *fakeVertex) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, actionPredictLongRunning):
		w.WriteHeader(f.createStatus)
		_, _ = io.WriteString(w, f.createBody)
	case strings.HasSuffix(r.URL.Path, actionFetchOperation):
		reply := f.recordPoll(r)
		if r.Header.Get("Authorization") != "Bearer "+fakeVertexToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"code":401,"message":"Request is missing required authentication credential.","status":"UNAUTHENTICATED"}}`)
			return
		}
		if reply.hijack {
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(reply.status)
			_, _ = io.WriteString(w, reply.body)
			w.(http.Flusher).Flush()
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// recordPoll stores what a poll carried and returns its scripted reply.
// It accepts the poll request and returns the reply to send.
func (f *fakeVertex) recordPoll(r *http.Request) pollReply {
	var body PollVideoTaskRequest
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls = append(f.polls, pollRecord{
		path:          r.URL.Path,
		rawQuery:      r.URL.RawQuery,
		authorization: r.Header.Get("Authorization"),
		custom:        r.Header.Get("X-Channel-Custom"),
		operationName: body.OperationName,
	})
	index := min(len(f.polls)-1, len(f.replies)-1)
	if index < 0 {
		return pollReply{status: http.StatusOK, body: `{"name":"` + veoTestOperation + `","done":false}`}
	}
	return f.replies[index]
}

// pollCount returns how many poll calls reached the provider.
func (f *fakeVertex) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.polls)
}

// pollRecords returns a copy of the recorded poll calls.
func (f *fakeVertex) pollRecords() []pollRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pollRecord(nil), f.polls...)
}

// createURL returns the predictLongRunning URL on the fake provider,
// including fakeCreateQuery.
func (f *fakeVertex) createURL() string {
	return f.srv.URL + veoTestModelPath + actionPredictLongRunning + "?" + fakeCreateQuery
}

// createLikeRelay sends the create call the way the relay does: an outbound
// client request carrying the Vertex bearer token and a channel custom header.
// The returned response's Request is a client request, so RequestURI is empty
// as in production. It accepts t and returns the live response.
func (f *fakeVertex) createLikeRelay(t *testing.T) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.createURL(),
		bytes.NewReader([]byte(`{"instances":[{"prompt":"a cat"}],"parameters":{"sampleCount":1}}`)))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+fakeVertexToken)
	req.Header.Set("X-Channel-Custom", "custom-value")
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Empty(t, resp.Request.RequestURI, "client-side requests never carry RequestURI")
	return resp
}

// pendingReply returns a poll reply for an operation that is still running.
func pendingReply() pollReply {
	return pollReply{status: http.StatusOK, body: `{"name":"` + veoTestOperation + `","done":false}`}
}

// doneReply returns a poll reply for a completed Vertex operation with one
// Cloud Storage video and one inline base64 video.
func doneReply() pollReply {
	return pollReply{status: http.StatusOK, body: `{"name":"` + veoTestOperation + `","done":true,"response":{` +
		`"@type":"type.googleapis.com/cloud.ai.large_models.vision.GenerateVideoResponse","raiMediaFilteredCount":0,` +
		`"videos":[{"gcsUri":"gs://bucket/sample_0.mp4","mimeType":"video/mp4"},{"bytesBase64Encoded":"AAAA","mimeType":"video/mp4"}]}}`}
}

// acceptedCreateBody is the predictLongRunning body naming the test operation.
const acceptedCreateBody = `{"name":"` + veoTestOperation + `"}`
