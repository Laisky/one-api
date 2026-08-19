package ali

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
)

// TestGetImageData_RefusesLoopback is the SSRF defense-in-depth regression
// test. The previous implementation called bare `http.Get` against URLs
// produced by Aliyun, which would happily reach any address the OS allows —
// including 127.0.0.1 and cloud metadata endpoints. With the hardened
// client wired in, the function MUST refuse to dial any forbidden IP, so an
// MITM-injected URL or a future caller that pipes a user URL through this
// helper cannot exfiltrate internal data.
//
// Reproduces the bug shape from gh #2387/#2388 even though those reports
// were filed against the Vision API path (which is already protected).
func TestGetImageData_RefusesLoopback(t *testing.T) {
	client.Init()

	// httptest binds to 127.0.0.1; the hardened client must refuse the dial.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("if you can read this, SSRF defense leaked"))
	}))
	defer server.Close()

	_, err := getImageData(server.URL + "/img.png")
	require.Error(t, err,
		"hardened client must refuse to fetch loopback URLs (SSRF guard)")
	// Don't assert on the error wording — Go's net stack and our dial
	// filter both wrap differently. Refusing the dial at all is the
	// contract this test pins down.
}

// TestGetImageData_RefusesRedirectToLoopback complements the loopback test:
// when a public host returns a redirect to a forbidden address, the
// CheckRedirect installed on UserContentRequestHTTPClient must reject it.
// Because httptest itself binds to loopback we cannot run this end-to-end
// without a non-loopback test endpoint; the test documents the contract
// and skips when the harness cannot reach a public host. The first-hop
// loopback test above is the one that runs unconditionally.
func TestGetImageData_RefusesRedirectToLoopback(t *testing.T) {
	t.Skip("documents contract; needs non-loopback test endpoint to exercise the redirect-revalidation branch")
}

// newClosedTestServerURL returns an http URL that is guaranteed to fail when
// dialed: a freshly allocated server is started and immediately closed, so any
// subsequent http.Get against the URL produces a connection error. Used to
// drive the b64-fetch failure branch in responseAli2OpenAIImage.
func newClosedTestServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	url := srv.URL
	srv.Close()
	return url + "/image.png"
}

// makeTaskResponseWithURLs assembles a TaskResponse whose Output.Results all
// reference the provided URL. Each entry uses an empty B64Image to ensure the
// b64_json branch must hit the fetch path.
func makeTaskResponseWithURLs(urls ...string) *TaskResponse {
	r := &TaskResponse{}
	for _, u := range urls {
		r.Output.Results = append(r.Output.Results, struct {
			B64Image string `json:"b64_image,omitempty"`
			Url      string `json:"url,omitempty"`
			Code     string `json:"code,omitempty"`
			Message  string `json:"message,omitempty"`
		}{Url: u})
	}
	return r
}

// TestResponseAli2OpenAIImage_AllResultsFailedEmitsDataArray ensures that when
// every upstream result fails the b64 fetch, the wire output still contains
// "data":[] and not null.
func TestResponseAli2OpenAIImage_AllResultsFailedEmitsDataArray(t *testing.T) {
	t.Parallel()
	deadURL := newClosedTestServerURL(t)
	resp := makeTaskResponseWithURLs(deadURL, deadURL, deadURL)

	out := responseAli2OpenAIImage(resp, "b64_json")
	require.NotNil(t, out.Data, "Data must be non-nil")
	require.Len(t, out.Data, 0)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"data":[]`)
	require.NotContains(t, string(raw), `"data":null`)
}

// TestResponseAli2OpenAIImage_NoResultsEmitsDataArray ensures empty/nil
// upstream results still serialize "data":[].
func TestResponseAli2OpenAIImage_NoResultsEmitsDataArray(t *testing.T) {
	t.Parallel()
	resp := &TaskResponse{}

	out := responseAli2OpenAIImage(resp, "")
	require.NotNil(t, out.Data, "Data must be non-nil")
	require.Len(t, out.Data, 0)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"data":[]`)
	require.NotContains(t, string(raw), `"data":null`)
}

// TestResponseAli2OpenAIImage_PopulatedRoundTrips sanity-checks normal
// multi-image responses.
func TestResponseAli2OpenAIImage_PopulatedRoundTrips(t *testing.T) {
	t.Parallel()

	r := &TaskResponse{}
	r.Output.Results = append(r.Output.Results, struct {
		B64Image string `json:"b64_image,omitempty"`
		Url      string `json:"url,omitempty"`
		Code     string `json:"code,omitempty"`
		Message  string `json:"message,omitempty"`
	}{B64Image: "AAAA", Url: "https://example.com/a.png"})
	r.Output.Results = append(r.Output.Results, struct {
		B64Image string `json:"b64_image,omitempty"`
		Url      string `json:"url,omitempty"`
		Code     string `json:"code,omitempty"`
		Message  string `json:"message,omitempty"`
	}{B64Image: "BBBB", Url: "https://example.com/b.png"})

	// Use the non-fetch (url) format so the test does not perform real network IO.
	out := responseAli2OpenAIImage(r, "url")
	require.Len(t, out.Data, 2)
	require.Equal(t, "AAAA", out.Data[0].B64Json)
	require.Equal(t, "https://example.com/a.png", out.Data[0].Url)
	require.Equal(t, "BBBB", out.Data[1].B64Json)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(raw), `"url":"https://example.com/a.png"`))
}
