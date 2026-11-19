package azure

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestAzurePaidErrorReceiptScope proves Azure OpenAI deployments settle an
// explicit error receipt while Foundry Claude deployments never do.
func TestAzurePaidErrorReceiptScope(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"message":"boom"},"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	var reader adaptor.PaidErrorReceiptReader = &Adaptor{}
	gpt := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "gpt-4o-mini", ActualModelName: "gpt-4o-mini"}
	usage := reader.PaidErrorReceipt(nil, gpt, http.StatusInternalServerError, body)
	require.NotNil(t, usage)
	require.Equal(t, 15, usage.TotalTokens)

	claude := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-4-5", ActualModelName: "claude-sonnet-4-5"}
	require.True(t, claude.AzureTargetsAnthropic(), "control: the fixture must target Foundry Claude")
	require.Nil(t, reader.PaidErrorReceipt(nil, claude, http.StatusInternalServerError, body))
}
