package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
)

func TestRenderChatResponseAsResponseAPIUsesOriginModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	request := &openai.ResponseAPIRequest{Model: "public-alias"}
	meta := &metalib.Meta{OriginModelName: "public-alias", ActualModelName: "hidden-target"}
	textResp := &openai_compatible.SlimTextResponse{}

	require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, textResp, request, meta))
	require.Equal(t, http.StatusOK, w.Code)

	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "public-alias", response.Model)
}
