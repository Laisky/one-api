package common

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// errJSONRootNotObject reports a JSON payload whose root is not an object.
var errJSONRootNotObject = errors.New("JSON root is not an object")

// FoldJSONKey returns the key under which encoding/json matches an object
// member to a struct field: every rune is replaced by the smallest rune of its
// Unicode simple-fold orbit. "Extra_Body", "EXTRA_BODY" and a Kelvin-sign
// spelling of "max_tokens" therefore fold exactly as Go's case-insensitive
// struct decoder folds them.
// Parameters: name is a decoded (unescaped) member name. Returns: its fold.
func FoldJSONKey(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		b.WriteRune(foldJSONRune(r))
	}
	return b.String()
}

// foldJSONRune returns the smallest rune in the simple-fold orbit of r.
// Parameters: r is any rune. Returns: the orbit's canonical representative.
func foldJSONRune(r rune) rune {
	for {
		next := unicode.SimpleFold(r)
		if next <= r {
			return next
		}
		r = next
	}
}

// JSONKeyMatches reports whether a decoded object key is read as the field
// named canonical by encoding/json's case-insensitive struct decoder.
// Parameters: key is a decoded member name; canonical is the field's JSON
// name. Returns: true for the exact name and every case-folded spelling.
func JSONKeyMatches(key, canonical string) bool {
	return key == canonical || FoldJSONKey(key) == FoldJSONKey(canonical)
}

// ScanJSONRootKeys lists the member names of a JSON object root in wire
// order, duplicates included. Names are unescaped exactly as encoding/json
// reads them, so a key that spells a letter as a JSON unicode escape is
// reported in its decoded form; byte-substring checks cannot do this. The
// payload is validated first, then walked once without decoding values.
// Parameters: body is a raw JSON payload. Returns: the decoded root keys, or
// an error when body is not exactly one valid JSON object.
func ScanJSONRootKeys(body []byte) ([]string, error) {
	if !json.Valid(body) {
		return nil, errors.New("invalid JSON payload")
	}
	return scanValidJSONRootKeys(body)
}

// scanValidJSONRootKeys walks the root keys of a payload already known to be
// valid JSON (for example one json.Unmarshal accepted, which validates the
// whole input first). Validity guarantees every index stays in range and that
// each member is a string key, a colon and one complete value.
// Parameters: body is valid JSON. Returns: the decoded root keys, or an error
// when the root is not an object.
func scanValidJSONRootKeys(body []byte) ([]string, error) {
	i := skipJSONSpace(body, 0)
	if body[i] != '{' {
		return nil, errors.WithStack(errJSONRootNotObject)
	}
	var keys []string
	i = skipJSONSpace(body, i+1)
	if body[i] == '}' {
		return keys, nil
	}
	for {
		end := jsonStringEnd(body, i)
		key, err := decodeJSONKey(body[i:end])
		if err != nil {
			return nil, errors.Wrap(err, "decode JSON root key")
		}
		keys = append(keys, key)
		i = skipJSONSpace(body, end)
		i = skipJSONValue(body, skipJSONSpace(body, i+1))
		i = skipJSONSpace(body, i)
		if body[i] == '}' {
			return keys, nil
		}
		i = skipJSONSpace(body, i+1)
	}
}

// skipJSONSpace returns the index of the first non-whitespace byte at or
// after i. Parameters: b is valid JSON; i is a start index. Returns: an index.
func skipJSONSpace(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// jsonStringEnd returns the index just past the closing quote of the string
// that opens at b[i]. A quote closes the string unless an odd number of
// backslashes precedes it. Parameters: b is valid JSON; b[i] is '"'.
// Returns: the index after the closing quote.
func jsonStringEnd(b []byte, i int) int {
	for i++; ; {
		q := bytes.IndexByte(b[i:], '"')
		if q < 0 {
			return len(b)
		}
		i += q
		backslashes := 0
		for j := i - 1; j >= 0 && b[j] == '\\'; j-- {
			backslashes++
		}
		i++
		if backslashes%2 == 0 {
			return i
		}
	}
}

// skipJSONValue returns the index just past the value that starts at b[i].
// Parameters: b is valid JSON; i is the first byte of a value. Returns: the
// index after the value.
func skipJSONValue(b []byte, i int) int {
	switch b[i] {
	case '"':
		return jsonStringEnd(b, i)
	case '{', '[':
		depth := 0
		for i < len(b) {
			switch b[i] {
			case '"':
				i = jsonStringEnd(b, i)
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
			i++
		}
		return i
	default:
		for i < len(b) {
			switch b[i] {
			case ',', '}', ']', ' ', '\t', '\n', '\r':
				return i
			}
			i++
		}
		return i
	}
}

// decodeJSONKey unquotes one JSON string token the way encoding/json does,
// including escape decoding and U+FFFD replacement of invalid UTF-8.
// Parameters: raw is a complete quoted string token. Returns: its value.
func decodeJSONKey(raw []byte) (string, error) {
	plain := len(raw) >= 2
	for _, c := range raw {
		if c == '\\' || c >= utf8.RuneSelf {
			plain = false
			break
		}
	}
	if plain {
		return string(raw[1 : len(raw)-1]), nil
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil {
		return "", errors.Wrap(err, "unquote JSON key")
	}
	return key, nil
}
