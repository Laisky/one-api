package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// resetRetentionMiniMax is the Groq catalog model that needs an operator
	// tariff and is therefore absent from Groq's zero-config default list.
	resetRetentionMiniMax = "minimaxai/minimax-m2.7"
	// resetRetentionValidTariff is a complete positive Groq tariff for it.
	resetRetentionValidTariff = `{"minimaxai/minimax-m2.7":{"ratio":1,"completion_ratio":2}}`
	// resetRetentionGroups are the groups every reset scenario channel serves.
	resetRetentionGroups = "default,vip"
)

// resetRetentionScenario describes one Groq channel configuration, the reset
// outcome the policy must produce for it, and the persisted model list it
// must leave behind. An empty wantCode means the reset must succeed.
type resetRetentionScenario struct {
	name            string
	models          string
	modelConfigs    *string
	modelMapping    *string
	modelRatio      *string
	wantCode        string
	wantField       string
	wantConflicting []string
	wantModels      []string
}

// resetRetentionString returns a pointer to value for nullable channel fields.
// Parameters: value is the JSON text. Returns: a pointer to a copy of value.
func resetRetentionString(value string) *string { return &value }

// groqResetDefaults returns Groq's raw adaptor catalog and that catalog minus
// the operator-priced MiniMax model, which is the zero-config default list a
// reset writes. Parameters: t is the test handle. Returns: raw and defaults.
func groqResetDefaults(t *testing.T) (raw, defaults []string) {
	t.Helper()
	raw = rawAdaptorModelList(t, channeltype.Groq)
	require.Contains(t, raw, resetRetentionMiniMax, "the fixture model must stay in Groq's catalog")
	defaults = slices.DeleteFunc(slices.Clone(raw), func(name string) bool { return name == resetRetentionMiniMax })
	require.NotEmpty(t, defaults)
	return raw, defaults
}

// resetRetentionScenarios enumerates explicitly priced excluded models, truly
// unsupported models, invalid tariffs and mappings, and an ordinary zero-config
// Groq channel. Parameters: t is the test handle. Returns: the scenarios.
func resetRetentionScenarios(t *testing.T) []resetRetentionScenario {
	t.Helper()
	_, defaults := groqResetDefaults(t)
	first := defaults[0]
	withMiniMax := append(slices.Clone(defaults), resetRetentionMiniMax)
	return []resetRetentionScenario{
		{
			name:         "listed explicitly priced excluded model is retained",
			models:       first + "," + resetRetentionMiniMax + ",retired-custom-model",
			modelConfigs: resetRetentionString(resetRetentionValidTariff),
			wantModels:   withMiniMax,
		},
		{
			name:         "explicitly priced excluded model is added like the full catalog did",
			models:       first,
			modelConfigs: resetRetentionString(resetRetentionValidTariff),
			wantModels:   withMiniMax,
		},
		{
			name:         "default mapped to a priced excluded model",
			models:       first,
			modelMapping: resetRetentionString(fmt.Sprintf(`{%q:%q}`, first, resetRetentionMiniMax)),
			modelConfigs: resetRetentionString(resetRetentionValidTariff),
			wantModels:   withMiniMax,
		},
		{
			name:         "excluded model mapped onto a default model",
			models:       first + "," + resetRetentionMiniMax,
			modelMapping: resetRetentionString(fmt.Sprintf(`{%q:"openai/gpt-oss-20b"}`, resetRetentionMiniMax)),
			wantModels:   withMiniMax,
		},
		{
			name:            "pricing for a model Groq does not recognize",
			models:          first,
			modelConfigs:    resetRetentionString(`{"not-a-groq-model":{"ratio":1,"completion_ratio":2}}`),
			wantCode:        "pricing_conflict",
			wantField:       "model_configs",
			wantConflicting: []string{"not-a-groq-model"},
		},
		{
			name:            "tariff without an output price",
			models:          first + "," + resetRetentionMiniMax,
			modelConfigs:    resetRetentionString(`{"minimaxai/minimax-m2.7":{"ratio":1}}`),
			wantCode:        "tariff_incomplete",
			wantField:       "model_configs",
			wantConflicting: []string{resetRetentionMiniMax},
		},
		{
			name:            "tariff with a zero output price",
			models:          first,
			modelConfigs:    resetRetentionString(`{"minimaxai/minimax-m2.7":{"ratio":1,"completion_ratio":0}}`),
			wantCode:        "tariff_incomplete",
			wantField:       "model_configs",
			wantConflicting: []string{resetRetentionMiniMax},
		},
		{
			name:            "legacy ratio cannot price an operator-priced model",
			models:          first + "," + resetRetentionMiniMax,
			modelRatio:      resetRetentionString(`{"minimaxai/minimax-m2.7":1}`),
			wantCode:        "tariff_incomplete",
			wantField:       "model_ratio",
			wantConflicting: []string{resetRetentionMiniMax},
		},
		{
			name:            "mapping target Groq does not recognize",
			models:          first,
			modelMapping:    resetRetentionString(fmt.Sprintf(`{%q:"not-a-groq-model"}`, first)),
			wantCode:        "mapping_conflict",
			wantField:       "model_mapping",
			wantConflicting: []string{"not-a-groq-model"},
		},
		{
			name:       "ordinary zero-config channel drops the unpriced model",
			models:     first + "," + resetRetentionMiniMax,
			wantModels: defaults,
		},
	}
}

// insertResetRetentionChannel persists a Groq channel for scenario with its
// routing abilities. Parameters: t is the test handle and scenario the
// configuration. Returns: the inserted channel.
func insertResetRetentionChannel(t *testing.T, scenario resetRetentionScenario) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type: channeltype.Groq, Name: "reset-retention", Key: "private-groq-key",
		Status: model.ChannelStatusEnabled, Group: resetRetentionGroups, Models: scenario.models,
		ModelConfigs: scenario.modelConfigs, ModelMapping: scenario.modelMapping, ModelRatio: scenario.modelRatio,
	}
	require.NoError(t, channel.Insert())
	return channel
}

// resetRetentionSnapshot returns the persisted channel row and its ability
// pairs. Parameters: t is the test handle and id the channel ID. Returns: the
// row and sorted "group/model/enabled" ability keys.
func resetRetentionSnapshot(t *testing.T, id int) (model.Channel, []string) {
	t.Helper()
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", id).Error)
	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", id).Find(&abilities).Error)
	keys := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		keys = append(keys, fmt.Sprintf("%s/%s/%t", ability.Group, ability.Model, ability.Enabled))
	}
	slices.Sort(keys)
	return stored, keys
}

// expectedResetAbilities returns the sorted enabled ability keys a channel in
// resetRetentionGroups must have for models. Parameters: models is the
// persisted list. Returns: sorted "group/model/true" keys.
func expectedResetAbilities(models []string) []string {
	keys := make([]string, 0, 2*len(models))
	for _, group := range strings.Split(resetRetentionGroups, ",") {
		for _, name := range models {
			keys = append(keys, group+"/"+name+"/true")
		}
	}
	slices.Sort(keys)
	return keys
}

// requireResetRetentionOutcome asserts the persisted state after a reset
// attempt: success writes exactly wantModels with matching abilities and
// byte-identical overrides, while a refusal leaves the row and abilities as
// they were. Parameters: t is the test handle, scenario the expectation, id
// the channel, and before/beforeAbilities the pre-reset snapshot. Returns: none.
func requireResetRetentionOutcome(t *testing.T, scenario resetRetentionScenario, id int, before model.Channel, beforeAbilities []string) {
	t.Helper()
	after, afterAbilities := resetRetentionSnapshot(t, id)
	if scenario.wantCode != "" {
		require.Equal(t, before, after, "a refused reset must not change the channel")
		require.Equal(t, beforeAbilities, afterAbilities, "a refused reset must not change routing")
		return
	}
	require.Equal(t, scenario.wantModels, after.GetSupportedModelNames())
	require.Equal(t, before.ModelConfigs, after.ModelConfigs, "a reset never rewrites pricing")
	require.Equal(t, before.ModelMapping, after.ModelMapping, "a reset never rewrites mappings")
	require.Equal(t, before.ModelRatio, after.ModelRatio, "a reset never rewrites legacy pricing")
	require.Equal(t, before.Key, after.Key)
	require.Nil(t, after.HiddenModels)
	require.Equal(t, expectedResetAbilities(scenario.wantModels), afterAbilities)
}

// TestResetChannelModelsRetainsExplicitlyServableModels drives the single
// channel reset handler through sqlite for every retention scenario.
// Parameters: t is the test handle. Returns: none.
func TestResetChannelModelsRetainsExplicitlyServableModels(t *testing.T) {
	for _, scenario := range resetRetentionScenarios(t) {
		t.Run(scenario.name, func(t *testing.T) {
			_, cleanup := setupUUIDContractTestEnvironment(t)
			t.Cleanup(cleanup)
			channel := insertResetRetentionChannel(t, scenario)
			before, beforeAbilities := resetRetentionSnapshot(t, channel.Id)

			recorder := httptest.NewRecorder()
			channelResetTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/"+channel.UUID+"/reset_models", nil))
			var payload struct {
				Success  bool                             `json:"success"`
				Message  string                           `json:"message"`
				Conflict *model.ChannelModelResetConflict `json:"conflict"`
				Data     channelModelResetOutcome         `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
			require.NotContains(t, recorder.Body.String(), "private-groq-key")
			if scenario.wantCode == "" {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.True(t, payload.Success)
				require.Equal(t, len(scenario.wantModels), payload.Data.ModelCount)
			} else {
				require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
				require.False(t, payload.Success)
				require.NotNil(t, payload.Conflict)
				require.Equal(t, scenario.wantCode, payload.Conflict.Code)
				require.Equal(t, scenario.wantField, payload.Conflict.Field)
				require.Equal(t, scenario.wantConflicting, payload.Conflict.Models)
				require.Equal(t, payload.Conflict.Error(), payload.Message)
			}
			requireResetRetentionOutcome(t, scenario, channel.Id, before, beforeAbilities)
		})
	}
}

// TestResetSelectedChannelModelsRetainsExplicitlyServableModels drives the
// bulk reset handler through sqlite: first one channel per scenario, then all
// scenario channels in one request so refusals stay isolated per channel.
// Parameters: t is the test handle. Returns: none.
func TestResetSelectedChannelModelsRetainsExplicitlyServableModels(t *testing.T) {
	scenarios := resetRetentionScenarios(t)
	// bulkReset posts one explicit UUID selection. Parameters: uuids are the
	// selected channels. Returns: the decoded summary.
	bulkReset := func(t *testing.T, uuids []string) channelModelResetSummary {
		t.Helper()
		body, err := json.Marshal(map[string]any{"selection": map[string]any{"mode": "ids", "ids": uuids}})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		channelResetTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/reset_models", strings.NewReader(string(body))))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "private-groq-key")
		var payload struct {
			Success bool                     `json:"success"`
			Data    channelModelResetSummary `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		require.True(t, payload.Success)
		require.Len(t, payload.Data.Results, len(uuids))
		return payload.Data
	}
	// requireResult asserts one channel's bulk outcome. Parameters: scenario is
	// the expectation and result the reported outcome. Returns: none.
	requireResult := func(t *testing.T, scenario resetRetentionScenario, result channelModelResetOutcome) {
		t.Helper()
		if scenario.wantCode == "" {
			require.True(t, result.Success, result.Message)
			require.Nil(t, result.Conflict)
			require.Equal(t, len(scenario.wantModels), result.ModelCount)
			return
		}
		require.False(t, result.Success)
		require.NotNil(t, result.Conflict, result.Message)
		require.Equal(t, scenario.wantCode, result.Conflict.Code)
		require.Equal(t, scenario.wantField, result.Conflict.Field)
		require.Equal(t, scenario.wantConflicting, result.Conflict.Models)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, cleanup := setupUUIDContractTestEnvironment(t)
			t.Cleanup(cleanup)
			channel := insertResetRetentionChannel(t, scenario)
			before, beforeAbilities := resetRetentionSnapshot(t, channel.Id)
			summary := bulkReset(t, []string{channel.UUID})
			require.Equal(t, 1, summary.Total)
			require.Zero(t, summary.Failed)
			requireResult(t, scenario, summary.Results[0])
			requireResetRetentionOutcome(t, scenario, channel.Id, before, beforeAbilities)
		})
	}

	t.Run("mixed selection", func(t *testing.T) {
		_, cleanup := setupUUIDContractTestEnvironment(t)
		t.Cleanup(cleanup)
		ids := make([]int, len(scenarios))
		uuids := make([]string, len(scenarios))
		befores := make([]model.Channel, len(scenarios))
		beforeAbilities := make([][]string, len(scenarios))
		wantRejected := 0
		for i, scenario := range scenarios {
			channel := insertResetRetentionChannel(t, scenario)
			ids[i], uuids[i] = channel.Id, channel.UUID
			befores[i], beforeAbilities[i] = resetRetentionSnapshot(t, channel.Id)
			if scenario.wantCode != "" {
				wantRejected++
			}
		}
		summary := bulkReset(t, uuids)
		require.Equal(t, len(scenarios), summary.Total)
		require.Equal(t, wantRejected, summary.Rejected)
		require.Equal(t, len(scenarios)-wantRejected, summary.Reset)
		require.Zero(t, summary.Failed)
		for i, scenario := range scenarios {
			require.Equal(t, uuids[i], summary.Results[i].UUID)
			requireResult(t, scenario, summary.Results[i])
			requireResetRetentionOutcome(t, scenario, ids[i], befores[i], beforeAbilities[i])
		}
	})
}
