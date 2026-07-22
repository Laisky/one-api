package message

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	// net/mail RFC 5322-quotes a display name that contains spaces.
	require.Equal(t, `"OneAPI Test" <noreply@example.com>`, gotBody.From)
	require.Equal(t, []string{"user@example.com"}, gotBody.To)
	require.Equal(t, "Hello", gotBody.Subject)
	require.Equal(t, "<b>hi</b>", gotBody.Html)
}

// TestResendSenderAddress verifies the from header is a valid RFC 5322 mailbox
// for display names that need quoting/encoding and for pre-formatted SMTPFrom values.
func TestResendSenderAddress(t *testing.T) {
	prevFrom := config.SMTPFrom
	prevAccount := config.SMTPAccount
	prevName := config.SystemName
	t.Cleanup(func() {
		config.SMTPFrom = prevFrom
		config.SMTPAccount = prevAccount
		config.SystemName = prevName
	})

	cases := []struct {
		name       string
		smtpFrom   string
		account    string
		systemName string
		want       string
	}{
		{
			name:       "plain name is quoted",
			smtpFrom:   "noreply@example.com",
			systemName: "OneAPI",
			want:       `"OneAPI" <noreply@example.com>`,
		},
		{
			name:       "name with comma is quoted",
			smtpFrom:   "noreply@example.com",
			systemName: "Acme, Inc.",
			want:       `"Acme, Inc." <noreply@example.com>`,
		},
		{
			name:       "name with quote is escaped",
			smtpFrom:   "noreply@example.com",
			systemName: `Quote"Me`,
			want:       `"Quote\"Me" <noreply@example.com>`,
		},
		{
			name:       "non-ascii name is rfc2047 encoded",
			smtpFrom:   "noreply@example.com",
			systemName: "Björk",
			want:       "=?utf-8?q?Bj=C3=B6rk?= <noreply@example.com>",
		},
		{
			name:       "preformatted SMTPFrom keeps its own name",
			smtpFrom:   "Existing Name <noreply@example.com>",
			systemName: "Ignored",
			want:       `"Existing Name" <noreply@example.com>`,
		},
		{
			name:       "no system name yields bare address",
			smtpFrom:   "noreply@example.com",
			systemName: "",
			want:       "<noreply@example.com>",
		},
		{
			name:       "falls back to SMTPAccount",
			smtpFrom:   "",
			account:    "mailer@example.com",
			systemName: "OneAPI",
			want:       `"OneAPI" <mailer@example.com>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config.SMTPFrom = tc.smtpFrom
			config.SMTPAccount = tc.account
			config.SystemName = tc.systemName
			got, err := resendSenderAddress()
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestResendSenderAddressInvalid rejects an unparseable sender address.
func TestResendSenderAddressInvalid(t *testing.T) {
	prevFrom := config.SMTPFrom
	prevAccount := config.SMTPAccount
	prevName := config.SystemName
	t.Cleanup(func() {
		config.SMTPFrom = prevFrom
		config.SMTPAccount = prevAccount
		config.SystemName = prevName
	})
	config.SMTPFrom = "not-an-email"
	config.SMTPAccount = ""
	config.SystemName = "OneAPI"
	_, err := resendSenderAddress()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid Resend sender address")
}

// TestSendEmailViaResendChunksRecipients confirms lists over the 50-recipient cap
// are split across multiple Resend requests, each within the limit.
func TestSendEmailViaResendChunksRecipients(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int
	var allRecipients []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got ResendEmailRequest
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &got))
		mu.Lock()
		batchSizes = append(batchSizes, len(got.To))
		allRecipients = append(allRecipients, got.To...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_batch"}`))
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
	config.SystemName = "OneAPI"
	t.Cleanup(func() {
		resendAPIURL = prevURL
		config.ResendAPIKey = prevKey
		config.SMTPFrom = prevFrom
		config.EmailProvider = prevProvider
		config.SystemName = prevName
	})

	const total = 120
	recipients := make([]string, 0, total)
	for i := 0; i < total; i++ {
		recipients = append(recipients, fmt.Sprintf("user%03d@example.com", i))
	}

	err := SendEmail("Notice", strings.Join(recipients, ";"), "<b>hi</b>")
	require.NoError(t, err)

	require.Equal(t, []int{50, 50, 20}, batchSizes)
	for _, size := range batchSizes {
		require.LessOrEqual(t, size, resendMaxRecipientsPerRequest)
	}
	require.ElementsMatch(t, recipients, allRecipients)
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

// TestSendEmailAutoProviderResolution verifies the empty EmailProvider falls back
// to Resend when an API key is present and to SMTP otherwise (backward-compat routing).
func TestSendEmailAutoProviderResolution(t *testing.T) {
	prevURL := resendAPIURL
	prevKey := config.ResendAPIKey
	prevProvider := config.EmailProvider
	prevFrom := config.SMTPFrom
	prevAccount := config.SMTPAccount
	prevName := config.SystemName
	prevServer := config.SMTPServer
	prevPort := config.SMTPPort
	t.Cleanup(func() {
		resendAPIURL = prevURL
		config.ResendAPIKey = prevKey
		config.EmailProvider = prevProvider
		config.SMTPFrom = prevFrom
		config.SMTPAccount = prevAccount
		config.SystemName = prevName
		config.SMTPServer = prevServer
		config.SMTPPort = prevPort
	})

	t.Run("empty provider with key routes to resend", func(t *testing.T) {
		var hits int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"re_auto"}`))
		}))
		defer server.Close()

		resendAPIURL = server.URL
		config.EmailProvider = ""
		config.ResendAPIKey = "re_auto_key"
		config.SMTPFrom = "noreply@example.com"
		config.SystemName = "OneAPI"

		err := SendEmail("s", "a@b.com", "c")
		require.NoError(t, err)
		require.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("empty provider without key routes to smtp", func(t *testing.T) {
		config.EmailProvider = ""
		config.ResendAPIKey = ""
		config.SMTPFrom = "noreply@example.com"
		config.SMTPAccount = ""
		config.SMTPServer = "127.0.0.1"
		config.SMTPPort = 1 // connection refused immediately, proving the SMTP branch was taken

		err := SendEmail("s", "a@b.com", "c")
		require.Error(t, err)
		require.Contains(t, err.Error(), "dial SMTP client")
	})
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
