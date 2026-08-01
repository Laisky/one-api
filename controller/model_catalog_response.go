package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// respondModelList writes the standard OpenAI model list and adds Codex's required catalog array
// when the request carries Codex's client-version discovery signal. It accepts the request context
// and available OpenAI models and returns no value.
func respondModelList(c *gin.Context, availableModels []OpenAIModels) {
	response := gin.H{
		"object": "list",
		"data":   availableModels,
	}
	if strings.TrimSpace(c.Query("client_version")) != "" {
		// Keep the Codex catalog empty so Codex retains its safer fallback metadata
		// instead of replacing it with incomplete gateway metadata.
		response["models"] = make([]any, 0)
	}

	c.JSON(http.StatusOK, response)
}
