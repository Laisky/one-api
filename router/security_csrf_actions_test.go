package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityCSRFActionMethods rejects navigation to every migrated account and
// channel action, retaining the signed session and original persisted values.
func TestSecurityCSRFActionMethods(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	for _, path := range []string{
		"/api/user/token", "/api/user/aff", "/api/user/totp/setup", "/api/user/logout",
		"/api/oauth/email/bind?email=attacker%40example.test&code=123456",
		"/api/oauth/wechat/bind?code=123456",
		"/api/channel/test", "/api/channel/test/018f0000-0000-7000-8000-000000000972",
		"/api/channel/update_balance", "/api/channel/update_balance/018f0000-0000-7000-8000-000000000972",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+path, func(t *testing.T) {
				w := csrfRequest(engine, saved, method, path, "", "", "cross-site")
				require.NotContains(t, w.Body.String(), `"success":true`)
				require.Empty(t, w.Result().Cookies(), "navigation changed the signed session")
				var user model.User
				require.NoError(t, db.First(&user, 971).Error)
				require.Equal(t, "fixture-access-token", user.AccessToken)
				require.Empty(t, user.AffCode)
				require.Empty(t, user.Email)
				require.Empty(t, user.WeChatId)
			})
		}
	}
}

// TestSecurityCSRFAuthorizedAccountActions verifies trusted cookie and bearer
// requests still execute account actions after their POST migration.
func TestSecurityCSRFAuthorizedAccountActions(t *testing.T) {
	for _, bearer := range []bool{false, true} {
		db, engine, saved := csrfFixture(t)
		if bearer {
			saved = nil
		}
		w := csrfRequest(engine, saved, http.MethodPost, "/api/user/token", "", "https://gateway.test", "same-origin")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"success":true`)
		var user model.User
		require.NoError(t, db.First(&user, 971).Error)
		require.NotEqual(t, "fixture-access-token", user.AccessToken)
		// Restore the fixture bearer so the second authenticated action exercises
		// the new POST endpoint without relying on a stale credential.
		require.NoError(t, db.Model(&user).Update("access_token", "fixture-access-token").Error)
		w = csrfRequest(engine, saved, http.MethodPost, "/api/user/aff", "", "https://gateway.test", "same-origin")
		require.Contains(t, w.Body.String(), `"success":true`)
		require.NoError(t, db.First(&user, 971).Error)
		require.NotEmpty(t, user.AffCode)
	}
}

// TestSecurityCSRFCookieCannotBorrowBearerProvenance verifies that the existing
// session-first authentication contract cannot be bypassed with a bearer header.
func TestSecurityCSRFCookieCannotBorrowBearerProvenance(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	request := httptest.NewRequest(http.MethodPost, "https://gateway.test/api/user/token", nil)
	request.AddCookie(saved)
	request.Header.Set("Authorization", "Bearer fixture-access-token")
	request.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, request)
	require.Equal(t, http.StatusForbidden, w.Code)
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "fixture-access-token", user.AccessToken)
}

// TestSecurityCSRFTrustedFrontend verifies an explicitly configured external
// frontend can mutate an account while an unconfigured sibling cannot.
func TestSecurityCSRFTrustedFrontend(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	config.FrontendBaseURL = "https://dashboard.test/app"
	w := csrfRequest(engine, saved, http.MethodPut, "/api/user/self", `{"display_name":"trusted frontend"}`, "https://dashboard.test", "cross-site")
	require.Equal(t, http.StatusOK, w.Code)
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "trusted frontend", user.DisplayName)
	w = csrfRequest(engine, saved, http.MethodPut, "/api/user/self", `{"display_name":"attacker"}`, "https://evil.dashboard.test", "same-site")
	require.Equal(t, http.StatusForbidden, w.Code)
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "trusted frontend", user.DisplayName)
}

// TestSecurityCSRFChannelProbes covers real single and bulk probe dispatch to a
// local upstream, with joined workers so rejected requests cannot race cleanup.
func TestSecurityCSRFChannelProbes(t *testing.T) {
	for _, path := range []string{"/api/channel/test/018f0000-0000-7000-8000-000000000972", "/api/channel/test"} {
		t.Run(path, func(t *testing.T) {
			db, engine, saved := csrfFixture(t)
			require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Log{}))
			// Single probes start legacy unregistered persistence goroutines. Observe
			// their completed DB operations before restoring the global handles;
			// joining the bulk worker alone does not cover those goroutines.
			logStored, latencyStored := make(chan struct{}, 1), make(chan struct{}, 1)
			require.NoError(t, db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("csrf_log_stored", func(tx *gorm.DB) {
				if tx.Statement.Table == "logs" {
					logStored <- struct{}{}
				}
			}))
			require.NoError(t, db.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("csrf_latency_stored", func(tx *gorm.DB) {
				if tx.Statement.Table == "channels" {
					latencyStored <- struct{}{}
				}
			}))
			oldLogDB, oldInterval, oldEmail := model.LOG_DB, config.RequestInterval, config.RootUserEmail
			oldProvider, oldPusher := config.EmailProvider, config.MessagePusherAddress
			model.LOG_DB, config.RequestInterval, config.RootUserEmail = db, 0, "fixture@example.test"
			config.EmailProvider, config.MessagePusherAddress = "disabled-test-provider", ""
			t.Cleanup(func() {
				model.LOG_DB, config.RequestInterval, config.RootUserEmail = oldLogDB, oldInterval, oldEmail
				config.EmailProvider, config.MessagePusherAddress = oldProvider, oldPusher
			})
			relay.InitializeGlobalPricing()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"fixture"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				require.NoError(t, err)
			}))
			t.Cleanup(upstream.Close)
			base := upstream.URL
			require.NoError(t, db.Create(&model.Channel{Id: 972, UUID: "018f0000-0000-7000-8000-000000000972", Name: "local-probe", Type: channeltype.OpenAICompatible, Key: "fixture-only-key", BaseURL: &base, Models: "gpt-4o-mini", Status: model.ChannelStatusEnabled}).Error)
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				w := csrfRequest(engine, saved, method, path, "", "https://evil.test", "cross-site")
				if method == http.MethodPost {
					require.Equal(t, http.StatusForbidden, w.Code)
				}
				deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				require.NoError(t, model.WaitForBackgroundWorkers(deadline))
				cancel()
				require.Equal(t, int32(0), calls.Load())
			}
			w := csrfRequest(engine, saved, http.MethodPost, path, "", "https://gateway.test", "same-origin")
			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), `"success":true`)
			deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, model.WaitForBackgroundWorkers(deadline))
			for _, completion := range []chan struct{}{logStored, latencyStored} {
				select {
				case <-completion:
				case <-deadline.Done():
					t.Fatal("probe persistence did not finish before fixture cleanup")
				}
			}
			require.Greater(t, calls.Load(), int32(0), "same-origin positive control must dispatch")
		})
	}
}
