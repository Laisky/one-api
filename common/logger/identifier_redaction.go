package logger

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/zap"
)

// maxLoggedIdentifierBytes bounds an identifier written to a log field.
const maxLoggedIdentifierBytes = 128

// Thresholds of the secret-shape rules in secretShapedIdentifier. They are
// chosen so that provider error vocabulary passes while credentials and
// opaque identifiers do not:
//   - provider numeric error codes have at most 8 digits (Baidu 336003, Zhipu
//     1302, iFlytek 11200, MiniMax 1008), while account numbers, AWS account
//     ids (12), phone numbers, payment cards and Unix timestamps have 10 or more;
//   - provider vocabulary words are letter-only (InvalidParameter,
//     ModelAccountTpmRateLimitExceeded) or embed one number as a single token
//     (IPv6AddressInvalid, Base64DecodeError, S3BucketNotFound), giving at most
//     two letter/digit boundaries, while hex digests and base62/base64url key
//     bodies of 16 or more characters scatter digits among letters;
//   - the longest letter-only vocabulary word seen in provider error codes is
//     38 characters (ProvisionedThroughputExceededException), so an unseparated
//     run of 48 or more characters is never treated as vocabulary.
const (
	minSecretDigitRun          = 10
	minTokenSegmentBytes       = 16
	minTokenDigitBoundaries    = 3
	maxVocabularySegmentBytes  = 48
	minIsolatedLowercaseLetter = 2
)

// loggableIdentifierPattern matches the identifier shapes that provider error
// codes, types and params use, such as "insufficient_quota", "1302" or
// "messages[0].content". Free text, whitespace, quotes, "@", ":", "+" and "="
// never match, which already excludes e-mail addresses, URLs, sentences and
// padded base64.
var loggableIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.\-\[\]/]{1,128}$`)

// uuidPattern matches an RFC 4122 UUID anywhere in an identifier. UUIDs name
// accounts, organizations, projects and subscriptions; they are never error
// vocabulary.
var uuidPattern = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// ipv4Pattern matches a dotted-quad IPv4 address that is not part of a longer
// dotted number, since an address identifies a client or host.
var ipv4Pattern = regexp.MustCompile(`(?:^|[^0-9.])(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?:$|[^0-9.])`)

// credentialPrefixes lists the namespaces that credential issuers reserve for
// secrets and bearer tokens. No provider error vocabulary starts with them, so
// a match at the start of the value or right after a separator is redacted
// regardless of length. AWS temporary key ids (ASIA...) are left to the token
// rule because "ASIA" also names a region.
var credentialPrefixes = []string{
	"sk-", "sk_", // OpenAI, Anthropic, DeepSeek, Moonshot and most OpenAI-compatible hosts; Stripe secret keys
	"rk_",                                  // Stripe restricted keys
	"sess-",                                // OpenAI session keys
	"eyJ",                                  // JWT, JWS and JWE: base64url of `{"`
	"AIza",                                 // Google API keys
	"ya29.",                                // Google OAuth access tokens
	"AKIA",                                 // AWS access key ids
	"AKID",                                 // Tencent Cloud SecretId
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", // GitHub tokens
	"github_pat_",                                        // GitHub fine-grained tokens
	"glpat-",                                             // GitLab personal access tokens
	"xoxa-", "xoxb-", "xoxp-", "xoxr-", "xoxs-", "xapp-", // Slack tokens
	"hf_",    // Hugging Face
	"gsk_",   // Groq
	"xai-",   // xAI
	"r8_",    // Replicate
	"pplx-",  // Perplexity
	"nvapi-", // NVIDIA
}

// UntrustedIdentifierFields takes a log key and an untrusted value, such as an
// error code, type or param that an upstream provider controls, and returns the
// fields that record it without leaking free-form data or secrets into
// operator logs.
//
// A value is logged verbatim under key only when it is vocabulary-shaped: a
// trimmed string (or a number of any Go numeric kind, formatted in plain
// decimal) of at most 128 bytes made of letters, digits and _ . - [ ] /, that
// is not secret-shaped (see secretShapedIdentifier). Anything else (free text,
// an e-mail address, a credential, an opaque account identifier, a map, slice,
// bool or pointer) is logged as Redact(value) plus a "<key>_redacted" true
// field, following the request_url/request_url_redacted convention. Callers
// that classify the value must keep using the raw value; only the log view is
// sanitized.
//
// Parameters:
//   - key: the log field name.
//   - value: the untrusted value; nil, nil pointers/maps/slices and blank
//     strings are treated as absent.
//
// Returns:
//   - []zap.Field: nothing for an absent value, one field for a loggable value,
//     or the redacted field and its "<key>_redacted" marker otherwise.
func UntrustedIdentifierFields(key string, value any) []zap.Field {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	text, ok := "", false
	switch rv.Kind() {
	case reflect.String:
		text = strings.TrimSpace(rv.String())
		if text == "" {
			return nil
		}
		ok = true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		text, ok = strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		text, ok = strconv.FormatUint(rv.Uint(), 10), true
	case reflect.Float32:
		text, ok = strconv.FormatFloat(rv.Float(), 'f', -1, 32), true
	case reflect.Float64:
		text, ok = strconv.FormatFloat(rv.Float(), 'f', -1, 64), true
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		if rv.IsNil() {
			return nil
		}
	default:
		// Bools, structs, arrays, channels and functions are never identifiers.
	}
	if ok && loggableIdentifier(text) {
		return []zap.Field{zap.String(key, text)}
	}
	return []zap.Field{zap.String(key, Redact(value)), zap.Bool(key+"_redacted", true)}
}

// loggableIdentifier reports whether text may be written to a log verbatim.
//
// Parameters:
//   - text: a trimmed, non-empty candidate identifier.
//
// Returns:
//   - bool: true when text is identifier-shaped and not secret-shaped.
func loggableIdentifier(text string) bool {
	if len(text) > maxLoggedIdentifierBytes || !loggableIdentifierPattern.MatchString(text) {
		return false
	}
	return !secretShapedIdentifier(text)
}

// secretShapedIdentifier reports whether an identifier-shaped value looks like
// a credential or an opaque identifier rather than error vocabulary. The value
// is secret-shaped when any of these holds:
//   - a credential prefix (credentialPrefixes) starts the value or follows a
//     separator, which covers API keys, JWTs and OAuth tokens;
//   - it contains a UUID or a dotted-quad IPv4 address;
//   - it contains a run of 10 or more digits (account and card numbers);
//   - a separator-free segment of 16 or more characters mixes letters and
//     digits and is either all hexadecimal or crosses between letters and
//     digits at least three times (hex digests, base62 and base64url key
//     bodies), unlike vocabulary that embeds one number;
//   - a separator-free segment of 16 or more letters mixes case the way random
//     text does (see randomCaseMix);
//   - a separator-free segment is 48 or more characters long.
//
// The rules are structural rather than entropy based because Shannon entropy
// does not separate CamelCase vocabulary from random text at these lengths.
// Measured on seeded random strings, they redact every hex token of 16 or more
// characters and over 99% of base62 and lowercase-alphanumeric tokens of 32
// characters, but only about 87% of letter-only base52 tokens of that length,
// which is why issuer prefixes are checked first. Residual risk: a short or
// vocabulary-looking opaque value (for example "internal-account-123") is
// still logged.
//
// Parameters:
//   - text: an identifier-shaped value.
//
// Returns:
//   - bool: true when the value must be redacted.
func secretShapedIdentifier(text string) bool {
	if hasCredentialPrefix(text) || uuidPattern.MatchString(text) || ipv4Pattern.MatchString(text) {
		return true
	}
	for segment := range strings.FieldsFuncSeq(text, isIdentifierSeparator) {
		if tokenShapedSegment(segment) {
			return true
		}
	}
	return false
}

// isIdentifierSeparator reports whether r separates words in an identifier.
//
// Parameters:
//   - r: the rune to classify.
//
// Returns:
//   - bool: true for _ . - / [ and ].
func isIdentifierSeparator(r rune) bool {
	switch r {
	case '_', '.', '-', '/', '[', ']':
		return true
	default:
		return false
	}
}

// hasCredentialPrefix reports whether a credential prefix starts text or
// follows one of its separators.
//
// Parameters:
//   - text: an identifier-shaped value.
//
// Returns:
//   - bool: true when a credential prefix is found at a word boundary.
func hasCredentialPrefix(text string) bool {
	for i := range len(text) {
		if i > 0 && !isIdentifierSeparator(rune(text[i-1])) {
			continue
		}
		for _, prefix := range credentialPrefixes {
			if strings.HasPrefix(text[i:], prefix) {
				return true
			}
		}
	}
	return false
}

// tokenShapedSegment reports whether one separator-free segment of an
// identifier looks like part of a secret or an account number.
//
// Parameters:
//   - segment: an ASCII letters-and-digits segment.
//
// Returns:
//   - bool: true when the segment has a long digit run, scatters digits among
//     letters over at least 16 characters, mixes case randomly, or is too long
//     to be a vocabulary word.
func tokenShapedSegment(segment string) bool {
	if len(segment) >= maxVocabularySegmentBytes {
		return true
	}
	letters, digits, digitRun, boundaries := 0, 0, 0, 0
	allHex := true
	for i := range len(segment) {
		c := segment[i]
		isDigit := c >= '0' && c <= '9'
		if i > 0 && isDigit != (segment[i-1] >= '0' && segment[i-1] <= '9') {
			boundaries++
		}
		if isDigit {
			digits++
			digitRun++
			if digitRun >= minSecretDigitRun {
				return true
			}
			continue
		}
		letters++
		digitRun = 0
		if !strings.ContainsRune("abcdefABCDEF", rune(c)) {
			allHex = false
		}
	}
	if len(segment) < minTokenSegmentBytes {
		return false
	}
	if letters > 0 && digits > 0 && (allHex || boundaries >= minTokenDigitBoundaries) {
		return true
	}
	return randomCaseMix(segment)
}

// randomCaseMix reports whether a letter segment mixes case the way random
// text does rather than the way CamelCase vocabulary does. In CamelCase an
// uppercase letter starts a word and is followed by a lowercase run, so
// uppercase letters are a minority and a lowercase letter squeezed between two
// non-lowercase characters (the "v" of "IPv6", the "r" of "Or") is rare. Random
// base52 text has uppercase near half of its letters and many such isolated
// lowercase letters. The segment is random when uppercase is between one third
// and two thirds of its letters and it has at least two isolated interior
// lowercase letters; vocabulary such as "HTTPRequestJSONError" or
// "TokenIsTooLargeOrInvalid" fails one of the two conditions.
//
// Parameters:
//   - segment: an ASCII letters-and-digits segment.
//
// Returns:
//   - bool: true when the case mixing looks random.
func randomCaseMix(segment string) bool {
	upper, lower, isolated := 0, 0, 0
	isLower := func(c byte) bool { return c >= 'a' && c <= 'z' }
	for i := range len(segment) {
		c := segment[i]
		switch {
		case c >= 'A' && c <= 'Z':
			upper++
		case isLower(c):
			lower++
			if i > 0 && i < len(segment)-1 && !isLower(segment[i-1]) && !isLower(segment[i+1]) {
				isolated++
			}
		}
	}
	letters := upper + lower
	return isolated >= minIsolatedLowercaseLetter && 3*upper >= letters && 3*upper <= 2*letters
}
