package router

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
)

// e2eCall is one HTTP request the fake upstream received.
type e2eCall struct {
	// Credential is the channel key the gateway sent (Bearer token or api-key).
	Credential string
	Method     string
	Host       string
	Path       string
	Raw        []byte
	// Body is the decoded JSON request body, or nil when it is not JSON.
	Body map[string]any
	// UserQuota is the wallet balance observed while the request was in flight,
	// i.e. after admission and before settlement.
	UserQuota int64
}

// Stream reports whether the upstream request asked for SSE delivery.
func (c e2eCall) Stream() bool {
	stream, _ := c.Body["stream"].(bool)
	return stream
}

// e2eReply is the fake upstream's answer to one call.
type e2eReply struct {
	Status      int
	ContentType string
	Body        string
}

// e2eResponder scripts the fake upstream's answer to each call.
type e2eResponder func(call e2eCall) e2eReply

// e2eUpstream is a TLS fake provider that serves every host the gateway dials.
type e2eUpstream struct {
	server  *httptest.Server
	respond e2eResponder
	mu      sync.Mutex
	calls   []e2eCall
	errs    []error
}

// newE2EUpstream starts the fake provider and routes every production HTTP
// client dial to it whatever host the channel base URL names, so native
// https://api.openai.com channels keep their production routing and no test
// can reach the real network. Parameters: t owns cleanup and respond scripts
// the answers. Returns: the started upstream.
func newE2EUpstream(t *testing.T, respond e2eResponder) *e2eUpstream {
	t.Helper()
	upstream := &e2eUpstream{respond: respond}
	upstream.server = httptest.NewTLSServer(http.HandlerFunc(upstream.serve))
	t.Cleanup(upstream.server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.server.Certificate())
	address := upstream.server.Listener.Addr().String()
	client.HTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		// httptest certificates are issued for example.com and loopback only.
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12},
	}}
	t.Cleanup(func() {
		upstream.mu.Lock()
		defer upstream.mu.Unlock()
		require.Empty(t, upstream.errs, "the fake upstream must observe every call cleanly")
	})
	return upstream
}

// serve records one call and writes the scripted reply.
func (u *e2eUpstream) serve(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	call := e2eCall{Method: r.Method, Host: r.Host, Path: r.URL.Path, Raw: raw}
	if key := r.Header.Get("api-key"); key != "" {
		call.Credential = key
	} else {
		call.Credential = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &call.Body)
	}
	quotaErr := model.DB.Model(&model.User{}).Where("id = ?", e2eUserID).Select("quota").Scan(&call.UserQuota).Error
	u.mu.Lock()
	u.calls = append(u.calls, call)
	if err != nil {
		u.errs = append(u.errs, err)
	}
	if quotaErr != nil {
		u.errs = append(u.errs, quotaErr)
	}
	u.mu.Unlock()
	reply := u.respond(call)
	if reply.ContentType == "" {
		reply.ContentType = "application/json"
	}
	if reply.Status == 0 {
		reply.Status = http.StatusOK
	}
	w.Header().Set("Content-Type", reply.ContentType)
	w.WriteHeader(reply.Status)
	_, _ = io.WriteString(w, reply.Body)
}

// callsFor returns the calls attributed to credential, in arrival order.
func (u *e2eUpstream) callsFor(credential string) []e2eCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []e2eCall
	for _, call := range u.calls {
		if call.Credential == credential {
			out = append(out, call)
		}
	}
	return out
}

// total returns the number of calls received from every channel.
func (u *e2eUpstream) total() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.calls)
}

// e2eUsage is a provider receipt. Input includes Cached and CacheWrite (30m
// cache-write tokens), as OpenAI reports them.
type e2eUsage struct {
	Input, Output, Cached, CacheWrite int
}

// e2eUpstreamSurface classifies a provider path by its wire shape.
func e2eUpstreamSurface(path string) string {
	switch {
	case strings.HasSuffix(path, "/responses"):
		return "responses"
	case strings.HasSuffix(path, "/images/generations"):
		return "images"
	case strings.HasSuffix(path, "/api/library/ask"):
		return "library"
	default:
		return "chat"
	}
}

// e2eSuccess answers call with a successful receipt in the call's own wire
// shape and delivery mode. Parameters: call is the upstream request, modelName
// is echoed, tier is the reported Responses service_tier ("" omits it) and
// usage is the receipt. Returns: the reply.
func e2eSuccess(call e2eCall, modelName, tier string, usage e2eUsage) e2eReply {
	switch e2eUpstreamSurface(call.Path) {
	case "responses":
		if call.Stream() {
			return e2eReply{ContentType: "text/event-stream", Body: e2eResponsesSSE(modelName, tier, usage)}
		}
		return e2eReply{Body: e2eResponsesJSON(modelName, "completed", tier, usage, "")}
	case "library":
		return e2eReply{Body: `{"success":true,"answer":"ack","documents":[]}`}
	default:
		if call.Stream() {
			return e2eReply{ContentType: "text/event-stream", Body: e2eChatSSE(modelName, usage)}
		}
		return e2eReply{Body: e2eChatJSON(modelName, usage)}
	}
}

// e2eChatUsageJSON renders a Chat Completions usage object.
func e2eChatUsageJSON(u e2eUsage) string {
	return fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}`,
		u.Input, u.Output, u.Input+u.Output, u.Cached)
}

// e2eResponsesUsageJSON renders a Responses usage object.
func e2eResponsesUsageJSON(u e2eUsage) string {
	writes := ""
	if u.CacheWrite > 0 {
		writes = fmt.Sprintf(`,"cache_write_tokens":%d,"cache_write_tokens_details":{"ttl_30m_tokens":%d,"ttl_12h_tokens":0}`, u.CacheWrite, u.CacheWrite)
	}
	return fmt.Sprintf(`{"input_tokens":%d,"input_tokens_details":{"cached_tokens":%d%s},"output_tokens":%d,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":%d}`,
		u.Input, u.Cached, writes, u.Output, u.Input+u.Output)
}

// e2eChatJSON renders a non-streaming Chat Completions receipt answering "ack".
func e2eChatJSON(modelName string, u e2eUsage) string {
	return `{"id":"chatcmpl-e2e","object":"chat.completion","created":1767225600,"model":"` + modelName +
		`","choices":[{"index":0,"message":{"role":"assistant","content":"ack"},"finish_reason":"stop"}],"usage":` + e2eChatUsageJSON(u) + `}`
}

// e2eChatSSE renders a streaming Chat Completions receipt answering "ack" with
// the include_usage terminal chunk.
func e2eChatSSE(modelName string, u e2eUsage) string {
	head := `{"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1767225600,"model":"` + modelName + `",`
	return "data: " + head + `"choices":[{"index":0,"delta":{"role":"assistant","content":"ack"},"finish_reason":null}]}` + "\n\n" +
		"data: " + head + `"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		"data: " + head + `"choices":[],"usage":` + e2eChatUsageJSON(u) + "}\n\n" +
		"data: [DONE]\n\n"
}

// e2eResponsesJSON renders a Responses envelope. Parameters: status is the
// response status, tier the reported service_tier ("" omits it), u the receipt
// and errorJSON an error object ("" renders null). A completed envelope
// carries one "ack" message; any other status carries no output.
func e2eResponsesJSON(modelName, status, tier string, u e2eUsage, errorJSON string) string {
	output := `[{"type":"message","id":"msg_e2e","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}]`
	if status != "completed" {
		output = "[]"
	}
	if errorJSON == "" {
		errorJSON = "null"
	}
	tierField := ""
	if tier != "" {
		tierField = `"service_tier":"` + tier + `",`
	}
	return `{"id":"resp_e2e","object":"response","created_at":1767225600,"status":"` + status + `","model":"` + modelName + `",` + tierField +
		`"error":` + errorJSON + `,"output":` + output + `,"usage":` + e2eResponsesUsageJSON(u) + `}`
}

// e2eResponsesSSE renders a complete streaming Responses event sequence that
// answers "ack" and ends with response.completed carrying the receipt.
func e2eResponsesSSE(modelName, tier string, u e2eUsage) string {
	tierField := ""
	if tier != "" {
		tierField = `"service_tier":"` + tier + `",`
	}
	created := `{"id":"resp_e2e","object":"response","created_at":1767225600,"status":"in_progress","model":"` + modelName + `",` + tierField + `"output":[],"usage":null}`
	item := `{"type":"message","id":"msg_e2e","status":"in_progress","role":"assistant","content":[]}`
	doneItem := `{"type":"message","id":"msg_e2e","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}`
	events := []string{
		`{"type":"response.created","sequence_number":0,"response":` + created + `}`,
		`{"type":"response.in_progress","sequence_number":1,"response":` + created + `}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":` + item + `}`,
		`{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_e2e","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_e2e","output_index":0,"content_index":0,"delta":"ack"}`,
		`{"type":"response.output_text.done","sequence_number":5,"item_id":"msg_e2e","output_index":0,"content_index":0,"text":"ack"}`,
		`{"type":"response.content_part.done","sequence_number":6,"item_id":"msg_e2e","output_index":0,"content_index":0,"part":{"type":"output_text","text":"ack","annotations":[]}}`,
		`{"type":"response.output_item.done","sequence_number":7,"output_index":0,"item":` + doneItem + `}`,
		`{"type":"response.completed","sequence_number":8,"response":` + e2eResponsesJSON(modelName, "completed", tier, u, "") + `}`,
	}
	var b strings.Builder
	for _, event := range events {
		var probe struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(event), &probe)
		b.WriteString("event: " + probe.Type + "\ndata: " + event + "\n\n")
	}
	// Like OpenAI, the Responses stream carries no [DONE] sentinel: its
	// response.completed event ends it. Converted Chat clients must still
	// receive their protocol's [DONE] terminator from the gateway.
	return b.String()
}

// e2eErrorType returns the OpenAI error type and code for an HTTP status.
func e2eErrorType(status int) (string, string) {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error", "invalid_value"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded", "rate_limit_exceeded"
	default:
		return "server_error", "server_error"
	}
}

// e2eProviderError answers call with a non-2xx provider error in the call's
// wire shape. Parameters: status is the HTTP status, modelName is echoed and
// receipt, when non-nil, is reported as billable usage in the error body
// (root usage on Chat, a failed envelope on Responses). Returns: the reply.
func e2eProviderError(call e2eCall, status int, modelName string, receipt *e2eUsage) e2eReply {
	errType, code := e2eErrorType(status)
	errorJSON := `{"message":"e2e provider failure","type":"` + errType + `","code":"` + code + `"}`
	if receipt == nil {
		return e2eReply{Status: status, Body: `{"error":` + errorJSON + `}`}
	}
	if e2eUpstreamSurface(call.Path) == "responses" {
		return e2eReply{Status: status, Body: e2eResponsesJSON(modelName, "failed", "", *receipt, errorJSON)}
	}
	return e2eReply{Status: status, Body: `{"error":` + errorJSON + `,"usage":` + e2eChatUsageJSON(*receipt) + `}`}
}
