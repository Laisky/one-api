package model

import (
	"bytes"
	"encoding/json"
)

const (
	// responseReceiptValueLimit bounds one retained receipt value. Provider
	// usage objects are a few hundred bytes; a larger one is not a receipt.
	responseReceiptValueLimit = 64 << 10
	// responseReceiptKeyLimit bounds one tracked member name. Receipt keys are
	// short; a longer key is skipped without being retained.
	responseReceiptKeyLimit = 64
)

// responseReceiptRootKeys are the root members consumeJSON reads for billing.
var responseReceiptRootKeys = map[string]bool{"type": true, "usage": true, "service_tier": true, "system_fingerprint": true}

// responseReceiptNestedParents are the root members whose object value is a
// protocol envelope carrying its own receipt (Responses stream events nest the
// response object; Claude message_start nests the message).
var responseReceiptNestedParents = map[string]bool{"response": true, "message": true}

// responseReceiptNestedKeys are the receipt members read inside a nested envelope.
var responseReceiptNestedKeys = map[string]bool{"usage": true, "service_tier": true}

// responseReceiptFrame is one tracked container: the root object (level 1) or
// a container directly under a root member (level 2).
type responseReceiptFrame struct {
	object, expectKey, awaitValue bool
	key, parent                   string
}

// responseReceiptScanner extracts the billing receipt members of one JSON
// object without retaining the rest of it. It is fed incrementally, so a
// response larger than the accumulator's capture window still yields its
// usage, service tier and event type in O(retained receipt) memory. It checks
// only the structure it needs; values it returns are validated JSON.
type responseReceiptScanner struct {
	depth                         int
	frames                        [3]responseReceiptFrame
	inString, escaped, readingKey bool
	keyOverflow                   bool
	key                           []byte
	capturing                     bool
	capturePath                   string
	captureDepth                  int
	captureKind                   byte
	capture                       []byte
	fields                        map[string]json.RawMessage
	closed, failed                bool
}

// newResponseReceiptScanner returns a scanner positioned before the root value.
func newResponseReceiptScanner() *responseReceiptScanner {
	return &responseReceiptScanner{fields: make(map[string]json.RawMessage)}
}

// Write feeds the next bytes of the JSON document. It never fails; a document
// the scanner cannot follow is reported by Result. It returns len(p) and nil
// so the scanner can be used as an io.Writer.
func (s *responseReceiptScanner) Write(p []byte) (int, error) {
	for i := 0; i < len(p) && !s.failed; i++ {
		if s.inString && !s.escaped && !s.capturing && !s.readingKey {
			// Large payloads are mostly string content the receipt never needs
			// (output text, base64 media, ciphertext): skip to its next quote or
			// escape instead of stepping through it byte by byte.
			skip := bytes.IndexAny(p[i:], `"\`)
			if skip < 0 {
				break
			}
			i += skip
		}
		s.feed(p[i])
	}
	return len(p), nil
}

// feed advances the scanner by one byte.
func (s *responseReceiptScanner) feed(b byte) {
	if s.inString {
		s.feedString(b)
		return
	}
	if isJSONSpace(b) {
		if s.capturing && s.captureKind == 'l' && s.depth == s.captureDepth {
			s.finishCapture()
		}
		return
	}
	if s.closed {
		s.failed = true // trailing data after the root value
		return
	}
	if s.capturing && s.captureKind == 'l' && s.depth == s.captureDepth && (b == ',' || b == '}' || b == ']') {
		s.finishCapture()
	}
	valueStart := s.depth >= 1 && s.depth <= 2 && s.frames[s.depth].awaitValue
	if valueStart {
		s.frames[s.depth].awaitValue = false
	}
	switch {
	case s.capturing:
		s.appendCapture(b) // a member of a value already being retained
	case valueStart:
		s.startValue(b)
	}
	switch b {
	case '{', '[':
		if s.depth == 0 && b != '{' {
			s.failed = true // receipts are carried by a root object
			return
		}
		s.depth++
		if s.depth <= 2 {
			parent := ""
			if s.depth == 2 {
				parent = s.frames[1].key
			}
			s.frames[s.depth] = responseReceiptFrame{object: b == '{', expectKey: b == '{', parent: parent}
		}
	case '}', ']':
		if s.depth == 0 || (s.depth <= 2 && s.frames[s.depth].object != (b == '}')) {
			s.failed = true
			return
		}
		s.depth--
		if s.capturing && s.depth == s.captureDepth {
			s.finishCapture()
		}
		if s.depth == 0 {
			s.closed = true
		}
	case ':':
		if s.depth >= 1 && s.depth <= 2 && s.frames[s.depth].object {
			s.frames[s.depth].awaitValue = true
		}
	case ',':
		if s.depth >= 1 && s.depth <= 2 && s.frames[s.depth].object {
			s.frames[s.depth].expectKey = true
		}
	case '"':
		s.inString, s.escaped = true, false
		if s.depth >= 1 && s.depth <= 2 && s.frames[s.depth].object && s.frames[s.depth].expectKey {
			s.frames[s.depth].expectKey = false
			s.readingKey, s.keyOverflow = true, false
			s.key = s.key[:0]
		}
	}
}

// feedString advances the scanner by one byte inside a JSON string.
func (s *responseReceiptScanner) feedString(b byte) {
	if s.capturing {
		s.appendCapture(b)
	}
	switch {
	case s.escaped:
		s.escaped = false
	case b == '\\':
		s.escaped = true
	case b == '"':
		s.inString = false
		if s.readingKey {
			s.readingKey = false
			s.frames[s.depth].key = s.decodedKey()
			return
		}
		if s.capturing && s.captureKind == '"' && s.depth == s.captureDepth {
			s.finishCapture()
		}
		return
	}
	if s.readingKey {
		if len(s.key) >= responseReceiptKeyLimit {
			s.keyOverflow = true
			return
		}
		s.key = append(s.key, b)
	}
}

// decodedKey returns the member name just read, unescaped as encoding/json
// would, or "" for an oversized or invalid name that no receipt key matches.
func (s *responseReceiptScanner) decodedKey() string {
	if s.keyOverflow {
		return ""
	}
	if bytes.IndexByte(s.key, '\\') < 0 {
		return string(s.key)
	}
	quoted := make([]byte, 0, len(s.key)+2)
	quoted = append(append(append(quoted, '"'), s.key...), '"')
	var name string
	if json.Unmarshal(quoted, &name) != nil {
		return ""
	}
	return name
}

// startValue begins retaining a member value when its path is a receipt field.
// Parameter b is the first byte of the value at the current depth.
func (s *responseReceiptScanner) startValue(b byte) {
	frame := s.frames[s.depth]
	path := ""
	switch {
	case s.depth == 1 && responseReceiptRootKeys[frame.key]:
		path = frame.key
	case s.depth == 2 && frame.object && responseReceiptNestedParents[frame.parent] && responseReceiptNestedKeys[frame.key]:
		path = frame.parent + "." + frame.key
	}
	if path == "" {
		return
	}
	s.capturing, s.capturePath, s.captureDepth = true, path, s.depth
	switch b {
	case '{', '[', '"':
		s.captureKind = b
	default:
		s.captureKind = 'l'
	}
	s.capture = append(s.capture[:0], b)
}

// appendCapture retains one byte of the current receipt value, abandoning a
// value that exceeds responseReceiptValueLimit.
func (s *responseReceiptScanner) appendCapture(b byte) {
	if len(s.capture) >= responseReceiptValueLimit {
		s.failed = true // an oversized "receipt" is not one the gateway can trust
		return
	}
	s.capture = append(s.capture, b)
}

// finishCapture stores the completed receipt value; later duplicates replace
// earlier ones, matching encoding/json's map decoding.
func (s *responseReceiptScanner) finishCapture() {
	s.capturing = false
	if !json.Valid(s.capture) {
		s.failed = true
		return
	}
	s.fields[s.capturePath] = append(json.RawMessage(nil), s.capture...)
	s.capture = s.capture[:0]
}

// Result returns a compact JSON object holding only the receipt members of a
// complete, structurally valid document, shaped like the original envelope so
// consumeJSON can read it. It returns false when the document was incomplete
// or could not be followed.
func (s *responseReceiptScanner) Result() ([]byte, bool) {
	if s.failed || !s.closed || s.inString || s.capturing {
		return nil, false
	}
	root := make(map[string]any, len(s.fields))
	nested := make(map[string]map[string]json.RawMessage)
	for path, raw := range s.fields {
		parent, child, found := bytes.Cut([]byte(path), []byte("."))
		if !found {
			root[path] = raw
			continue
		}
		members := nested[string(parent)]
		if members == nil {
			members = make(map[string]json.RawMessage)
			nested[string(parent)] = members
		}
		members[string(child)] = raw
	}
	for parent, members := range nested {
		root[parent] = members
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, false
	}
	return out, true
}

// isJSONSpace reports whether b is insignificant JSON whitespace.
func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
