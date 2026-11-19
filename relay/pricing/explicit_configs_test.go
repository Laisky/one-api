package pricing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
)

// explicitTariffCases are channel model_configs documents for the model
// "operator-model", each with whether dispatch must admit it at
// explicitTariffAt.
var explicitTariffCases = []struct {
	name    string
	configs map[string]model.ModelConfigLocal
	valid   bool
}{
	{name: "no_configs"},
	{name: "other_model_only", configs: map[string]model.ModelConfigLocal{"other": {Ratio: 1, CompletionRatio: 2}}},
	{name: "complete", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1, CompletionRatio: 2}}, valid: true},
	{name: "input_only", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1}}},
	{name: "output_only", configs: map[string]model.ModelConfigLocal{"operator-model": {CompletionRatio: 2}}},
	{name: "metadata_only", configs: map[string]model.ModelConfigLocal{"operator-model": {MaxTokens: 100}}},
	{name: "negative_input", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: -1, CompletionRatio: 2}}},
	{name: "incomplete_tier", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1, CompletionRatio: 2,
		Tiers: []model.ModelRatioTierLocal{{Ratio: 1, InputTokenThreshold: 1000}}}}},
	{name: "complete_tier", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1, CompletionRatio: 2,
		Tiers: []model.ModelRatioTierLocal{{Ratio: 2, CompletionRatio: 2, InputTokenThreshold: 1000}}}}, valid: true},
	// The active window supplies the missing output price only while it applies.
	{name: "active_window_completes", valid: true, configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1,
		TimeWindows: []model.TimeWindowLocal{{TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "11:00", End: "13:00"}},
			Overlay: model.ModelConfigLocal{CompletionRatio: 3}}}}}},
	{name: "inactive_window_does_not", configs: map[string]model.ModelConfigLocal{"operator-model": {Ratio: 1,
		TimeWindows: []model.TimeWindowLocal{{TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "01:00", End: "02:00"}},
			Overlay: model.ModelConfigLocal{CompletionRatio: 3}}}}}},
}

// explicitTariffAt is the request start time every case is evaluated at.
var explicitTariffAt = time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)

// TestValidateExplicitTokenTariffConfigsMatchesDispatch is a differential test:
// for every saved channel configuration, the request-free check the channel
// editor uses must return exactly the verdict and message of
// RequireExplicitTokenPricing, which dispatch runs against the saved row.
func TestValidateExplicitTokenTariffConfigsMatchesDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range explicitTariffCases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Id: 1}
			if tc.configs != nil {
				encoded, err := json.Marshal(tc.configs)
				require.NoError(t, err)
				raw := string(encoded)
				channel.ModelConfigs = &raw
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set(ctxkey.ChannelModel, channel)
			dispatchErr := RequireExplicitTokenPricing(c, &meta.Meta{ActualModelName: "operator-model", StartTime: explicitTariffAt})

			parsed, err := model.ParseModelPriceConfigs(channel.ModelConfigs)
			require.NoError(t, err)
			draftErr := ValidateExplicitTokenTariffConfigs("operator-model", parsed, explicitTariffAt)

			if tc.valid {
				require.NoError(t, dispatchErr)
				require.NoError(t, draftErr)
				return
			}
			require.Error(t, dispatchErr)
			require.Error(t, draftErr)
			require.True(t, adaptor.IsRequestPricingNotConfigured(draftErr))
			require.Equal(t, dispatchErr.Error(), draftErr.Error())
			require.Contains(t, draftErr.Error(), `model "operator-model" requires explicit positive input and output pricing in channel model_configs`)
		})
	}
}
