package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestSecurityRequestCostSingleResponse proves the public request-cost lookup
// writes exactly one JSON document: an unknown request id answers one error
// envelope instead of an error followed by a second "null" body, while a known
// id still returns its cost record (positive control).
func TestSecurityRequestCostSingleResponse(t *testing.T) {
	db, engine, _ := csrfFixture(t)
	require.NoError(t, db.AutoMigrate(&model.UserRequestCost{}))
	costUserUUID := "018f0000-0000-7000-8000-000000000971"
	require.NoError(t, db.Create(&model.UserRequestCost{UUID: "018f0000-0000-7000-8000-000000000999", UserID: 971, UserUUID: &costUserUUID, RequestID: "known-request", Quota: 250000}).Error)
	browser := &oauthBrowser{engine: engine}

	missing := browser.do(http.MethodGet, "/api/cost/request/unknown-request", "", "", nil)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(missing.Body.Bytes(), &envelope), "body must be one JSON document: %s", missing.Body.String())
	require.Equal(t, false, envelope["success"])
	require.NotEmpty(t, envelope["message"])

	known := browser.do(http.MethodGet, "/api/cost/request/known-request", "", "", nil)
	require.Equal(t, http.StatusOK, known.Code)
	var cost map[string]any
	require.NoError(t, json.Unmarshal(known.Body.Bytes(), &cost), known.Body.String())
	require.Equal(t, "known-request", cost["request_id"])
	require.EqualValues(t, 0.5, cost["cost_usd"])
}
