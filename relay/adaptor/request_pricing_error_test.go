package adaptor

import (
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// stubPricingValidator is a provider whose validator returns a fixed verdict.
type stubPricingValidator struct {
	Adaptor
	verdict error
}

// stubPlainAdaptor is a provider without a RequestPricingValidator.
type stubPlainAdaptor struct {
	Adaptor
}

// ValidateRequestPricing returns the stub's fixed verdict. Parameters are unused.
func (s *stubPricingValidator) ValidateRequestPricing(*gin.Context, *meta.Meta) error {
	return s.verdict
}

// TestIsRequestPricingNotConfigured proves only tariff refusals marked at their
// source match the billing-configuration sentinel, through any wrapping, while
// other validator refusals (a retired model, missing metadata) and unrelated
// dispatch errors do not, and every refusal keeps its own client message.
func TestIsRequestPricingNotConfigured(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	tariff := RequestPricingNotConfigured(errors.New(`model "m" requires explicit positive input and output pricing`))
	require.Nil(t, RequestPricingNotConfigured(nil))

	refused := ValidateRequestPricing(&stubPricingValidator{verdict: tariff}, c, &meta.Meta{})
	require.True(t, IsRequestPricingNotConfigured(refused))
	require.True(t, IsRequestPricingNotConfigured(errors.Wrap(refused, "do request")))
	require.Contains(t, refused.Error(), "requires explicit positive input and output pricing")

	retired := ValidateRequestPricing(&stubPricingValidator{verdict: errors.New(`Groq model "x" is retired`)}, c, &meta.Meta{})
	require.Error(t, retired)
	require.False(t, IsRequestPricingNotConfigured(retired), "a retired-model refusal is not a missing tariff")
	require.Contains(t, retired.Error(), "is retired")

	require.NoError(t, ValidateRequestPricing(&stubPricingValidator{}, c, &meta.Meta{}))
	require.NoError(t, ValidateRequestPricing(&stubPlainAdaptor{}, c, &meta.Meta{}), "providers without a validator always pass")
	require.False(t, IsRequestPricingNotConfigured(nil))
	require.False(t, IsRequestPricingNotConfigured(errors.New("dial tcp: connection refused")))
}
