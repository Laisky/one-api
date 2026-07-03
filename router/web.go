package router

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/middleware"
)

const agentDiscoveryLinks = "<https://oneapi.laisky.com/llms.txt>; rel=\"describedby\"; type=\"text/plain\", " +
	"<https://oneapi.laisky.com/index.md>; rel=\"alternate\"; type=\"text/markdown\", " +
	"<https://oneapi.laisky.com/sitemap.xml>; rel=\"sitemap\"; type=\"application/xml\", " +
	"<https://oneapi.laisky.com/openapi.json>; rel=\"service-desc\"; type=\"application/vnd.oai.openapi+json\", " +
	"<https://oneapi.laisky.com/.well-known/api-catalog>; rel=\"api-catalog\"; type=\"application/linkset+json\", " +
	"<https://oneapi.laisky.com/.well-known/ai-catalog.json>; rel=\"describedby\"; type=\"application/json\", " +
	"<https://oneapi.laisky.com/.well-known/agent-card.json>; rel=\"describedby\"; type=\"application/json\", " +
	"<https://oneapi.laisky.com/.well-known/mcp/manifest.json>; rel=\"service-desc\"; type=\"application/json\""

// SetWebRouter registers static frontend assets and agent-readable discovery
// endpoints. Parameters: router is the Gin engine and buildFS contains the
// embedded frontend build. Return value: none; the function mutates router.
func SetWebRouter(router *gin.Engine, buildFS embed.FS) {
	indexPageData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/index.html", config.Theme))
	indexMarkdownData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/index.md", config.Theme))
	agentModeData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/agent-mode.md", config.Theme))
	apiCatalogData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/api-catalog.json", config.Theme))
	aiCatalogData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/ai-catalog.json", config.Theme))
	agentCardData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/agent-card.json", config.Theme))
	agentSkillsIndexData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/agent-skills-index.json", config.Theme))
	httpMessageSignaturesDirectoryData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/http-message-signatures-directory.json", config.Theme))
	mcpManifestData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/mcp-manifest.json", config.Theme))
	mcpServerCardData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/mcp-server-card.json", config.Theme))
	openAPIData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/openapi.json", config.Theme))
	openAPIMarkdownData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/openapi.json.md", config.Theme))
	oauthAuthorizationServerData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/oauth-authorization-server.json", config.Theme))
	oauthProtectedResourceData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/oauth-protected-resource.json", config.Theme))
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(addAgentDiscoveryHeaders())

	router.GET("/", func(c *gin.Context) {
		if c.Query("mode") == "agent" && len(agentModeData) > 0 {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", agentModeData)
			return
		}

		if wantsMarkdown(c) && len(indexMarkdownData) > 0 {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", indexMarkdownData)
			return
		}

		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPageData)
	})
	router.GET("/.well-known/api-catalog", func(c *gin.Context) {
		if len(apiCatalogData) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "application/linkset+json; charset=utf-8", apiCatalogData)
	})
	router.GET("/.well-known/api-catalog.json", func(c *gin.Context) {
		if len(apiCatalogData) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "application/linkset+json; charset=utf-8", apiCatalogData)
	})
	router.GET("/.well-known/ai-catalog.json", servePreparedAgentData(aiCatalogData, "application/json; charset=utf-8"))
	router.GET("/.well-known/agent-card.json", servePreparedAgentData(agentCardData, "application/json; charset=utf-8"))
	router.GET(
		"/.well-known/agent-skills/index.json",
		servePreparedAgentData(agentSkillsIndexData, "application/json; charset=utf-8"),
	)
	router.GET(
		"/.well-known/http-message-signatures-directory",
		servePreparedAgentData(httpMessageSignaturesDirectoryData, "application/json; charset=utf-8"),
	)
	router.GET("/openapi.json", servePreparedAgentData(openAPIData, "application/vnd.oai.openapi+json; charset=utf-8"))
	router.GET("/swagger.json", servePreparedAgentData(openAPIData, "application/vnd.oai.openapi+json; charset=utf-8"))
	router.GET("/openapi.json.md", servePreparedAgentData(openAPIMarkdownData, "text/markdown; charset=utf-8"))
	router.GET(
		"/.well-known/mcp/manifest.json",
		servePreparedAgentData(mcpManifestData, "application/json; charset=utf-8"),
	)
	router.GET("/.well-known/mcp", servePreparedAgentData(mcpManifestData, "application/json; charset=utf-8"))
	router.POST("/.well-known/mcp", servePublicMCPDiscovery)
	router.GET(
		"/.well-known/mcp/server-card.json",
		servePreparedAgentData(mcpServerCardData, "application/json; charset=utf-8"),
	)
	router.GET(
		"/.well-known/oauth-authorization-server",
		servePreparedAgentData(oauthAuthorizationServerData, "application/json; charset=utf-8"),
	)
	router.GET(
		"/.well-known/oauth-protected-resource",
		servePreparedAgentData(oauthProtectedResourceData, "application/json; charset=utf-8"),
	)
	router.GET("/docs/llms.txt", serveFileFromBuild(buildFS, "docs/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/api/llms.txt", serveFileFromBuild(buildFS, "api/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/developers/llms.txt", serveFileFromBuild(buildFS, "developers/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/.well-known/llms.txt", serveFileFromBuild(buildFS, "well-known-llms.txt", "text/plain; charset=utf-8"))
	router.GET("/.well-known/pricing.md", serveFileFromBuild(buildFS, "well-known-pricing.md", "text/markdown; charset=utf-8"))
	router.GET(
		"/.well-known/agent-skills/one-api-agent-guide.md",
		serveFileFromBuild(buildFS, "agent-skill-one-api-agent-guide.md", "text/markdown; charset=utf-8"),
	)
	router.GET("/docs", serveMarkdownFromBuild(buildFS, "docs.md"))
	router.GET("/developers", serveMarkdownFromBuild(buildFS, "developers.md"))
	router.GET("/api-reference", serveMarkdownFromBuild(buildFS, "api.md"))
	router.GET("/ask", serveAgentAsk)
	router.POST("/ask", serveAgentAsk)
	router.GET("/sandbox", serveMarkdownFromBuild(buildFS, "sandbox.md"))
	router.GET("/developer-resources", serveMarkdownFromBuild(buildFS, "developer-resources.md"))
	router.GET("/laisky-developer-resources", serveMarkdownFromBuild(buildFS, "laisky-developer-resources.md"))
	router.POST("/sandbox/v1/chat/completions", serveSandboxChatCompletion)
	router.POST("/sandbox/v1/responses", serveSandboxResponse)
	router.POST("/sandbox/v1/messages", serveSandboxClaudeMessage)

	router.Use(static.Serve("/", common.EmbedFolder(buildFS, fmt.Sprintf("web/build/%s", config.Theme))))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPageData)
	})
}

// addAgentDiscoveryHeaders returns middleware that advertises machine-readable
// agent resources through RFC 8288 Link headers. Parameters: none. Return
// value: a Gin middleware function.
func addAgentDiscoveryHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Link", agentDiscoveryLinks)
		c.Header("Vary", "Accept, Accept-Encoding")
		c.Next()
	}
}

type publicMCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
}

// servePublicMCPDiscovery handles unauthenticated MCP discovery calls for
// agents. Parameters: c carries the JSON-RPC request. Return value: none; the
// function writes a JSON-RPC response describing public documentation only.
func servePublicMCPDiscovery(c *gin.Context) {
	var req publicMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, publicMCPError(nil, -32700, "invalid JSON-RPC request"))
		return
	}

	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": req.JSONRPC,
			"id":      req.ID,
			"result": gin.H{
				"protocolVersion": "2025-06-18",
				"capabilities": gin.H{
					"tools": gin.H{"listChanged": false},
				},
				"serverInfo": gin.H{
					"name":    "Laisky One API Public Discovery",
					"version": "0.6",
				},
				"instructions": "This public MCP endpoint exposes documentation discovery only. Use Authorization: Bearer <relay-api-key> with https://oneapi.laisky.com/mcp for authenticated configured tools.",
			},
		})
	case "tools/list":
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": req.JSONRPC,
			"id":      req.ID,
			"result": gin.H{
				"tools": []gin.H{
					{
						"name":        "one_api_public_docs",
						"description": "Return public Laisky One API integration links and capability notes.",
						"inputSchema": gin.H{
							"type":                 "object",
							"additionalProperties": false,
							"properties": gin.H{
								"topic": gin.H{
									"type":        "string",
									"description": "Optional topic such as auth, openapi, mcp, pricing, or models.",
								},
							},
						},
					},
				},
			},
		})
	case "tools/call":
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": req.JSONRPC,
			"id":      req.ID,
			"result": gin.H{
				"content": []gin.H{
					{
						"type": "text",
						"text": "Laisky One API public docs: https://oneapi.laisky.com/llms.txt, https://oneapi.laisky.com/openapi.json, https://oneapi.laisky.com/auth.md, https://oneapi.laisky.com/api.md, and https://oneapi.laisky.com/.well-known/api-catalog. Authenticated MCP tools are available at https://oneapi.laisky.com/mcp with a relay API key.",
					},
				},
				"isError": false,
			},
		})
	default:
		c.JSON(http.StatusOK, publicMCPError(req.ID, -32601, "method not found"))
	}
}

// publicMCPError builds a JSON-RPC error response. Parameters: id is the
// request identifier, code is the JSON-RPC error code, and message is the human
// readable reason. Return value: a response object suitable for c.JSON.
func publicMCPError(id any, code int, message string) gin.H {
	return gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
}

// serveAgentAsk returns a public NLWeb-style discovery answer for agents.
// Parameters: c carries an optional q query parameter or JSON question field.
// Return value: none; the function writes JSON or text/event-stream.
func serveAgentAsk(c *gin.Context) {
	question := strings.TrimSpace(c.Query("q"))
	if question == "" && c.Request.Method == http.MethodPost {
		var body struct {
			Question string `json:"question"`
			Query    string `json:"query"`
		}
		if err := c.ShouldBindJSON(&body); err == nil {
			if body.Question != "" {
				question = body.Question
			} else {
				question = body.Query
			}
		}
	}

	response := agentAskResponse(trimAgentQuestion(question))
	if strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/event-stream") {
		payload, err := json.Marshal(response)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Data(http.StatusOK, "text/event-stream; charset=utf-8", []byte("event: result\ndata: "+string(payload)+"\n\n"))
		return
	}

	c.JSON(http.StatusOK, response)
}

// trimAgentQuestion normalizes and bounds public /ask input. Parameters:
// question is untrusted user input. Return value: a trimmed string with a
// maximum length suitable for lightweight discovery responses.
func trimAgentQuestion(question string) string {
	question = strings.TrimSpace(question)
	if len(question) > 500 {
		return question[:500]
	}

	return question
}

// agentAskResponse builds the public /ask response. Parameters: question is a
// bounded user prompt. Return value: NLWeb-style metadata, answer, citations,
// and capability links.
func agentAskResponse(question string) gin.H {
	if question == "" {
		question = "How do agents integrate with Laisky One API?"
	}

	return gin.H{
		"answer":   "Laisky One API is an agent-friendly AI gateway at oneapi.laisky.com. Start with /llms.txt and /openapi.json, authenticate relay calls with Authorization: Bearer <relay-api-key>, and use /v1/chat/completions, /v1/responses, /v1/messages, or authenticated /mcp according to your client format.",
		"question": question,
		"citations": []gin.H{
			{"title": "LLM instructions", "url": "https://oneapi.laisky.com/llms.txt"},
			{"title": "OpenAPI", "url": "https://oneapi.laisky.com/openapi.json"},
			{"title": "Authentication", "url": "https://oneapi.laisky.com/auth.md"},
			{"title": "MCP manifest", "url": "https://oneapi.laisky.com/.well-known/mcp/manifest.json"},
		},
		"capabilities": []string{
			"OpenAI Chat Completions relay",
			"OpenAI Responses relay",
			"Claude Messages relay",
			"MCP Streamable HTTP relay for authenticated configured tools",
		},
		"_meta": gin.H{
			"schema": "nlweb",
			"source": "oneapi.laisky.com",
			"type":   "agent-discovery-answer",
		},
	}
}

// serveSandboxChatCompletion returns a mock Chat Completions response for
// unauthenticated agent testing. Parameters: c carries the incoming request.
// Return value: none; the function writes a static OpenAI-compatible response.
func serveSandboxChatCompletion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":      "chatcmpl-sandbox",
		"object":  "chat.completion",
		"created": 1783036800,
		"model":   "oneapi-sandbox",
		"choices": []gin.H{
			{
				"index": 0,
				"message": gin.H{
					"role":    "assistant",
					"content": "This is a mock Laisky One API sandbox response. Use authenticated /v1/chat/completions with a relay API key for real provider calls.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": gin.H{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	})
}

// serveSandboxResponse returns a mock Responses API object for unauthenticated
// agent testing. Parameters: c carries the incoming request. Return value: none;
// the function writes a static Responses-compatible response.
func serveSandboxResponse(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":     "resp_sandbox",
		"object": "response",
		"model":  "oneapi-sandbox",
		"output": []gin.H{
			{
				"type": "message",
				"role": "assistant",
				"content": []gin.H{
					{
						"type": "output_text",
						"text": "This is a mock Laisky One API sandbox response. Use authenticated /v1/responses with a relay API key for real provider calls.",
					},
				},
			},
		},
		"usage": gin.H{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
	})
}

// serveSandboxClaudeMessage returns a mock Claude Messages response for
// unauthenticated agent testing. Parameters: c carries the incoming request.
// Return value: none; the function writes a static Claude-compatible response.
func serveSandboxClaudeMessage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":            "msg_sandbox",
		"type":          "message",
		"role":          "assistant",
		"model":         "oneapi-sandbox",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"content": []gin.H{
			{
				"type": "text",
				"text": "This is a mock Laisky One API sandbox response. Use authenticated /v1/messages with a relay API key for real provider calls.",
			},
		},
		"usage": gin.H{"input_tokens": 0, "output_tokens": 0},
	})
}

// serveMarkdownFromBuild returns a handler that serves a markdown document from
// the selected embedded frontend build. Parameters: buildFS is the embedded
// filesystem and filename is relative to the theme build root. Return value: a
// Gin handler that serves text/markdown or 404 when the file is absent.
func serveMarkdownFromBuild(buildFS embed.FS, filename string) gin.HandlerFunc {
	return serveFileFromBuild(buildFS, filename, "text/markdown; charset=utf-8")
}

// serveFileFromBuild returns a handler that serves a static document from the
// selected embedded frontend build. Parameters: buildFS is the embedded
// filesystem, filename is relative to the theme build root, and contentType is
// the HTTP Content-Type value. Return value: a Gin handler that serves bytes or
// 404 when the file is absent.
func serveFileFromBuild(buildFS embed.FS, filename string, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := buildFS.ReadFile(fmt.Sprintf("web/build/%s/%s", config.Theme, filename))
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, contentType, data)
	}
}

// servePreparedAgentData returns a handler that serves preloaded agent
// discovery bytes. Parameters: data is the response body and contentType is the
// HTTP Content-Type value. Return value: a Gin handler that serves the data or
// 404 when it was not embedded.
func servePreparedAgentData(data []byte, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(data) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, contentType, data)
	}
}

// wantsMarkdown reports whether a request prefers markdown over HTML.
// Parameters: c carries the incoming request headers. Return value: true when
// the Accept header asks for text/markdown.
func wantsMarkdown(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/markdown")
}
