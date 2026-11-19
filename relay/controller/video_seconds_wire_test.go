package controller

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestVideoSecondsWireShadowsEveryDurationAlias pins videoRequestWire to the
// shared DTO: each shadowing field must carry exactly the json and form tags
// of the DTO field it replaces (otherwise it would stop shadowing and the alias
// would be decoded twice), and every numeric DTO duration alias must be
// shadowed so no alias escapes string decoding and the conflict check.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoSecondsWireShadowsEveryDurationAlias(t *testing.T) {
	wire := reflect.TypeFor[videoRequestWire]()
	dto := reflect.TypeFor[relaymodel.VideoRequest]()
	shadowed := map[string]bool{}
	for field := range wire.Fields() {
		if field.Anonymous {
			require.Equal(t, dto, field.Type, "the wire embeds the shared DTO")
			continue
		}
		original, ok := dto.FieldByName(field.Name)
		require.True(t, ok, "wire field %s must shadow a DTO field", field.Name)
		require.Equal(t, original.Tag.Get("json"), field.Tag.Get("json"), field.Name)
		require.Equal(t, original.Tag.Get("form"), field.Tag.Get("form"), field.Name)
		require.Equal(t, reflect.TypeFor[*videoDuration](), field.Type, field.Name)
		shadowed[field.Name] = true
	}
	for field := range dto.Fields() {
		if field.Type == reflect.TypeFor[*float64]() {
			require.True(t, shadowed[field.Name], "numeric DTO alias %s must accept the string form", field.Name)
		}
	}
	require.Len(t, shadowed, 3)
}
