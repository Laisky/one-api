package controller

import (
	"context"
	"encoding/json"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/constant/role"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSetSystemPromptSuppressesContent verifies both prompt application paths
// retain their business behavior while diagnostic logs expose only byte counts.
func TestSetSystemPromptSuppressesContent(t *testing.T) {
	for _, existingRole := range []string{"user", role.System} {
		t.Run(existingRole, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewConsoleWithName("privacy", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			ctx := gmw.SetLogger(context.Background(), lg)
			request := &relaymodel.GeneralOpenAIRequest{Messages: []relaymodel.Message{{Role: existingRole, Content: "original"}}}
			prompt := "sentinel-system-prompt-secret"
			require.True(t, setSystemPrompt(ctx, request, prompt))
			require.Equal(t, role.System, request.Messages[0].Role)
			require.Equal(t, prompt, request.Messages[0].Content)
			require.Len(t, observed.All(), 1)
			encoded, err := json.Marshal(observed.All()[0].ContextMap())
			require.NoError(t, err)
			require.NotContains(t, string(encoded), prompt)
			require.Equal(t, int64(len(prompt)), observed.All()[0].ContextMap()["prompt_bytes"])
			require.NotContains(t, observed.All()[0].ContextMap(), "prompt")
			if existingRole == "user" {
				require.Len(t, request.Messages, 2)
				require.Equal(t, "original", request.Messages[1].Content)
			} else {
				require.Len(t, request.Messages, 1)
			}
		})
	}
}
