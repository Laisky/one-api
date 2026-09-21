package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	xaiadaptor "github.com/Laisky/one-api/relay/adaptor/xai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetImageRequestRecordsEditOperation verifies the non-serialized operation
// marker used by endpoint-specific image pricing.
func TestGetImageRequestRecordsEditOperation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		mode      int
		wantIsEdit bool
	}{
		{name: "generation", mode: relaymode.ImagesGenerations, wantIsEdit: false},
		{name: "edit", mode: relaymode.ImagesEdits, wantIsEdit: true},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(
				http.MethodPost,
				"/v1/images/"+tt.name,
				strings.NewReader(`{"model":"grok-imagine-image-2.0","prompt":"test","quality":"auto"}`),
			)
			ctx.Request.Header.Set("Content-Type", "application/json")

			request, err := getImageRequest(ctx, tt.mode)
			require.NoError(t, err)
			require.Equal(t, tt.wantIsEdit, request.IsEdit)
		})
	}
}

// TestGrokImagineImage20AutoQualityBillingByOperation verifies that xAI's auto
// quality resolves to low for generation and medium for image editing.
func TestGrokImagineImage20AutoQualityBillingByOperation(t *testing.T) {
	t.Parallel()

	cfg := xaiadaptor.ModelRatios["grok-imagine-image-2.0"].Image
	require.NotNil(t, cfg)

	tests := []struct {
		name    string
		quality string
		isEdit  bool
		want    float64
	}{
		{name: "generation auto uses low", quality: "auto", isEdit: false, want: 1.0},
		{name: "edit auto uses medium", quality: "auto", isEdit: true, want: 1.5},
		{name: "edit explicit low stays low", quality: "low", isEdit: true, want: 1.0},
		{name: "generation explicit medium stays medium", quality: "medium", isEdit: false, want: 1.5},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := &relaymodel.ImageRequest{
				Model:   "grok-imagine-image-2.0",
				Size:    "1024x1024",
				Quality: tt.quality,
				IsEdit:  tt.isEdit,
			}

			ratio, err := getImageCostRatio(request, cfg)
			require.NoError(t, err)
			require.InDelta(t, tt.want, ratio, 1e-12)
		})
	}
}
