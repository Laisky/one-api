package router

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/channeltype"
)

// lastValueFormUpstream is an image provider whose form parser keeps the LAST
// value of a repeated field, as Starlette/FastAPI, Django and PHP do.
type lastValueFormUpstream struct {
	mu     sync.Mutex
	models []string
}

// newLastValueFormUpstream starts the provider and returns it with the server.
func newLastValueFormUpstream(t *testing.T) (*httptest.Server, *lastValueFormUpstream) {
	t.Helper()
	upstream := &lastValueFormUpstream{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		values := r.PostForm["model"]
		if len(values) == 0 {
			http.Error(w, "missing model", http.StatusBadRequest)
			return
		}
		upstream.mu.Lock()
		upstream.models = append(upstream.models, values[len(values)-1])
		upstream.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"created":1767225600,"data":[{"url":"https://images.example/cat.png"}]}`)
	}))
	t.Cleanup(server.Close)
	return server, upstream
}

// rendered returns the models the provider rendered, in request order.
func (u *lastValueFormUpstream) rendered() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.models...)
}

// TestSecurityImageFormRepeatedModelCannotSplitBilling drives
// POST /v1/images/generations with form bodies that name the model twice. The
// gateway binds the first value while a last-value provider renders the
// second, so a repeated (or re-spelled) scalar must be refused before dispatch
// and charge; a single-valued form is the control and still renders.
func TestSecurityImageFormRepeatedModelCannotSplitBilling(t *testing.T) {
	multipartBody := func(fields [][2]string) (string, string) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for _, field := range fields {
			require.NoError(t, w.WriteField(field[0], field[1]))
		}
		require.NoError(t, w.Close())
		return w.FormDataContentType(), buf.String()
	}
	const urlencoded = "application/x-www-form-urlencoded"
	repeatedType, repeatedBody := multipartBody([][2]string{{"model", "dall-e-2"}, {"prompt", "a cat"}, {"model", "dall-e-3"}})
	singleType, singleBody := multipartBody([][2]string{{"model", "dall-e-2"}, {"prompt", "a cat"}})
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		accepted    bool
	}{
		{name: "urlencoded_repeated", contentType: urlencoded, body: "model=dall-e-2&prompt=a+cat&model=dall-e-3"},
		{name: "urlencoded_case_variant", contentType: urlencoded, body: "model=dall-e-2&prompt=a+cat&Model=dall-e-3"},
		{name: "urlencoded_bracket_variant", contentType: urlencoded, body: "model=dall-e-2&prompt=a+cat&model%5B%5D=dall-e-3"},
		{name: "multipart_repeated", contentType: repeatedType, body: repeatedBody},
		{name: "urlencoded_single_control", contentType: urlencoded, body: "model=dall-e-2&prompt=a+cat", accepted: true},
		{name: "multipart_single_control", contentType: singleType, body: singleBody, accepted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, provider := newLastValueFormUpstream(t)
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, "dall-e-2,dall-e-3", true)
			client.HTTPClient = upstream.Client()
			before, _ := passthroughKeysLedger(t)
			w := relayAs(t, engine, key, "/v1/images/generations", "", tc.contentType, tc.body)
			after, billed := passthroughKeysLedger(t)
			if tc.accepted {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Equal(t, []string{"dall-e-2"}, provider.rendered())
				require.Equal(t, []string{"dall-e-2"}, billed)
				require.Positive(t, before-after)
				return
			}
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.True(t, strings.Contains(w.Body.String(), "ambiguous form request parameter"), w.Body.String())
			require.Empty(t, provider.rendered(), "an ambiguous form must not reach the provider")
			require.Zero(t, before-after)
			require.Empty(t, billed)
		})
	}
}
