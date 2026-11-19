package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestNewSessionStoreRoundTripsEverySecretShape proves a session written
// with the store can be read back for a raw secret, a base64 secret of a
// valid AES key size, and a base64 secret of any other decoded length. The
// last case used to fail every login with "crypto/aes: invalid key size".
func TestNewSessionStoreRoundTripsEverySecretShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, secret := range map[string]string{
		"raw":            "213",
		"aes256_base64":  base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		"48_byte_base64": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 48)),
		"20_byte_base64": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 20)),
	} {
		t.Run(name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(sessions.Sessions("session", newSessionStore(secret)))
			engine.GET("/set", func(c *gin.Context) {
				session := sessions.Default(c)
				session.Set("username", "root")
				if err := session.Save(); err != nil {
					c.String(http.StatusInternalServerError, err.Error())
					return
				}
				c.Status(http.StatusNoContent)
			})
			engine.GET("/get", func(c *gin.Context) {
				value, _ := sessions.Default(c).Get("username").(string)
				c.String(http.StatusOK, value)
			})

			set := httptest.NewRecorder()
			engine.ServeHTTP(set, httptest.NewRequest(http.MethodGet, "/set", nil))
			require.Equal(t, http.StatusNoContent, set.Code, set.Body.String())
			cookies := set.Result().Cookies()
			require.NotEmpty(t, cookies)

			get := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/get", nil)
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			engine.ServeHTTP(get, request)
			require.Equal(t, "root", get.Body.String())
		})
	}
}
