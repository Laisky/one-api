package controller

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

// TestSecurityRootPolicyDisclosure checks the shipped operator/login copy and
// API documentation, not a simulated successful authentication bypass.
func TestSecurityRootPolicyDisclosure(t *testing.T) {
	for _, locale := range []string{"en", "zh", "ja", "fr", "es"} {
		for _, name := range []string{"settings", "auth"} {
			raw, err := os.ReadFile("../web/modern/src/i18n/locales/" + locale + "/" + name + ".json")
			require.NoError(t, err)
			var root map[string]any
			require.NoError(t, json.Unmarshal(raw, &root))
			var texts []string
			if name == "settings" {
				texts = append(texts, root["system_settings"].(map[string]any)["descriptions"].(map[string]any)["PasswordLoginEnabled"].(string))
			} else {
				login := root["auth"].(map[string]any)["login"].(map[string]any)
				for _, key := range []string{"password_login_disabled", "password_login_disabled_no_methods"} {
					texts = append(texts, login[key].(string))
				}
			}
			for _, text := range texts {
				require.Contains(t, strings.ToLower(text), "root", locale+":"+name)
				require.Contains(t, text, "TOTP", locale+":"+name)
			}
		}
	}
	raw, err := os.ReadFile("../docs/manuals/api_references.md")
	require.NoError(t, err)
	require.Contains(t, string(raw), "**Root recovery policy:**")
}

// TestSecurityRootRecoveryStillRequiresValidAccountAndFactor preserves the
// intentional recovery exception without exempting password, status or TOTP.
func TestSecurityRootRecoveryStillRequiresValidAccountAndFactor(t *testing.T) {
	for _, test := range []struct {
		name             string
		status           int
		password, secret string
		success          bool
		message          string
	}{
		{"enabled", model.UserStatusEnabled, "correctpassword", "", true, ""},
		{"wrong", model.UserStatusEnabled, "wrongpassword", "", false, ""},
		{"disabled", model.UserStatusDisabled, "correctpassword", "", false, ""},
		{"deleted", model.UserStatusDeleted, "correctpassword", "", false, ""},
		{"totp", model.UserStatusEnabled, "correctpassword", "JBSWY3DPEHPK3PXP", false, "totp_required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupPasswordLoginDisabledTest(t)
			username := "recovery-" + test.name
			user := createLoginUser(t, username, "correctpassword", model.RoleRootUser)
			defer middleware.ClearLoginFailure(username)
			require.NoError(t, model.DB.Model(user).Updates(map[string]any{"status": test.status, "totp_secret": test.secret}).Error)
			config.PasswordLoginEnabled = false
			response := postLogin(t, newLoginRouter(), username, test.password)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, test.success, body["success"])
			if test.message != "" {
				require.Equal(t, test.message, body["message"])
			}
			if !test.success {
				require.Empty(t, response.Result().Cookies(), "failed recovery must not mint a login session")
			}
		})
	}
}
