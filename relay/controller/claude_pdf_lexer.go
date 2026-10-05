package controller

import (
	"bytes"
	"io"
)

// claudePDFTokenKind classifies one lexical PDF token for page-signal scanning.
type claudePDFTokenKind uint8

const (
	claudePDFTokenName claudePDFTokenKind = iota + 1
	claudePDFTokenInteger
	claudePDFTokenKeyword
	claudePDFTokenDictOpen
	claudePDFTokenDictClose
	claudePDFTokenArrayOpen
	claudePDFTokenArrayClose
)

// claudePDFTokenContext records where a token appeared. Only structural tokens
// update dictionary state; other contexts feed conservative counters only.
type claudePDFTokenContext uint8

const (
	claudePDFContextStructural claudePDFTokenContext = iota
	claudePDFContextComment
	claudePDFContextString
	claudePDFContextData
)

// claudePDFTokenHeadSize bounds the retained prefix of a name or keyword; every
// name the scanner matches is shorter, so longer tokens can never match.
const claudePDFTokenHeadSize = 16

// claudePDFHugeInteger replaces integers too long to parse; it exceeds every
// documented page limit so oversized page counts stay conservative.
const claudePDFHugeInteger = 1 << 30

// claudePDFDataPatternTokens is how many tokens stream data is fully lexed for
// after a /Kids or /Count name, long enough for a maximal page-tree array.
const claudePDFDataPatternTokens = 4096

// claudePDFEndstream terminates binary stream data, matched as a substring the
// way lenient readers search for it.
var claudePDFEndstream = []byte("endstream")

// claudePDFToken is one lexical token. Names are #xx-decoded into head.
type claudePDFToken struct {
	kind    claudePDFTokenKind
	context claudePDFTokenContext
	head    [claudePDFTokenHeadSize]byte
	length  int
	value   int
}

// is reports whether the complete token text equals text without allocating.
func (token *claudePDFToken) is(text string) bool {
	return token.length == len(text) && string(token.head[:token.length]) == text
}

// claudePDFLexer is a lenient, allocation-free PDF tokenizer over in-memory
// bytes or a streaming reader. It understands comments, literal strings and
// binary stream data only enough to classify token contexts; it never parses
// objects, follows references or decodes strings. Stream data exists only for
// in-memory input and is skipped quickly up to the next "endstream".
type claudePDFLexer struct {
	data        []byte
	pos         int
	end         int
	source      io.Reader
	comment     bool
	stringDepth int
	inData      bool
	dataTokens  int
}

// newClaudePDFMemoryLexer returns a lexer over the complete decoded document.
func newClaudePDFMemoryLexer(data []byte) *claudePDFLexer {
	return &claudePDFLexer{data: data, end: len(data)}
}

// newClaudePDFStreamLexer returns a lexer that refills chunk from source.
func newClaudePDFStreamLexer(source io.Reader, chunk []byte) *claudePDFLexer {
	return &claudePDFLexer{data: chunk[:0], source: source}
}

// position returns the in-memory offset of the next unread byte.
func (lexer *claudePDFLexer) position() int {
	return lexer.pos
}

// enterData marks the bytes after a stream keyword as binary data ending at the
// next "endstream". It applies only to in-memory input.
func (lexer *claudePDFLexer) enterData() {
	if lexer.source != nil {
		return
	}
	lexer.inData, lexer.dataTokens = true, 0
	if index := bytes.Index(lexer.data[lexer.pos:], claudePDFEndstream); index >= 0 {
		lexer.end = lexer.pos + index
	}
}

// leaveData resumes structural lexing after the "endstream" that ended data.
func (lexer *claudePDFLexer) leaveData() {
	if lexer.end < len(lexer.data) {
		lexer.pos = lexer.end + len(claudePDFEndstream)
	}
	lexer.inData, lexer.end, lexer.dataTokens = false, len(lexer.data), 0
}

// readByte returns the next byte and false at end of input, end of the current
// data region, or a read error.
func (lexer *claudePDFLexer) readByte() (byte, bool) {
	if lexer.pos >= lexer.end && !lexer.fill() {
		return 0, false
	}
	value := lexer.data[lexer.pos]
	lexer.pos++
	return value, true
}

// unreadByte steps back over the byte just returned by readByte, which always
// remains in the current chunk.
func (lexer *claudePDFLexer) unreadByte() {
	lexer.pos--
}

// fill loads the next streaming chunk and returns false when no bytes remain.
func (lexer *claudePDFLexer) fill() bool {
	if lexer.source == nil {
		return false
	}
	chunk := lexer.data[:cap(lexer.data)]
	for attempt := 0; attempt < 8; attempt++ {
		n, err := lexer.source.Read(chunk)
		if n > 0 {
			lexer.data, lexer.pos, lexer.end = chunk[:n], 0, n
			return true
		}
		if err != nil {
			return false
		}
	}
	return false
}

// claudePDFRegular reports whether value is neither PDF whitespace nor a delimiter.
func claudePDFRegular(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

// claudePDFHexValue decodes one hexadecimal digit and reports validity.
func claudePDFHexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	}
	return 0, false
}

// next stores the next token in token and returns false at end of input.
// Comments end at an end-of-line byte and literal strings honor nesting and
// backslash escapes. In stream data only names are reported, except that the
// tokens following a /Kids or /Count name are fully lexed.
func (lexer *claudePDFLexer) next(token *claudePDFToken) bool {
	for {
		if lexer.inData {
			if lexer.pos >= lexer.end {
				lexer.leaveData()
				continue
			}
			if lexer.dataTokens == 0 {
				slash := bytes.IndexByte(lexer.data[lexer.pos:lexer.end], '/')
				if slash < 0 {
					lexer.pos = lexer.end
					continue
				}
				lexer.pos += slash + 1
				// Stream data only matters for Encrypt, ObjStm, Kids and Count
				// names, possibly #xx-escaped; skip other names cheaply.
				if lexer.pos < lexer.end {
					switch lexer.data[lexer.pos] {
					case 'E', 'O', 'K', 'C', '#':
					default:
						continue
					}
				}
				lexer.lexName(token)
				token.context = claudePDFContextData
				if token.is("Kids") || token.is("Count") {
					lexer.dataTokens = claudePDFDataPatternTokens
				}
				return true
			}
		}
		value, ok := lexer.readByte()
		if !ok {
			if lexer.inData {
				continue
			}
			return false
		}
		if lexer.comment && (value == '\n' || value == '\r') {
			lexer.comment = false
			continue
		}
		context := claudePDFContextStructural
		switch {
		case lexer.inData:
			context = claudePDFContextData
		case lexer.comment:
			context = claudePDFContextComment
		case lexer.stringDepth > 0:
			context = claudePDFContextString
		}
		switch value {
		case 0, '\t', '\n', '\f', '\r', ' ', '{', '}':
			continue
		case '%':
			if context == claudePDFContextStructural {
				lexer.comment = true
			}
			continue
		case '(':
			if context == claudePDFContextStructural || context == claudePDFContextString {
				lexer.stringDepth++
			}
			continue
		case ')':
			if context == claudePDFContextString {
				lexer.stringDepth--
			}
			continue
		case '\\':
			if context == claudePDFContextString {
				lexer.readByte()
			}
			continue
		case '[':
			token.kind = claudePDFTokenArrayOpen
		case ']':
			token.kind = claudePDFTokenArrayClose
		case '<', '>':
			following, more := lexer.readByte()
			if !more || following != value {
				if more {
					lexer.unreadByte()
				}
				continue
			}
			token.kind = claudePDFTokenDictOpen
			if value == '>' {
				token.kind = claudePDFTokenDictClose
			}
		case '/':
			lexer.lexName(token)
		default:
			lexer.lexRegular(token, value)
		}
		token.context = context
		if context == claudePDFContextData {
			lexer.dataTokens--
		}
		return true
	}
}

// lexName reads a name after its solidus, decoding #xx escapes as PDF readers do.
func (lexer *claudePDFLexer) lexName(token *claudePDFToken) {
	token.kind, token.length = claudePDFTokenName, 0
	for {
		value, ok := lexer.readByte()
		if !ok {
			return
		}
		if !claudePDFRegular(value) {
			lexer.unreadByte()
			return
		}
		if value == '#' {
			if high, more := lexer.readByte(); more {
				if highValue, valid := claudePDFHexValue(high); valid {
					if low, more := lexer.readByte(); more {
						if lowValue, valid := claudePDFHexValue(low); valid {
							value = highValue<<4 | lowValue
						} else {
							lexer.unreadByte()
							lexer.appendName(token, '#')
							value = high
						}
					} else {
						lexer.appendName(token, '#')
						value = high
					}
				} else {
					lexer.unreadByte()
				}
			}
		}
		lexer.appendName(token, value)
	}
}

// appendName stores one decoded name byte while counting the full length.
func (lexer *claudePDFLexer) appendName(token *claudePDFToken, value byte) {
	if token.length < claudePDFTokenHeadSize {
		token.head[token.length] = value
	}
	token.length++
}

// lexRegular reads a keyword or number starting with first. Integers of at most
// nine digits are parsed; longer integers become claudePDFHugeInteger.
func (lexer *claudePDFLexer) lexRegular(token *claudePDFToken, first byte) {
	token.length, token.value = 0, 0
	digits, integer, negative := 0, true, false
	value, ok := first, true
	for ok && claudePDFRegular(value) {
		if token.length < claudePDFTokenHeadSize {
			token.head[token.length] = value
		}
		switch {
		case value >= '0' && value <= '9':
			digits++
			if digits <= 9 {
				token.value = token.value*10 + int(value-'0')
			}
		case token.length == 0 && (value == '+' || value == '-'):
			negative = value == '-'
		default:
			integer = false
		}
		token.length++
		value, ok = lexer.readByte()
	}
	if ok {
		lexer.unreadByte()
	}
	token.kind = claudePDFTokenKeyword
	if integer && digits > 0 {
		token.kind = claudePDFTokenInteger
		if digits > 9 {
			token.value = claudePDFHugeInteger
		}
		if negative {
			token.value = -token.value
		}
	}
}
