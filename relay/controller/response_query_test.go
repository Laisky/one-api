package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	metalib "github.com/songquanpeng/one-api/relay/meta"
)

func TestApplyResponseAPIStreamParams(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		url     string
		want    bool
		wantErr bool
	}{
		{
			name: "default without stream",
			url:  "/v1/responses/resp_123",
			want: false,
		},
		{
			name: "stream true",
			url:  "/v1/responses/resp_123?stream=true",
			want: true,
		},
		{
			name: "stream false",
			url:  "/v1/responses/resp_123?stream=false",
			want: false,
		},
		{
			name:    "invalid stream",
			url:     "/v1/responses/resp_123?stream=foo",
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			c.Request = req

			meta := &metalib.Meta{}
			err := applyResponseAPIStreamParams(c, meta)
			if tt.wantErr {
				require.Error(t, err, "expected error but got nil")
				return
			}
			require.NoError(t, err, "unexpected error")
			require.Equal(t, tt.want, meta.IsStream, "unexpected IsStream value")
		})
	}
}
