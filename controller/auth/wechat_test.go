package auth

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestGetWeChatIdByCodeEscapesCode verifies a user-supplied code reaches the
// authenticated WeChat verifier as one query value and cannot add parameters.
func TestGetWeChatIdByCodeEscapesCode(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		query := r.URL.Query()
		if r.URL.Path != "/api/wechat/user" || len(query) != 1 || query.Get("code") != "abc&user=root#frag" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"success":true,"data":"escaped-wechat-id"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	oldAddress := config.WeChatServerAddress
	config.WeChatServerAddress = upstream.URL
	t.Cleanup(func() { config.WeChatServerAddress = oldAddress })

	wechatID, err := getWeChatIdByCode("abc&user=root#frag")
	require.NoError(t, err)
	require.Equal(t, "escaped-wechat-id", wechatID)
	require.Equal(t, int32(1), calls.Load())
}
