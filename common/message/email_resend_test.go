package message

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestSendEmailViaResendSuccess verifies a successful Resend API round-trip and request shape.
func TestSendEmailViaResendSuccess(t *testing.T) {
	var gotAuth string
	var gotBody ResendEmailRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_test_msg_1"}`))
	}))
	defer server.Close()

	prevURL := resendAPIURL
	prevKey := config.ResendAPIKey
	prevFrom := config.SMTPFrom
	prevProvider := config.EmailProvider
	prevName := config.SystemName
	resendAPIURL = server.URL
	config.ResendAPIKey = "re_test_key"
	config.SMTPFrom = "noreply@example.com"
	config.EmailProvider = "resend"
	config.SystemName = "OneAPI Test"
	t.Cleanup(func() {
		resendAPIURL = prevURL
		config.ResendAPIKey = prevKey
		config.SMTPFrom = prevFrom
		config.EmailProvider = prevProvider
		config.SystemName = prevName
	})

	err := SendEmail("Hello", "user@example.com", "<b>hi</b>")
	require.NoError(t, err)
	require.Equal(t, "Bearer re_test_key", gotAuth)
	require.Equal(t, "OneAPI Test <noreply@example.com>", gotBody.From)
	require.Equal(t, []string{"user@example.com"}, gotBody.To)
	require.Equal(t, "Hello", gotBody.Subject)
	require.Equal(t, "<b>hi</b>", gotBody.Html)
}

// TestSendEmailViaResendAPIError surfaces Resend top-level error payloads.
func TestSendEmailViaResendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"statusCode":422,"name":"validation_error","message":"Invalid from address"}`))
	}))
	defer server.Close()

	prevURL := resendAPIURL
	prevKey := config.ResendAPIKey
	prevFrom := config.SMTPFrom
	prevProvider := config.EmailProvider
	resendAPIURL = server.URL
	config.ResendAPIKey = "re_test_key"
	config.SMTPFrom = "bad@example.com"
	config.EmailProvider = "resend"
	t.Cleanup(func() {
		resendAPIURL = prevURL
		config.ResendAPIKey = prevKey
		config.SMTPFrom = prevFrom
		config.EmailProvider = prevProvider
	})

	err := SendEmail("s", "a@b.com", "c")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid from address")
	require.Contains(t, err.Error(), "validation_error")
}

// TestSendEmailProviderRouting rejects unknown providers and missing Resend config.
func TestSendEmailProviderRouting(t *testing.T) {
	prevProvider := config.EmailProvider
	prevKey := config.ResendAPIKey
	t.Cleanup(func() {
		config.EmailProvider = prevProvider
		config.ResendAPIKey = prevKey
	})

	config.EmailProvider = "not-a-provider"
	err := SendEmail("s", "a@b.com", "c")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown email provider")

	config.EmailProvider = "resend"
	config.ResendAPIKey = ""
	err = SendEmail("s", "a@b.com", "c")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Resend API key is not configured")
}

// TestSendEmailViaResendMissingFrom requires SMTPFrom or SMTPAccount.
func TestSendEmailViaResendMissingFrom(t *testing.T) {
	prevProvider := config.EmailProvider
	prevKey := config.ResendAPIKey
	prevFrom := config.SMTPFrom
	prevAccount := config.SMTPAccount
	t.Cleanup(func() {
		config.EmailProvider = prevProvider
		config.ResendAPIKey = prevKey
		config.SMTPFrom = prevFrom
		config.SMTPAccount = prevAccount
	})
	config.EmailProvider = "resend"
	config.ResendAPIKey = "re_test"
	config.SMTPFrom = ""
	config.SMTPAccount = ""
	err := SendEmail("s", "a@b.com", "c")
	require.Error(t, err)
	require.Contains(t, err.Error(), "sender address is not configured")
}
