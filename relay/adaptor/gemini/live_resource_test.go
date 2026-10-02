package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVertexLiveSetupPinsResource tests alias and fully qualified setup names
// against the same authenticated resource. Parameters: t owns the test. Returns:
// none. Project, location, and model switches remain rejected.
func TestVertexLiveSetupPinsResource(t *testing.T) {
	t.Parallel()
	actual := "gemini-3.8-live"
	resource := "projects/owned/locations/europe-west4/publishers/google/models/" + actual
	for _, name := range []string{"", "friendly", "models/friendly", actual, "models/" + actual, resource} {
		t.Run("accept/"+name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"setup": map[string]any{"model": name}})
			require.NoError(t, err)
			frame, err := prepareLiveSetupForResource(data, actual, "friendly", resource)
			require.NoError(t, err)
			var envelope struct {
				Setup struct {
					Model string `json:"model"`
				} `json:"setup"`
			}
			require.NoError(t, json.Unmarshal(frame, &envelope))
			require.Equal(t, resource, envelope.Setup.Model)
		})
	}
	for _, name := range []string{
		"other-model", "projects/other/locations/europe-west4/publishers/google/models/" + actual,
		"projects/owned/locations/global/publishers/google/models/" + actual,
		resource + "-other", "../" + actual,
	} {
		data, err := json.Marshal(map[string]any{"setup": map[string]any{"model": name}})
		require.NoError(t, err)
		_, err = prepareLiveSetupForResource(data, actual, "friendly", resource)
		require.ErrorIs(t, err, ErrLiveProtocol)
	}
}

// TestConfiguredLiveModelDoesNotInheritAnotherModelsRestrictions verifies that
// an unlisted native model can retain TEXT and provider thinking settings while
// known 3.8 restrictions and local session ownership remain enforced. Parameters:
// t owns the test. Returns: none.
func TestConfiguredLiveModelDoesNotInheritAnotherModelsRestrictions(t *testing.T) {
	t.Parallel()
	frame, err := prepareLiveSetup([]byte(`{"setup":{"generationConfig":{"responseModalities":["TEXT"],"thinkingConfig":{"thinkingBudget":128}}}}`), "configured-native-model", "alias")
	require.NoError(t, err)
	require.Contains(t, string(frame), `"thinkingBudget":128`)
	_, err = prepareLiveSetup([]byte(`{"setup":{"generationConfig":{"responseModalities":["TEXT"]}}}`), "gemini-3.8-live", "alias")
	require.Error(t, err)
	_, err = prepareLiveSetup([]byte(`{"setup":{"sessionResumption":{"handle":"other-session"}}}`), "configured-native-model", "alias")
	require.Error(t, err)
	for _, name := range []string{"", "..", "../model", "model?key=value", "model#fragment", "model%2Fother", "model\nheader"} {
		require.Error(t, ValidateLiveModelName(name), name)
	}
	require.NoError(t, ValidateLiveModelName("operator-configured-live-id"))
}
