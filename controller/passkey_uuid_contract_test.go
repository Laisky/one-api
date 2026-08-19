package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestPasskeyStrictOutResponses verifies passkey strict-out responses and strict-in path handling.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestPasskeyStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/user/passkey", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		PasskeyList(c)
	})
	router.DELETE("/api/user/passkey/:id", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		PasskeyDelete(c)
	})

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/user/passkey", nil)
	router.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code)

	var listPayload map[string]any
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	require.Equal(t, true, listPayload["success"])
	rows, ok := listPayload["data"].([]any)
	require.True(t, ok, "passkey list data must be an array: %s", listRecorder.Body.String())
	require.NotEmpty(t, rows)
	row, ok := rows[0].(map[string]any)
	require.True(t, ok, "passkey list row must be an object")
	require.Equal(t, fixture.passkey.UUID, row["uuid"])
	require.Equal(t, fixture.user.UUID, row["user_uuid"])
	require.Equal(t, fixture.passkey.CredentialName, row["credential_name"])
	require.EqualValues(t, fixture.passkey.SignCount, row["sign_count"])
	require.NotContains(t, row, "id")
	require.NotContains(t, row, "user_id")

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/user/passkey/"+fixture.passkey.UUID, nil)
	router.ServeHTTP(deleteRecorder, deleteRequest)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)

	_, err := model.GetPasskeyCredentialByID(fixture.passkey.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	legacyPasskey := &model.PasskeyCredential{
		UserId:         fixture.user.Id,
		UserUUID:       &fixture.user.UUID,
		CredentialName: "uuid-contract-legacy-passkey",
		CredentialID:   []byte("uuid-contract-legacy-passkey-credential"),
		PublicKey:      []byte("uuid-contract-legacy-passkey-public-key"),
	}
	require.NoError(t, model.DB.Create(legacyPasskey).Error)
	require.NotEmpty(t, legacyPasskey.UUID)

	legacyRecorder := httptest.NewRecorder()
	legacyRequest := httptest.NewRequest(http.MethodDelete, "/api/user/passkey/2", nil)
	router.ServeHTTP(legacyRecorder, legacyRequest)
	require.Equal(t, http.StatusOK, legacyRecorder.Code)
	var legacyPayload map[string]any
	require.NoError(t, json.Unmarshal(legacyRecorder.Body.Bytes(), &legacyPayload))
	require.Equal(t, false, legacyPayload["success"])

	_, err = model.GetPasskeyCredentialByID(legacyPasskey.Id)
	require.NoError(t, err)
}
