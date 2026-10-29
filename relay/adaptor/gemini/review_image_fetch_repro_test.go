package gemini

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type reviewNotFoundTransport struct{}

func (reviewNotFoundTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("missing")), Request: r}, nil
}

// TestNanoImageInputFetchFailureStopsConversion verifies malformed input cannot become empty inline data.
func TestNanoImageInputFetchFailureStopsConversion(t *testing.T) {
	old := client.UserContentRequestHTTPClient
	client.UserContentRequestHTTPClient = &http.Client{Transport: reviewNotFoundTransport{}}
	t.Cleanup(func() { client.UserContentRequestHTTPClient = old })
	for _, tc := range []struct {
		name, url string
		valid     bool
	}{
		{"invalid_data_url", "data:invalid", false},
		{"invalid_base64", "data:image/png;base64,%%%", false},
		{"missing_http_image", "https://8.8.8.8/missing.png", false},
		{"valid_data_url", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", Messages: []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: tc.url}}}}}}
			got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, got)
				return
			}
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

// TestNanoImageAliasAndLegacyCompatibility checks mapped model admission and the existing legacy converter.
func TestNanoImageAliasAndLegacyCompatibility(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
	req := &model.GeneralOpenAIRequest{Model: "alias", Messages: []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: "data:invalid"}}}}}}
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.Error(t, err)
	require.Nil(t, got)
	legacy := &model.GeneralOpenAIRequest{Model: "gemini-2.5-flash", Messages: req.Messages}
	legacyContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err = (&Adaptor{}).ConvertRequest(legacyContext, relaymode.ChatCompletions, legacy)
	require.NoError(t, err)
	require.NotNil(t, got)
}

type reviewOctetImageTransport struct{ data []byte }

func (w reviewOctetImageTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/octet-stream"}}, Body: io.NopCloser(bytes.NewReader(w.data)), Request: r}, nil
}

// TestNanoImageBytesAndOctetStream validates real images and preserves octet-stream fetches.
func TestNanoImageBytesAndOctetStream(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	require.NoError(t, err)
	old := client.UserContentRequestHTTPClient
	client.UserContentRequestHTTPClient = &http.Client{Transport: reviewOctetImageTransport{data: png}}
	t.Cleanup(func() { client.UserContentRequestHTTPClient = old })
	for _, tc := range []struct {
		name, url string
		valid     bool
	}{
		{"real_png_octet_stream", "https://8.8.8.8/valid.png", true},
		{"base64_text_mislabeled_png", "data:image/png;base64,YWJj", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", Messages: []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: tc.url}}}}}}
			got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "image/png", got.(*ChatRequest).Contents[0].Parts[0].InlineData.MimeType)
		})
	}
}
