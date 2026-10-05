package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// perPageDisplayResult carries both the decoded display entry and its raw JSON
// object so tests can assert on wire field names as well as typed values.
type perPageDisplayResult struct {
	info ModelDisplayInfo
	raw  map[string]json.RawMessage
}

// fetchPerPageDisplayModel requests /api/models/display through the real handler
// and returns the entry for advertised under channel ch.
// Parameters: t is the test handle, ch is the persisted channel, advertised is
// the listed model name, and userID is zero for anonymous or a user id otherwise.
// Returns: the decoded entry plus its raw JSON fields.
func fetchPerPageDisplayModel(t *testing.T, ch *model.Channel, advertised string, userID int) perPageDisplayResult {
	t.Helper()
	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		if userID != 0 {
			c.Set(ctxkey.Id, userID)
		}
		GetModelsDisplay(c)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/display", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var response ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	key := channeltype.IdToName(ch.Type) + ":" + ch.Name
	require.Contains(t, response.Data, key)
	require.Contains(t, response.Data[key].Models, advertised)

	var rawResponse struct {
		Data map[string]struct {
			Models map[string]map[string]json.RawMessage `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rawResponse))
	require.Contains(t, rawResponse.Data, key)
	require.Contains(t, rawResponse.Data[key].Models, advertised)

	return perPageDisplayResult{info: response.Data[key].Models[advertised], raw: rawResponse.Data[key].Models[advertised]}
}

// TestGetModelsDisplay_PerPagePricingFromChannelConfig verifies that a
// channel-local per_page tariff is surfaced as per_page_pricing on both the
// anonymous and authenticated display endpoints, that an explicit zero tariff
// stays visible as free rather than disappearing, and that page-priced models
// never advertise token or per-call prices.
func TestGetModelsDisplay_PerPagePricingFromChannelConfig(t *testing.T) {
	for _, tc := range []struct {
		name, modelName string
		usdPerThousand  float64
		custom          bool
	}{
		{name: "paid_override_only_model", modelName: "custom-ocr", usdPerThousand: 5, custom: true},
		{name: "free_override_only_model", modelName: "custom-ocr-free", usdPerThousand: 0, custom: true},
		{name: "token_model_repriced_per_page", modelName: "gpt-4o", usdPerThousand: 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupModelsDisplayTestEnv(t)
			gin.SetMode(gin.TestMode)

			group := fmt.Sprintf("per-page-%s-%d", tc.name, time.Now().UnixNano())
			ch := &model.Channel{Name: tc.name, Type: channeltype.OpenAI, Status: model.ChannelStatusEnabled, Models: tc.modelName, Group: group}
			if tc.custom {
				ch.Models = ""
			}
			local := map[string]model.ModelConfigLocal{
				tc.modelName: {PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: tc.usdPerThousand}},
			}
			require.NoError(t, ch.SetModelPriceConfigs(local))
			require.NoError(t, model.DB.Create(ch).Error)
			user := &model.User{Username: "per-page-display", Password: "fixture", Group: group, Status: model.UserStatusEnabled}
			require.NoError(t, model.DB.Create(user).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: tc.modelName, ChannelId: ch.Id, Enabled: true}).Error)

			provider := relay.GetAdaptor(channeltype.ToAPIType(ch.Type))
			provider.Init(&metalib.Meta{ChannelType: ch.Type})
			resolved, ok := pricing.ResolveModelConfig(tc.modelName, local, provider, time.Now())
			require.True(t, ok)
			require.NotNil(t, resolved.PerPage, "billing resolver must carry the per-page tariff")

			for _, loggedIn := range []bool{false, true} {
				// Config-only entries are anonymous discovery only; authenticated
				// listings still obey the channel's Models list.
				if tc.custom && loggedIn {
					continue
				}
				t.Run(fmt.Sprintf("authenticated_%v", loggedIn), func(t *testing.T) {
					userID := 0
					if loggedIn {
						userID = user.Id
					}
					got := fetchPerPageDisplayModel(t, ch, tc.modelName, userID)

					require.Equal(t, buildPerPageDisplayPricing(resolved.PerPage), got.info.PerPagePricing)
					require.NotNil(t, got.info.PerPagePricing)
					require.InDelta(t, tc.usdPerThousand, got.info.PerPagePricing.UsdPerThousandPages, 1e-12)
					require.InDelta(t, tc.usdPerThousand/1000, got.info.PerPagePricing.UsdPerPage, 1e-12)
					require.Nil(t, got.info.PerCallPricing)
					require.Zero(t, got.info.InputPrice)
					require.Zero(t, got.info.OutputPrice)
					require.Zero(t, got.info.CachedInputPrice)
					require.Zero(t, got.info.CacheWrite5mPrice)
					require.Zero(t, got.info.CacheWrite1hPrice)
					require.Empty(t, got.info.Tiers)

					require.Contains(t, got.raw, "per_page_pricing")
					require.NotContains(t, got.raw, "per_call_pricing")
					require.JSONEq(t,
						fmt.Sprintf(`{"usd_per_thousand_pages":%v,"usd_per_page":%v}`, tc.usdPerThousand, tc.usdPerThousand/1000),
						string(got.raw["per_page_pricing"]))
				})
			}
		})
	}
}

// TestGetModelsDisplay_PerPagePricingTimeWindowOverlay verifies that per-page
// tariffs inside channel-local time-window overlays are rendered on each
// window's overlay, including an explicit free window that must serialize its
// zero prices instead of being dropped.
func TestGetModelsDisplay_PerPagePricingTimeWindowOverlay(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)

	const modelName = "custom-ocr-windowed"
	ch := &model.Channel{Name: "per-page-windows", Type: channeltype.OpenAI, Status: model.ChannelStatusEnabled, Group: "public"}
	local := map[string]model.ModelConfigLocal{
		modelName: {
			PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 10},
			TimeWindows: []model.TimeWindowLocal{
				{
					Name:     "off-peak",
					TimeZone: "UTC",
					Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "08:00"}},
					Overlay:  model.ModelConfigLocal{PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 4}},
				},
				{
					Name:       "promo",
					TimeZone:   "UTC",
					Ranges:     []model.ClockRangeLocal{{Start: "08:00", End: "09:00"}},
					DaysOfWeek: []int{0},
					Overlay:    model.ModelConfigLocal{PerPage: &model.PerPagePricingLocal{}},
				},
			},
		},
	}
	require.NoError(t, ch.SetModelPriceConfigs(local))
	require.NoError(t, model.DB.Create(ch).Error)

	got := fetchPerPageDisplayModel(t, ch, modelName, 0)
	require.Equal(t, &PerPageDisplayPricing{UsdPerThousandPages: 10, UsdPerPage: 0.01}, got.info.PerPagePricing)
	require.Zero(t, got.info.InputPrice)
	require.Zero(t, got.info.OutputPrice)

	require.Len(t, got.info.TimeWindows, 2)
	require.Equal(t, "off-peak", got.info.TimeWindows[0].Name)
	require.Equal(t, &PerPageDisplayPricing{UsdPerThousandPages: 4, UsdPerPage: 0.004}, got.info.TimeWindows[0].Overlay.PerPagePricing)
	require.Nil(t, got.info.TimeWindows[0].Overlay.PerCallPricing)
	require.Equal(t, "promo", got.info.TimeWindows[1].Name)
	require.Equal(t, &PerPageDisplayPricing{}, got.info.TimeWindows[1].Overlay.PerPagePricing)

	var windows []struct {
		Overlay map[string]json.RawMessage `json:"overlay"`
	}
	require.NoError(t, json.Unmarshal(got.raw["time_windows"], &windows))
	require.Len(t, windows, 2)
	require.JSONEq(t, `{"usd_per_thousand_pages":4,"usd_per_page":0.004}`, string(windows[0].Overlay["per_page_pricing"]))
	require.JSONEq(t, `{"usd_per_thousand_pages":0,"usd_per_page":0}`, string(windows[1].Overlay["per_page_pricing"]))
}

// TestBuildPerPageDisplayPricing verifies nil, free, and paid per-page tariffs
// convert into the expected display values.
func TestBuildPerPageDisplayPricing(t *testing.T) {
	require.Nil(t, buildPerPageDisplayPricing(nil))
	require.Equal(t, &PerPageDisplayPricing{}, buildPerPageDisplayPricing(&adaptorpkg.PerPagePricingConfig{}))
	require.Equal(t,
		&PerPageDisplayPricing{UsdPerThousandPages: 3.5, UsdPerPage: 0.0035},
		buildPerPageDisplayPricing(&adaptorpkg.PerPagePricingConfig{UsdPerThousandPages: 3.5}))
}

// TestPerPageFreeWindowOverlayJSON verifies that a free per-page window overlay
// keeps its zero fields on the wire so it is distinguishable from no override.
func TestPerPageFreeWindowOverlayJSON(t *testing.T) {
	result := buildTimeWindowOverlayDisplayWithBase(adaptorpkg.ModelConfig{PerPage: &adaptorpkg.PerPagePricingConfig{}}, 0, 0, func(v float64) float64 { return v })
	body, err := json.Marshal(result)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	require.JSONEq(t, `{"usd_per_thousand_pages":0,"usd_per_page":0}`, string(fields["per_page_pricing"]))
	require.NotContains(t, fields, "per_call_pricing")
}

// TestConvertLocalDisplayConfigPerPage verifies that channel-local per-page
// tariffs, including those nested in time-window overlays, survive conversion
// into the adaptor-shaped display config.
func TestConvertLocalDisplayConfigPerPage(t *testing.T) {
	require.Nil(t, convertLocalDisplayConfig(model.ModelConfigLocal{Ratio: 1}).PerPage)

	converted := convertLocalDisplayConfig(model.ModelConfigLocal{PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 7}})
	require.Equal(t, &adaptorpkg.PerPagePricingConfig{UsdPerThousandPages: 7}, converted.PerPage)
	require.Nil(t, converted.PerCall)

	windows := convertLocalDisplayTimeWindows([]model.TimeWindowLocal{{
		Ranges:  []model.ClockRangeLocal{{Start: "00:00", End: "01:00"}},
		Overlay: model.ModelConfigLocal{PerPage: &model.PerPagePricingLocal{}},
	}})
	require.Len(t, windows, 1)
	require.Equal(t, &adaptorpkg.PerPagePricingConfig{}, windows[0].Overlay.PerPage)
}

// TestApplyUnitTariffDisplayOverride verifies that an explicit local tariff of
// one flat unit hides an inherited tariff of the other unit, and that a local
// override without per_page hides an inherited page tariff, matching billing.
func TestApplyUnitTariffDisplayOverride(t *testing.T) {
	inheritedCall := &PerCallDisplayPricing{UsdPerThousandCalls: 2, UsdPerCall: 0.002}
	inheritedPage := &PerPageDisplayPricing{UsdPerThousandPages: 9, UsdPerPage: 0.009}

	call, page, drop := applyUnitTariffDisplayOverride(adaptorpkg.ModelConfig{PerPage: &adaptorpkg.PerPagePricingConfig{UsdPerThousandPages: 1}}, inheritedCall, inheritedPage)
	require.Nil(t, call)
	require.Equal(t, &PerPageDisplayPricing{UsdPerThousandPages: 1, UsdPerPage: 0.001}, page)
	require.True(t, drop, "an inherited page schedule never applies to a local page tariff")

	call, page, drop = applyUnitTariffDisplayOverride(adaptorpkg.ModelConfig{PerCall: &adaptorpkg.PerCallPricingConfig{UsdPerThousandCalls: 4}}, inheritedCall, inheritedPage)
	require.Same(t, inheritedCall, call)
	require.Nil(t, page)
	require.True(t, drop)

	call, page, drop = applyUnitTariffDisplayOverride(adaptorpkg.ModelConfig{Ratio: 2}, inheritedCall, inheritedPage)
	require.Same(t, inheritedCall, call)
	require.Nil(t, page, "a local override without per_page bills by tokens, so the inherited page tariff is hidden")
	require.True(t, drop, "the inherited page-price schedule must not be advertised for a local token tariff")

	call, page, drop = applyUnitTariffDisplayOverride(adaptorpkg.ModelConfig{Ratio: 2}, inheritedCall, nil)
	require.Same(t, inheritedCall, call)
	require.Nil(t, page)
	require.False(t, drop, "without an inherited page tariff the existing schedule rules are unchanged")
}
