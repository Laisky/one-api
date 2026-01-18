package router

import (
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/controller"
	"github.com/songquanpeng/one-api/middleware"
)

func SetRelayRouter(router *gin.Engine) {
	// Rewrite various Claude Code prefixes to the canonical /v1/messages path.
	// Put this before other middlewares to avoid double-running them on redispatch.
	router.Use(
		middleware.RewriteClaudeMessagesPrefix("/v1/v1/messages", router),
		middleware.RewriteClaudeMessagesPrefix("/openai/v1/messages", router),
		middleware.RewriteClaudeMessagesPrefix("/openai/v1/v1/messages", router),
		middleware.RewriteClaudeMessagesPrefix("/api/v1/v1/messages", router),
	)

	// Auto-detect API format for misrouted requests (e.g., Response API sent to chat/completions).
	// This must be placed early in the chain, before authentication and other middlewares,
	// so that misrouted requests are redirected to the correct endpoint with all middlewares applied.
	router.Use(middleware.APIFormatAutoDetect(router))
	router.Use(middleware.CORS())
	router.Use(middleware.GzipDecodeMiddleware())
	// https://platform.openai.com/docs/api-reference/introduction
	modelsRouter := router.Group("/v1/models")
	modelsRouter.Use(middleware.TokenAuth())
	{
		modelsRouter.GET("", controller.ListModels)
		modelsRouter.GET("/:model", controller.RetrieveModel)
	}

	router.POST("/mcp", middleware.TokenAuth(), controller.MCPProxy)

	relayMws := []gin.HandlerFunc{
		// Track in-flight requests for graceful shutdown/drain
		func(c *gin.Context) { done := graceful.BeginRequest(); defer done(); c.Next() },
		middleware.RelayPanicRecover(), middleware.TokenAuth(),
		middleware.BindAsyncTaskChannel(),
		middleware.Distribute(),
		middleware.GlobalRelayRateLimit(),
		middleware.ChannelRateLimit(),
	}

	// -------------------------------------
	relayV1Router := router.Group("/v1")
	relayV1Router.Use(relayMws...)

	relayV1Router.GET("/realtime", controller.RelayRealtime)
	relayV1Router.Any("/oneapi/proxy/:channelid/*target", controller.Relay)
	relayV1Router.POST("/completions", controller.Relay)
	relayV1Router.POST("/chat/completions", controller.Relay)
	relayV1Router.POST("/responses", controller.Relay)
	relayV1Router.GET("/responses/:response_id", controller.RelayResponseGet)
	relayV1Router.DELETE("/responses/:response_id", controller.RelayResponseDelete)
	relayV1Router.POST("/responses/:response_id/cancel", controller.RelayResponseCancel)
	relayV1Router.POST("/messages", controller.Relay)
	relayV1Router.POST("/edits", controller.Relay)
	relayV1Router.POST("/images/generations", controller.Relay)
	relayV1Router.POST("/images/edits", controller.Relay)
	relayV1Router.POST("/images/variations", controller.RelayNotImplemented)
	relayV1Router.POST("/videos", controller.Relay)
	relayV1Router.GET("/videos", controller.Relay)
	relayV1Router.GET("/videos/:video_id", controller.Relay)
	relayV1Router.GET("/videos/:video_id/content", controller.Relay)
	relayV1Router.DELETE("/videos/:video_id", controller.Relay)
	relayV1Router.POST("/embeddings", controller.Relay)
	relayV1Router.POST("/rerank", controller.Relay)
	relayV1Router.POST("/engines/:model/embeddings", controller.Relay)
	relayV1Router.POST("/audio/transcriptions", controller.Relay)
	relayV1Router.POST("/audio/translations", controller.Relay)
	relayV1Router.POST("/audio/speech", controller.Relay)
	relayV1Router.GET("/files", controller.RelayNotImplemented)
	relayV1Router.POST("/files", controller.RelayNotImplemented)
	relayV1Router.DELETE("/files/:id", controller.RelayNotImplemented)
	relayV1Router.GET("/files/:id", controller.RelayNotImplemented)
	relayV1Router.GET("/files/:id/content", controller.RelayNotImplemented)
	relayV1Router.POST("/fine_tuning/jobs", controller.RelayNotImplemented)
	relayV1Router.GET("/fine_tuning/jobs", controller.RelayNotImplemented)
	relayV1Router.GET("/fine_tuning/jobs/:id", controller.RelayNotImplemented)
	relayV1Router.POST("/fine_tuning/jobs/:id/cancel", controller.RelayNotImplemented)
	relayV1Router.GET("/fine_tuning/jobs/:id/events", controller.RelayNotImplemented)
	relayV1Router.DELETE("/models/:model", controller.RelayNotImplemented)
	relayV1Router.POST("/moderations", controller.Relay)
	relayV1Router.POST("/assistants", controller.RelayNotImplemented)
	relayV1Router.GET("/assistants/:id", controller.RelayNotImplemented)
	relayV1Router.POST("/assistants/:id", controller.RelayNotImplemented)
	relayV1Router.DELETE("/assistants/:id", controller.RelayNotImplemented)
	relayV1Router.GET("/assistants", controller.RelayNotImplemented)
	relayV1Router.POST("/assistants/:id/files", controller.RelayNotImplemented)
	relayV1Router.GET("/assistants/:id/files/:fileId", controller.RelayNotImplemented)
	relayV1Router.DELETE("/assistants/:id/files/:fileId", controller.RelayNotImplemented)
	relayV1Router.GET("/assistants/:id/files", controller.RelayNotImplemented)
	relayV1Router.POST("/threads", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id", controller.RelayNotImplemented)
	relayV1Router.DELETE("/threads/:id", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/messages", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/messages/:messageId", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/messages/:messageId", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/messages/:messageId/files/:filesId", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/messages/:messageId/files", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/runs", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/runs/:runsId", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/runs/:runsId", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/runs", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/runs/:runsId/submit_tool_outputs", controller.RelayNotImplemented)
	relayV1Router.POST("/threads/:id/runs/:runsId/cancel", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/runs/:runsId/steps/:stepId", controller.RelayNotImplemented)
	relayV1Router.GET("/threads/:id/runs/:runsId/steps", controller.RelayNotImplemented)

	// -------------------------------------
	relayV2Router := router.Group("/v2")
	relayV2Router.Use(relayMws...)
	relayV2Router.POST("/rerank", controller.Relay)
}
