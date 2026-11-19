package controller

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// megapixelTestTariff mirrors DeepInfra's FLUX.2 [max] tariff.
// Returns: an image config that opts into megapixel billing.
func megapixelTestTariff() *relayadaptor.ImagePricingConfig {
	return &relayadaptor.ImagePricingConfig{
		PricePerImageUsd: 0.07,
		DefaultSize:      "1024x1024",
		Megapixel: &relayadaptor.ImageMegapixelPricing{
			FirstMegapixelUsd:           0.07,
			AdditionalMegapixelUsd:      0.03,
			ReferenceMegapixelUsd:       0.03,
			ReferenceFallbackMegapixels: 16,
		},
	}
}

// encodeTestImage encodes a blank image. Parameters: t owns assertions, format
// is png, jpeg, or gif, and width/height are its size. Returns: the bytes.
func encodeTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	img := image.NewGray(image.Rect(0, 0, width, height))
	switch format {
	case "png":
		require.NoError(t, png.Encode(&out, img))
	case "jpeg":
		require.NoError(t, jpeg.Encode(&out, img, nil))
	case "gif":
		require.NoError(t, gif.Encode(&out, img, nil))
	default:
		t.Fatalf("unsupported test format %s", format)
	}
	return out.Bytes()
}

// testMultipartPart is one part of a test multipart body.
type testMultipartPart struct {
	name, filename string
	content        []byte
}

// buildTestMultipart renders parts as a multipart body. Parameters: t owns
// assertions and parts are written in order. Returns: body and boundary.
func buildTestMultipart(t *testing.T, parts ...testMultipartPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.filename == "" {
			require.NoError(t, writer.WriteField(part.name, string(part.content)))
			continue
		}
		w, err := writer.CreateFormFile(part.name, part.filename)
		require.NoError(t, err)
		_, err = w.Write(part.content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.Boundary()
}

// TestMeasureMultipartReferenceImages verifies every uploaded file is measured
// from its decoded header, text image fields count as unmeasurable references,
// and ordinary form fields are ignored.
func TestMeasureMultipartReferenceImages(t *testing.T) {
	t.Parallel()
	body, boundary := buildTestMultipart(t,
		testMultipartPart{name: "model", content: []byte("black-forest-labs/FLUX-2-max")},
		testMultipartPart{name: "prompt", content: []byte("add a hat")},
		testMultipartPart{name: "image_prompt", content: []byte("not an image")},
		testMultipartPart{name: "image", filename: "a.png", content: encodeTestImage(t, "png", 1500, 1000)},
		testMultipartPart{name: "image[]", filename: "b.jpg", content: encodeTestImage(t, "jpeg", 640, 480)},
		testMultipartPart{name: "image[]", filename: "c.gif", content: encodeTestImage(t, "gif", 32, 16)},
		testMultipartPart{name: "mask", filename: "mask.png", content: encodeTestImage(t, "png", 256, 256)},
		testMultipartPart{name: "image", filename: "d.heic", content: []byte("opaque bytes")},
		testMultipartPart{name: "images", content: []byte("https://example.com/ref.png")},
		testMultipartPart{name: "image", content: []byte("   ")},
	)

	references, err := measureMultipartReferenceImages(body, boundary)
	require.NoError(t, err)
	require.Equal(t, []pricing.ImageDimensions{
		{Width: 1500, Height: 1000, Known: true},
		{Width: 640, Height: 480, Known: true},
		{Width: 32, Height: 16, Known: true},
		{Width: 256, Height: 256, Known: true},
		{},
		{},
	}, references)
}

// TestMeasureMultipartReferenceImagesRejectsAbuse bounds the reference count
// and refuses a body that is not multipart, or whose closing boundary was cut
// off, instead of pricing it as reference-free.
func TestMeasureMultipartReferenceImagesRejectsAbuse(t *testing.T) {
	t.Parallel()
	small := encodeTestImage(t, "png", 8, 8)
	parts := make([]testMultipartPart, 0, maxImageMegapixelReferences+1)
	for i := range maxImageMegapixelReferences + 1 {
		parts = append(parts, testMultipartPart{name: "image[]", filename: fmt.Sprintf("%d.png", i), content: small})
	}
	body, boundary := buildTestMultipart(t, parts...)
	_, err := measureMultipartReferenceImages(body, boundary)
	require.ErrorContains(t, err, "too many reference images")

	_, err = measureMultipartReferenceImages([]byte("not a multipart body"), boundary)
	require.Error(t, err, "a body without parts must not be priced as reference-free")

	body, boundary = buildTestMultipart(t, testMultipartPart{name: "image", filename: "a.png", content: small})
	_, err = measureMultipartReferenceImages(body[:len(body)-10], boundary)
	require.Error(t, err, "a truncated multipart body must not be priced")
	references, err := measureMultipartReferenceImages(body, boundary)
	require.NoError(t, err)
	require.Equal(t, []pricing.ImageDimensions{{Width: 8, Height: 8, Known: true}}, references)
}

// TestParseImagePixelSize covers the accepted WIDTHxHEIGHT spellings.
func TestParseImagePixelSize(t *testing.T) {
	t.Parallel()
	for size, want := range map[string][2]int{
		"1440x1440":    {1440, 1440},
		"1440X1024":    {1440, 1024},
		" 1024 × 768 ": {1024, 768},
		"512*512":      {512, 512},
	} {
		width, height, ok := parseImagePixelSize(size)
		require.True(t, ok, size)
		require.Equal(t, want, [2]int{width, height}, size)
	}
	for _, size := range []string{"", "auto", "1024", "x1024", "0x1024", "-1x1024", "10x10x10", "1kx1k"} {
		_, _, ok := parseImagePixelSize(size)
		require.False(t, ok, size)
	}
}

// newMegapixelTestContext builds a request context. Parameters: contentType is
// the client Content-Type. Returns: the gin context.
func newMegapixelTestContext(contentType string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", contentType)
	return c
}

// TestQuoteImageMegapixelChargeScope proves the quote is opt-in: models
// without a megapixel tariff are untouched, including JSON edits, while
// opted-in models price edits from the multipart upload.
func TestQuoteImageMegapixelChargeScope(t *testing.T) {
	t.Parallel()
	jsonEdit := &relaymodel.ImageRequest{Model: "m", Prompt: "p", Size: "2048x2048", IsEdit: true}
	for _, cfg := range []*relayadaptor.ImagePricingConfig{nil, {PricePerImageUsd: 0.04, DefaultSize: "1024x1024"}} {
		charge, bizErr := quoteImageMegapixelCharge(newMegapixelTestContext("application/json"), []byte(`{}`), jsonEdit, cfg)
		require.Nil(t, bizErr)
		require.Nil(t, charge, "a flat tariff must keep its existing billing")
	}

	charge, bizErr := quoteImageMegapixelCharge(newMegapixelTestContext("application/json"), []byte(`{}`), jsonEdit, megapixelTestTariff())
	require.Nil(t, charge)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)

	broken := megapixelTestTariff()
	broken.Megapixel.FirstMegapixelUsd = 0
	_, bizErr = quoteImageMegapixelCharge(newMegapixelTestContext("application/json"), []byte(`{}`), &relaymodel.ImageRequest{Size: "1024x1024"}, broken)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusServiceUnavailable, bizErr.StatusCode)

	body, boundary := buildTestMultipart(t,
		testMultipartPart{name: "prompt", content: []byte("p")},
		testMultipartPart{name: "image", filename: "a.png", content: encodeTestImage(t, "png", 1500, 1000)},
		testMultipartPart{name: "image", filename: "b.bin", content: []byte("??")},
	)
	edit := &relaymodel.ImageRequest{Model: "m", Prompt: "p", Size: "1440x1440", IsEdit: true}
	charge, bizErr = quoteImageMegapixelCharge(newMegapixelTestContext("multipart/form-data; boundary="+boundary), body, edit, megapixelTestTariff())
	require.Nil(t, bizErr)
	require.NotNil(t, charge)
	// $0.07 + $0.03 (2 MP output) + 2 MP x $0.03 + 16 MP fallback x $0.03.
	require.Equal(t, 0.64, charge.Quote.UnitUsd)
	require.Equal(t, imageMegapixelReferenceFallbackReason, charge.EstimateReason)
	require.Equal(t, " output_mp=2 reference_mp=18 references=2 references_estimated=1", formatImageMegapixelDetail(charge))
	require.Empty(t, formatImageMegapixelDetail(nil))
}
