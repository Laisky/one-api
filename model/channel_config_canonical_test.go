package model

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// repeatedOverrideConfig repeats api_version under two spellings and an exact
// duplicate; encoding/json applies them in document order, so "c" is effective.
const repeatedOverrideConfig = `{"api_version":"a","region":"r","API_VERSION":"b","api_version":"c"}`

// canonicalOverrideConfig is repeatedOverrideConfig collapsed into one member
// under the canonical key at the first spelling's position.
const canonicalOverrideConfig = `{"api_version":"c","region":"r"}`

// newCanonicalTestChannel returns an Azure channel holding config for the
// persistence tests. Parameters: name labels the row and config is its raw
// config text. Returns: the unsaved channel.
func newCanonicalTestChannel(name, config string) Channel {
	return Channel{
		UUID: uuid.NewString(), Name: name, Type: 3, Status: ChannelStatusEnabled,
		Models: "gpt-4o", Group: "default", Config: config,
	}
}

// storedChannelConfig reads the raw config column of the channel with id.
// Parameters: t owns assertions and id names the row. Returns: the column text.
func storedChannelConfig(t *testing.T, id int) string {
	t.Helper()
	var stored Channel
	require.NoError(t, DB.Where("id = ?", id).Take(&stored).Error)
	return stored.Config
}

// TestChannelWritesCanonicalizeRepeatedConfigKeys verifies that create, batch
// create and update store one member per config field holding the value
// LoadConfig decodes, and leave a config without repeated keys byte-identical.
func TestChannelWritesCanonicalizeRepeatedConfigKeys(t *testing.T) {
	useChannelPersistenceTestDB(t)
	effective, err := (&Channel{Config: repeatedOverrideConfig}).LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "c", effective.APIVersion, "encoding/json applies the last spelling")

	single := newCanonicalTestChannel("canonical-insert", repeatedOverrideConfig)
	require.NoError(t, single.Insert())
	require.Equal(t, canonicalOverrideConfig, storedChannelConfig(t, single.Id), "Insert")

	batch := []Channel{
		newCanonicalTestChannel("canonical-batch-repeated", repeatedOverrideConfig),
		newCanonicalTestChannel("canonical-batch-plain", `{ "API_VERSION" : "x",  "region":"r" }`),
	}
	require.NoError(t, BatchInsertChannels(batch))
	require.Equal(t, canonicalOverrideConfig, storedChannelConfig(t, batch[0].Id), "BatchInsertChannels")
	require.Equal(t, `{ "API_VERSION" : "x",  "region":"r" }`, storedChannelConfig(t, batch[1].Id), "a config without repeats is stored byte for byte")

	update := newCanonicalTestChannel("canonical-update", `{"region":"r"}`)
	require.NoError(t, update.Insert())
	update.Config = repeatedOverrideConfig
	require.NoError(t, update.UpdateWithContext(context.Background()))
	require.Equal(t, canonicalOverrideConfig, storedChannelConfig(t, update.Id), "UpdateWithContext")

	for _, id := range []int{single.Id, batch[0].Id, update.Id} {
		cfg, err := (&Channel{Config: storedChannelConfig(t, id)}).LoadConfig()
		require.NoError(t, err)
		require.Equal(t, effective, cfg, "canonicalization keeps the decoded configuration")
	}
}

// TestCanonicalChannelConfig pins the canonical rewrite: repeated members of a
// field collapse into its json name at the first member's position holding
// the value encoding/json decodes, other members keep their bytes and order,
// and text without repeats is returned unchanged.
func TestCanonicalChannelConfig(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"empty", ``, ``},
		{"invalid json", `{"api_version":`, `{"api_version":`},
		{"array", `["api_version"]`, `["api_version"]`},
		{"null", `null`, `null`},
		{"no repeats keeps bytes", "{\n  \"API_VERSION\" : \"a\",\n  \"region\":\"r\"\n}", "{\n  \"API_VERSION\" : \"a\",\n  \"region\":\"r\"\n}"},
		{"repeated unknown keys stay", `{"x":1,"x":2,"X":3}`, `{"x":1,"x":2,"X":3}`},
		{"underscore is significant", `{"api_version":"a","APIVERSION":"b","api-version":"c"}`, `{"api_version":"a","APIVERSION":"b","api-version":"c"}`},
		{"exact duplicate", `{"api_version":"a","api_version":"b"}`, `{"api_version":"b"}`},
		{"mixed spellings and exact duplicate", `{"api_version":"a","API_VERSION":"b","api_version":"c"}`, `{"api_version":"c"}`},
		{"canonical key at first position", `{"API_VERSION":"a","region":"r","Api_Version":"b"}`, `{"api_version":"b","region":"r"}`},
		{"formatting of kept members", "{\n  \"API_VERSION\": \"a\",\n  \"region\": \"r\",\n  \"Api_Version\": \"b\"\n}", "{\n  \"api_version\":\"b\",\n  \"region\": \"r\"\n}"},
		{"dropped last member", `{"a":1, "api_version":"x" , "API_VERSION":"y" }`, `{"a":1, "api_version":"y" }`},
		{"surrounding whitespace", "  {\"api_version\":\"a\",\"api_version\":\"b\"}  \n", "  {\"api_version\":\"b\"}  \n"},
		{"escaped key", `{"\u0061pi_version":"a","api_version":"b"}`, `{"api_version":"b"}`},
		{"kelvin sign folds to k", `{"ak":"x","a\u212a":"y"}`, `{"ak":"y"}`},
		{"long s folds to s", `{"api_version":"x","api_ver` + "\u017f" + `ion":"y"}`, `{"api_version":"y"}`},
		{"empty last string", `{"api_version":"a","API_VERSION":""}`, `{"api_version":""}`},
		{"null is skipped", `{"api_version":"a","API_VERSION":null}`, `{"api_version":"a"}`},
		{"only nulls", `{"api_version":null,"API_VERSION":null}`, `{"api_version":""}`},
		{"kind mismatch is skipped", `{"api_version":"a","API_VERSION":5}`, `{"api_version":"a"}`},
		{"no html escaping", `{"api_version":"<a>","API_VERSION":"x&y"}`, `{"api_version":"x&y"}`},
		{"maps merge", `{"custom_headers":{"A":"1"},"CUSTOM_HEADERS":{"B":"2","A":"3"}}`, `{"custom_headers":{"A":"3","B":"2"}}`},
		{"slices replace", `{"supported_endpoints":["a","b"],"Supported_Endpoints":["c"]}`, `{"supported_endpoints":["c"]}`},
		{"structs merge", `{"tooling":{"whitelist":["x"]},"TOOLING":{"pricing":{"web":{"usd_per_call":0.5}}}}`, `{"tooling":{"whitelist":["x"],"pricing":{"web":{"usd_per_call":0.5}}}}`},
		{"several fields keep unrelated order", `{"z":1,"plugin":"p","api_version":"a","PLUGIN":"q","x":2,"API_VERSION":"b"}`, `{"z":1,"plugin":"q","api_version":"b","x":2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canonicalChannelConfig(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			members, ok := scanConfigMembers(tc.raw)
			if !ok {
				return
			}
			// The relay decodes the same configuration before and after the rewrite.
			before, beforeErr := decodeChannelConfig(tc.raw)
			after, afterErr := decodeChannelConfig(got)
			require.Equal(t, before, after, "the decoded configuration is unchanged")
			if beforeErr == nil {
				require.NoError(t, afterErr)
			}
			// At most one member per field remains, and a rewrite happened only for a repeat.
			seen := map[int]int{}
			repeated := false
			for _, member := range members {
				if field := channelConfigFieldFor(member.key); field >= 0 {
					seen[field]++
					repeated = repeated || seen[field] > 1
				}
			}
			require.Equal(t, repeated, got != tc.raw, "only a repeated field triggers a rewrite")
			rewritten, ok := scanConfigMembers(got)
			require.True(t, ok)
			kept := map[int]int{}
			for _, member := range rewritten {
				if field := channelConfigFieldFor(member.key); field >= 0 {
					kept[field]++
					require.LessOrEqual(t, kept[field], 1, "field %q keeps one member", channelConfigFieldNames[field])
				}
			}
		})
	}
}

// TestChannelConfigFieldForMatchesEncodingJSON proves the key-to-field mapping
// agrees with encoding/json for tricky spellings of every string field: case
// changes, Unicode simple folds (Kelvin sign, long s), lookalikes that do not
// fold (dotless i, dotted capital I), and dropped or replaced underscores.
func TestChannelConfigFieldForMatchesEncodingJSON(t *testing.T) {
	configType := reflect.TypeFor[ChannelConfig]()
	for index, name := range channelConfigFieldNames {
		if name == "" || configType.Field(index).Type.Kind() != reflect.String {
			continue
		}
		variants := []string{
			name, strings.ToUpper(name), strings.ToUpper(name[:1]) + name[1:],
			strings.ReplaceAll(name, "k", "\u212a"), strings.ReplaceAll(name, "s", "\u017f"),
			strings.ReplaceAll(name, "i", "\u0131"), strings.ReplaceAll(name, "i", "\u0130"),
			strings.ReplaceAll(name, "_", ""), strings.ReplaceAll(name, "_", "-"),
		}
		for _, variant := range variants {
			raw, err := json.Marshal(map[string]string{variant: "sentinel"})
			require.NoError(t, err)
			var cfg ChannelConfig
			require.NoError(t, json.Unmarshal(raw, &cfg))
			decoded := reflect.ValueOf(cfg).Field(index).String() == "sentinel"
			require.Equal(t, decoded, channelConfigFieldFor(variant) == index, "field %q, key %q", name, variant)
		}
	}
}

// TestChannelConfigFieldNamesAreUnambiguous guards the assumptions of the
// mapping: ChannelConfig embeds no struct (promoted fields would need their
// own lookup), and no two json names fold together.
func TestChannelConfigFieldNamesAreUnambiguous(t *testing.T) {
	configType := reflect.TypeFor[ChannelConfig]()
	for i := range configType.NumField() {
		require.False(t, configType.Field(i).Anonymous, "ChannelConfig.%s must not be embedded", configType.Field(i).Name)
	}
	for i, a := range channelConfigFieldNames {
		for j, b := range channelConfigFieldNames {
			if i != j && a != "" && b != "" {
				require.False(t, strings.EqualFold(a, b), "json names %q and %q fold together", a, b)
			}
		}
	}
}
