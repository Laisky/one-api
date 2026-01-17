package router

import (
	"github.com/songquanpeng/one-api/controller"
	"github.com/songquanpeng/one-api/controller/auth"
	"github.com/songquanpeng/one-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/status/channel", controller.GetChannelStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		// Public endpoint: anonymous users see all supported models; logged-in users see only allowed models
		apiRouter.GET("/models/display", controller.GetModelsDisplay)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/about", controller.GetAbout)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/verification", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.GET("/user/get-by-token", middleware.TokenAuth(), controller.GetSelfByToken)
		apiRouter.GET("/available_models", middleware.TokenAuth(), controller.GetAvailableModelsByToken)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		apiRouter.GET("/oauth/github", middleware.CriticalRateLimit(), auth.GitHubOAuth)
		apiRouter.GET("/oauth/oidc", middleware.CriticalRateLimit(), auth.OidcAuth)
		apiRouter.GET("/oauth/lark", middleware.CriticalRateLimit(), auth.LarkOAuth)
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), auth.GenerateOAuthCode)
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), auth.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), middleware.UserAuth(), auth.WeChatBind)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), middleware.UserAuth(), controller.EmailBind)
		apiRouter.POST("/topup", middleware.AdminAuth(), controller.AdminTopUp)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.GET("/logout", controller.Logout)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/dashboard", controller.GetUserDashboard)
				selfRoute.GET("/dashboard/users", controller.GetDashboardUsers)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.POST("/topup", controller.TopUp)
				selfRoute.GET("/available_models", controller.GetUserAvailableModels)
				selfRoute.GET("/totp/status", controller.GetTotpStatus)
				selfRoute.GET("/totp/setup", controller.SetupTotp)
				selfRoute.POST("/totp/confirm", controller.ConfirmTotp)
				selfRoute.POST("/totp/disable", controller.DisableTotp)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.POST("/totp/disable/:id", controller.AdminDisableUserTotp)
			}
		}
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ListAllModels)
			channelRoute.GET("/metadata", controller.GetChannelMetadata)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.GET("/test", controller.TestChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.GET("/pricing/:id", controller.GetChannelPricing)
			channelRoute.GET("/default-pricing", controller.GetChannelDefaultPricing)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.PUT("/pricing/:id", controller.UpdateChannelPricing)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
		}
		debugRoute := apiRouter.Group("/debug")
		debugRoute.Use(middleware.AdminAuth())
		{
			debugRoute.POST("/channel/:id/debug", controller.DebugChannelModelConfigs)
			debugRoute.GET("/channels", controller.DebugAllChannelModelConfigs)
			debugRoute.POST("/channel/:id/fix", controller.FixChannelModelConfigs)
			debugRoute.GET("/channels/validate", controller.ValidateAllChannelModelConfigs)
			debugRoute.POST("/channels/remigrate", controller.RemigratAllChannels)
			debugRoute.GET("/channel/:id/migration-status", controller.GetChannelMigrationStatus)
			debugRoute.POST("/channels/clean", controller.CleanAllMixedModelData)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			apiRouter.POST("/token/consume", middleware.TokenAuth(), controller.ConsumeToken)
			apiRouter.GET("/token/balance", middleware.TokenAuth(), controller.GetTokenBalance)
			apiRouter.GET("/token/transactions", middleware.TokenAuth(), controller.GetTokenTransactions)
			apiRouter.GET("/token/logs", middleware.TokenAuth(), controller.GetTokenLogs)
		}
		costRoute := apiRouter.Group("/cost")
		{
			costRoute.GET("/request/:request_id", controller.GetRequestCost)
		}
		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), controller.SearchUserLogs)

		// Tracing routes
		traceRoute := apiRouter.Group("/trace")
		traceRoute.Use(middleware.UserAuth()) // Users can view traces for their own logs
		{
			traceRoute.GET("/log/:log_id", controller.GetTraceByLogId)
			traceRoute.GET("/:trace_id", controller.GetTraceByTraceId)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		mcpServerRoute := apiRouter.Group("/mcp_servers")
		mcpServerRoute.Use(middleware.AdminAuth())
		{
			mcpServerRoute.GET("/", controller.GetMCPServers)
			mcpServerRoute.GET("", controller.GetMCPServers)
			mcpServerRoute.GET("/:id", controller.GetMCPServer)
			mcpServerRoute.POST("/", controller.CreateMCPServer)
			mcpServerRoute.POST("", controller.CreateMCPServer)
			mcpServerRoute.PUT("/:id", controller.UpdateMCPServer)
			mcpServerRoute.DELETE("/:id", controller.DeleteMCPServer)
			mcpServerRoute.POST("/:id/sync", controller.SyncMCPServer)
			mcpServerRoute.POST("/:id/test", controller.TestMCPServer)
			mcpServerRoute.GET("/:id/tools", controller.ListMCPServerTools)
		}

		mcpToolRoute := apiRouter.Group("/mcp_tools")
		mcpToolRoute.Use(middleware.AdminAuth())
		{
			mcpToolRoute.GET("/", controller.GetMCPTools)
			mcpToolRoute.GET("", controller.GetMCPTools)
		}
	}
}
