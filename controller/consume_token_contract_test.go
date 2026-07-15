package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// ConsumeToken (controller/token.go) is the only TokenAuth/API-client-facing site
// that echoes a token entity back as `data`, so its response shape is an external
// client contract rather than an internal management-console detail. The key sets
// frozen below are the exact bytes the pre-refactor binary emits; they must not
// drift when the serializer behind `data` changes.
var (
	// consumeTokenContractTopLevelKeys is the exact top-level key set of a
	// successful ConsumeToken response for every phase that yields a transaction.
	consumeTokenContractTopLevelKeys = []string{
		"data",
		"message",
		"success",
		"transaction",
	}

	// consumeTokenContractDataKeys is the exact key set of the nested `data`
	// object. It carries UUID identifiers only: no `id`, no `user_id`.
	consumeTokenContractDataKeys = []string{
		"accessed_time",
		"created_at",
		"created_time",
		"expired_time",
		"key",
		"models",
		"name",
		"remain_quota",
		"status",
		"subnet",
		"unlimited_quota",
		"updated_at",
		"used_quota",
		"user_uuid",
		"uuid",
	}

	// consumeTokenContractTxnBaseKeys is the key set buildTransactionResponse
	// emits unconditionally. `final_quota` is always present (explicitly null
	// while a hold is still pending); `confirmed_at`, `canceled_at` and
	// `elapsed_time_ms` are phase-conditional and appended per subtest.
	consumeTokenContractTxnBaseKeys = []string{
		"auto_confirmed",
		"expires_at",
		"final_quota",
		"log_uuid",
		"pre_quota",
		"reason",
		"request_id",
		"status",
		"status_code",
		"token_uuid",
		"trace_id",
		"transaction_id",
		"user_uuid",
		"uuid",
	}

	// consumeTokenContractForbiddenKeys must never appear at any depth of a
	// ConsumeToken response: internal integer identifiers and user secrets.
	// Note `key` is deliberately absent from this list — the token key is part
	// of the frozen contract, returned (prefixed) to the token's own bearer.
	consumeTokenContractForbiddenKeys = []string{
		"access_token",
		"id",
		"inviter_id",
		"log_id",
		"password",
		"token_id",
		"totp_secret",
		"user_id",
		"verification_code",
	}
)

// consumeTokenContractDo issues a single ConsumeToken request against the seeded
// fixtures and decodes the successful response body.
//
// Parameters:
//   - t: the running test.
//   - userID: the authenticated user id bound to the request context.
//   - tokenID: the authenticated token id bound to the request context.
//   - requestID: the value bound to helper.RequestIdKey.
//   - body: the raw JSON request body.
//
// Return values:
//   - the decoded top-level response object of a 200 OK ConsumeToken call.
//   - the raw response bytes, for assertions that need the wire form.
func consumeTokenContractDo(t *testing.T, userID, tokenID int, requestID, body string) (map[string]any, []byte) {
	t.Helper()

	c, recorder := newConsumeTokenContext(t, http.MethodPost, body, userID, tokenID, requestID)
	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code, "body: %s", recorder.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp["success"].(bool), "body: %s", recorder.Body.String())

	return resp, recorder.Body.Bytes()
}

// consumeTokenContractObjectKeys extracts the sorted key set of a decoded JSON object.
//
// Parameters:
//   - t: the running test.
//   - value: a decoded JSON value that must be an object.
//
// Return values:
//   - the object's keys in lexical order.
func consumeTokenContractObjectKeys(t *testing.T, value any) []string {
	t.Helper()

	obj, ok := value.(map[string]any)
	require.True(t, ok, "expected a JSON object, got %T", value)

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

// consumeTokenContractExpectTxnKeys builds the expected transaction key set for a
// phase by appending its conditional keys to the unconditional base set.
//
// Parameters:
//   - extra: phase-conditional keys such as "confirmed_at" or "canceled_at".
//
// Return values:
//   - the expected transaction key set in lexical order.
func consumeTokenContractExpectTxnKeys(extra ...string) []string {
	keys := append([]string{}, consumeTokenContractTxnBaseKeys...)
	keys = append(keys, extra...)
	sort.Strings(keys)

	return keys
}

// consumeTokenContractAssertNoForbiddenKeys walks a decoded JSON value and fails
// if any forbidden key is present at any depth.
//
// Parameters:
//   - t: the running test.
//   - value: a decoded JSON value.
//   - path: the JSON path of value, used to report a readable location.
func consumeTokenContractAssertNoForbiddenKeys(t *testing.T, value any, path string) {
	t.Helper()

	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			for _, forbidden := range consumeTokenContractForbiddenKeys {
				require.NotEqual(t, forbidden, k, "forbidden key %q found at %s", forbidden, path)
			}
			consumeTokenContractAssertNoForbiddenKeys(t, v, path+"."+k)
		}
	case []any:
		for i, v := range typed {
			consumeTokenContractAssertNoForbiddenKeys(t, v, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// TestConsumeTokenResponseContract freezes the full external shape of the
// ConsumeToken response — top level, the nested `data` token object and the
// `transaction` object — for every phase. The frozen key sets are captured from
// the pre-refactor binary, so this test must pass unchanged on both sides of the
// boundary-response-DTO flip.
func TestConsumeTokenResponseContract(t *testing.T) {
	testCases := []struct {
		name string
		// prepare optionally runs a pre-consume and returns the follow-up body
		// (which needs the generated transaction id).
		prepare      func(t *testing.T, userID, tokenID int) string
		body         string
		expectTxnAdd []string
		expectStatus string
	}{
		{
			name:         "pre",
			body:         `{"phase":"pre","add_used_quota":100,"add_reason":"contract-pre","timeout_seconds":30}`,
			expectTxnAdd: nil,
			expectStatus: "pending",
		},
		{
			name: "post",
			prepare: func(t *testing.T, userID, tokenID int) string {
				resp, _ := consumeTokenContractDo(t, userID, tokenID, "req-contract-pre",
					`{"phase":"pre","add_used_quota":100,"add_reason":"contract-post","timeout_seconds":30}`)
				txn := resp["transaction"].(map[string]any)
				return fmt.Sprintf(
					`{"phase":"post","transaction_id":%q,"final_used_quota":80,"add_reason":"contract-post"}`,
					txn["transaction_id"].(string))
			},
			expectTxnAdd: []string{"confirmed_at"},
			expectStatus: "confirmed",
		},
		{
			name: "cancel",
			prepare: func(t *testing.T, userID, tokenID int) string {
				resp, _ := consumeTokenContractDo(t, userID, tokenID, "req-contract-pre",
					`{"phase":"pre","add_used_quota":100,"add_reason":"contract-cancel","timeout_seconds":30}`)
				txn := resp["transaction"].(map[string]any)
				return fmt.Sprintf(
					`{"phase":"cancel","transaction_id":%q,"add_reason":"contract-cancel"}`,
					txn["transaction_id"].(string))
			},
			expectTxnAdd: []string{"canceled_at"},
			expectStatus: "canceled",
		},
		{
			name:         "single",
			body:         `{"phase":"single","add_used_quota":100,"add_reason":"contract-single"}`,
			expectTxnAdd: []string{"confirmed_at"},
			expectStatus: "confirmed",
		},
		{
			name:         "single_zero_quota",
			body:         `{"phase":"single","add_used_quota":0,"add_reason":"contract-single-free"}`,
			expectTxnAdd: []string{"confirmed_at"},
			expectStatus: "confirmed",
		},
		{
			name:         "single_with_elapsed",
			body:         `{"phase":"single","add_used_quota":100,"add_reason":"contract-elapsed","elapsed_time_ms":42}`,
			expectTxnAdd: []string{"confirmed_at", "elapsed_time_ms"},
			expectStatus: "confirmed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup, user, token := setupConsumeTokenTest(t)
			defer cleanup()

			body := tc.body
			if tc.prepare != nil {
				body = tc.prepare(t, user.Id, token.Id)
			}

			resp, raw := consumeTokenContractDo(t, user.Id, token.Id, "req-contract", body)

			// 1. Top level: exactly success/message/data/transaction.
			require.Equal(t, consumeTokenContractTopLevelKeys, consumeTokenContractObjectKeys(t, resp))

			// 2. Nested data: the full token shape, UUID-keyed only.
			require.Equal(t, consumeTokenContractDataKeys, consumeTokenContractObjectKeys(t, resp["data"]))

			// 3. Transaction: base keys plus this phase's conditional keys.
			require.Equal(t, consumeTokenContractExpectTxnKeys(tc.expectTxnAdd...),
				consumeTokenContractObjectKeys(t, resp["transaction"]))

			// 4. No internal integer id and no user secret at any depth.
			consumeTokenContractAssertNoForbiddenKeys(t, resp, "$")

			// 5. Identity is carried by UUIDs that actually resolve.
			data := resp["data"].(map[string]any)
			txn := resp["transaction"].(map[string]any)
			require.Equal(t, token.UUID, data["uuid"])
			require.Equal(t, user.UUID, data["user_uuid"])
			require.Equal(t, token.UUID, txn["token_uuid"])
			require.Equal(t, user.UUID, txn["user_uuid"])
			require.Equal(t, tc.expectStatus, txn["status"])

			// 6. `key` is part of the frozen contract: the caller's own token
			// key, returned with the configured prefix applied at response time.
			prefix := config.TokenKeyPrefix
			if prefix == "" {
				prefix = "sk-"
			}
			require.Equal(t, prefix+token.Key, data["key"])

			// 7. Guard against an integer id sneaking back in under any name:
			// the seeded ids are 1/1, and no bare `1` may appear as a value.
			require.NotContains(t, string(raw), `"id":`)
			require.NotContains(t, string(raw), `"user_id":`)
			require.False(t, strings.Contains(string(raw), `"password"`))
		})
	}
}
