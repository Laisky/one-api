package aws

import (
	"testing"

	"github.com/stretchr/testify/require"

	qwen "github.com/songquanpeng/one-api/relay/adaptor/aws/qwen"
)

func TestGetAdaptorReturnsQwenAdaptor(t *testing.T) {
	t.Parallel()
	adaptor := GetAdaptor("qwen3-32b")
	require.NotNil(t, adaptor, "expected non-nil adaptor for qwen model")

	_, ok := adaptor.(*qwen.Adaptor)
	require.True(t, ok, "expected adaptor type *qwen.Adaptor, got %T", adaptor)
}
