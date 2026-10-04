package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
)

// TestSecurityLoginRejectsInvalidWireUTF8 exercises the real login route and
// failure tracker before malformed bytes can be repaired by JSON decoding.
func TestSecurityLoginRejectsInvalidWireUTF8(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	config.PasswordLoginEnabled = true
	router := newLoginRouter()
	for _, chunked := range []bool{false, true} {
		for _, field := range []string{"username", "password", "padding"} {
			t.Run(field+map[bool]string{false: "/fixed", true: "/chunked"}[chunked], func(t *testing.T) {
				username := "security-wire-utf8-" + field
				fields := map[string]string{"username": username, "password": "wrong", "padding": "safe"}
				fields[field] = "security-wire-marker"
				body, err := json.Marshal(fields)
				require.NoError(t, err)
				body = bytes.Replace(body, []byte("security-wire-marker"), []byte{'b', 'a', 'd', 0xff}, 1)
				var repaired LoginRequest
				require.NoError(t, json.Unmarshal(body, &repaired), "the decoder's replacement behavior is the guarded baseline")
				middleware.ClearLoginFailure(repaired.Username)
				t.Cleanup(func() { middleware.ClearLoginFailure(repaired.Username) })
				req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if chunked {
					req.ContentLength = -1
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code, "preserve the dashboard error envelope")
				var reply map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
				require.Equal(t, false, reply["success"])
				retained := middleware.HasLoginFailure(repaired.Username)
				t.Logf("INVALID_WIRE field=%s chunked=%v retained=%v response=%s", field, chunked, retained, w.Body.String())
				require.Contains(t, reply["message"], "invalid parameter")
				require.False(t, retained, "invalid wire bytes must not reach failure tracking")
				require.Empty(t, w.Header().Values("Set-Cookie"))
			})
		}
	}
	// These controls distinguish valid Unicode from actual invalid wire bytes.
	for _, username := range []string{"security-valid-é", "security-valid-�"} {
		t.Run("valid_unicode/"+username, func(t *testing.T) {
			createLoginUser(t, username, "safe-fixture-password", model.RoleCommonUser)
			w := postLogin(t, router, username, "safe-fixture-password")
			var reply map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
			require.Equal(t, true, reply["success"], w.Body.String())
		})
	}
}
