package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Channel configs are stored as raw JSON text and decoded by LoadConfig with
// encoding/json, which matches an object key to a ChannelConfig field by exact
// name first and otherwise under strings.EqualFold (underscores and dashes stay
// significant), accepts repeated keys, and applies them in document order. A
// config that spells one field several times therefore means only what the
// decoder leaves behind, while editors and other readers that collapse
// repeated keys (JavaScript's JSON.parse keeps the first position with the last
// value) can see a different value. Every channel write stores one member per
// field instead: the canonical json name, at the position of the field's first
// member, holding the value LoadConfig decodes.

// channelConfigFieldNames lists the json name of every ChannelConfig field by
// field index; an empty name marks a field encoding/json never decodes.
var channelConfigFieldNames = func() []string {
	t := reflect.TypeFor[ChannelConfig]()
	names := make([]string, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch name {
		case "-":
			name = ""
		case "":
			name = field.Name
		}
		names[i] = name
	}
	return names
}()

// channelConfigFieldFor returns the index of the ChannelConfig field that
// encoding/json decodes the object key into, preferring an exact name match
// over a strings.EqualFold match. Parameters: key is the decoded object key.
// Returns: the field index, or -1 for a key no field decodes.
func channelConfigFieldFor(key string) int {
	for i, name := range channelConfigFieldNames {
		if name != "" && name == key {
			return i
		}
	}
	for i, name := range channelConfigFieldNames {
		if name != "" && strings.EqualFold(name, key) {
			return i
		}
	}
	return -1
}

// configMember is one top-level member of a JSON object text: the byte span
// of `"key": value` and the decoded key.
type configMember struct {
	start, end int
	key        string
}

// scanConfigMembers lists the top-level members of a JSON object text in
// document order, keeping repeated keys. Parameters: raw is the config text.
// Returns: the members and true, or false when raw is not a JSON object.
func scanConfigMembers(raw string) ([]configMember, bool) {
	if !json.Valid([]byte(raw)) {
		return nil, false
	}
	i := skipJSONSpace(raw, 0)
	if raw[i] != '{' {
		return nil, false
	}
	var members []configMember
	i = skipJSONSpace(raw, i+1)
	if raw[i] == '}' {
		return members, true
	}
	for {
		start := i
		keyEnd := skipJSONString(raw, i)
		var key string
		if err := json.Unmarshal([]byte(raw[start:keyEnd]), &key); err != nil {
			return nil, false
		}
		colon := skipJSONSpace(raw, keyEnd)
		i = skipJSONValue(raw, skipJSONSpace(raw, colon+1))
		members = append(members, configMember{start: start, end: i, key: key})
		i = skipJSONSpace(raw, i)
		if raw[i] != ',' {
			return members, true
		}
		i = skipJSONSpace(raw, i+1)
	}
}

// skipJSONSpace returns the index of the first non-whitespace byte at or after
// i. Parameters: raw is valid JSON text and i an index into it.
func skipJSONSpace(raw string, i int) int {
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\n' || raw[i] == '\r') {
		i++
	}
	return i
}

// skipJSONString returns the index just past the JSON string starting at i.
// Parameters: raw is valid JSON text and raw[i] is the opening quote.
func skipJSONString(raw string, i int) int {
	for i++; raw[i] != '"'; i++ {
		if raw[i] == '\\' {
			i++
		}
	}
	return i + 1
}

// skipJSONValue returns the index just past the JSON value starting at i.
// Parameters: raw is valid JSON text and raw[i] starts a value.
func skipJSONValue(raw string, i int) int {
	switch raw[i] {
	case '"':
		return skipJSONString(raw, i)
	case '{', '[':
		depth := 0
		for {
			switch raw[i] {
			case '"':
				i = skipJSONString(raw, i)
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			i++
			if depth == 0 {
				return i
			}
		}
	default:
		for i < len(raw) && !strings.ContainsRune(",}] \t\n\r", rune(raw[i])) {
			i++
		}
		return i
	}
}

// decodeChannelConfig decodes raw like LoadConfig and returns the decoded
// value together with the decode error. encoding/json keeps decoding past a
// kind mismatch, and LoadConfig callers such as the distributor keep using
// that partial value, so canonicalization compares and preserves it instead of
// failing on it. Parameters: raw is valid JSON object text.
func decodeChannelConfig(raw string) (ChannelConfig, error) {
	var cfg ChannelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, errors.Wrap(err, "decode channel config")
	}
	return cfg, nil
}

// encodeConfigValue encodes one effective ChannelConfig field value without
// HTML escaping. Parameters: value is the field value. Returns: compact JSON.
func encodeConfigValue(value reflect.Value) ([]byte, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value.Interface()); err != nil {
		return nil, errors.Wrap(err, "encode effective channel config value")
	}
	return bytes.TrimRight(encoded.Bytes(), "\n"), nil
}

// canonicalChannelConfig collapses every ChannelConfig field that more than
// one top-level key of raw decodes into (exact duplicates or spellings that
// match case-insensitively) into one member: the field's canonical json name
// at the position of its first member, holding the value LoadConfig decodes
// (for an object field, the merge encoding/json performs). Other members keep
// their bytes and order. Parameters: raw is the stored config text. Returns:
// raw itself when no field repeats or raw is not a JSON object, otherwise the
// canonical text; or an error when the effective value cannot be encoded or
// the rewrite would change what LoadConfig decodes.
func canonicalChannelConfig(raw string) (string, error) {
	members, ok := scanConfigMembers(raw)
	if !ok {
		return raw, nil
	}
	fields := make([]int, len(members))
	counts := make(map[int]int, len(members))
	repeated := false
	for i, member := range members {
		fields[i] = channelConfigFieldFor(member.key)
		if fields[i] >= 0 {
			counts[fields[i]]++
			repeated = repeated || counts[fields[i]] > 1
		}
	}
	if !repeated {
		return raw, nil
	}

	effective, decodeErr := decodeChannelConfig(raw)
	value := reflect.ValueOf(effective)
	written := make(map[int]bool, len(counts))
	var out strings.Builder
	out.WriteString(raw[:members[0].start])
	for i, member := range members {
		field := fields[i]
		collapsed := field >= 0 && counts[field] > 1
		if collapsed && written[field] {
			continue // a later member of a collapsed field; its value is already merged in.
		}
		if i > 0 {
			// The comma and whitespace before this member. Member 0 is always kept,
			// and a dropped member contributes neither its text nor its separator.
			out.WriteString(raw[members[i-1].end:member.start])
		}
		if !collapsed {
			out.WriteString(raw[member.start:member.end])
			continue
		}
		written[field] = true
		name, err := json.Marshal(channelConfigFieldNames[field])
		if err != nil {
			return "", errors.Wrapf(err, "encode channel config key %q", channelConfigFieldNames[field])
		}
		encoded, err := encodeConfigValue(value.Field(field))
		if err != nil {
			return "", errors.Wrapf(err, "canonicalize channel config field %q", channelConfigFieldNames[field])
		}
		out.Write(name)
		out.WriteByte(':')
		out.Write(encoded)
	}
	out.WriteString(raw[members[len(members)-1].end:])
	canonical := out.String()

	check, checkErr := decodeChannelConfig(canonical)
	if !reflect.DeepEqual(check, effective) {
		return "", errors.New("canonical channel config would change the decoded configuration")
	}
	if checkErr != nil && decodeErr == nil {
		return "", errors.Wrap(checkErr, "canonical channel config would no longer decode")
	}
	return canonical, nil
}

// canonicalizeConfig rewrites channel.Config with canonicalChannelConfig so a
// write never stores two members for one config field. Returns: a wrapped
// error when the config cannot be canonicalized.
func (channel *Channel) canonicalizeConfig() error {
	canonical, err := canonicalChannelConfig(channel.Config)
	if err != nil {
		return errors.Wrapf(err, "canonicalize config of channel %d", channel.Id)
	}
	channel.Config = canonical
	return nil
}
