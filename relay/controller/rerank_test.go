package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestPostConsumeRerankQuotaPerCall(t *testing.T) {
	t.Parallel()
	usage := &relaymodel.Usage{PromptTokens: 42, CompletionTokens: 0}
	meta := &metalib.Meta{
		UserId:    1,
		ChannelId: 1,
		TokenId:   0,
		TokenName: "unit-test",
		StartTime: time.Now().Add(-500 * time.Millisecond),
	}
	request := &relaymodel.RerankRequest{Model: "rerank-v3.5"}

	totalQuota := int64(1000)
	preConsumed := int64(100)

	got := postConsumeRerankQuota(context.Background(), usage, meta, request, preConsumed, totalQuota, 1000, 1)
	require.Equal(t, totalQuota, got)
}
