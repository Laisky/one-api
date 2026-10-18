package otelbridge

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
)

// maxPrivateTextBytes bounds sanitizer work before queue admission. Existing SDK
// attribute and queue limits still apply after this independent per-value cap.
const maxPrivateTextBytes = 4096

const privateReplacement = "[REDACTED]"

var (
	privateVersionPattern    = regexp.MustCompile(`v[0-9]+$`)
	privateURLPattern        = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s<>"']+`)
	privateHeaderPattern     = regexp.MustCompile(`(?im)\b((?:set[-_]?cookie|cookie|authorization|proxy[-_]?authorization)\s*:\s*)[^\r\n]*`)
	privateAuthPattern       = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[a-z0-9._~+/=-]+`)
	privateKeyPattern        = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]+|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`)
	privateAssignmentPattern = regexp.MustCompile(`(?i)(["']?(?:authorization|proxy[-_ ]?authorization|api[-_ ]?key|access[-_ ]?key|private[-_ ]?key|secret[-_ ]?key|password[-_ ]?hash|session[-_ ]?secret|password|passwd|pwd|secret|client[-_ ]?secret|access[-_ ]?token|refresh[-_ ]?token|auth[-_ ]?token|id[-_ ]?token|token|credentials|cookie|set[-_ ]?cookie)(?:[-_ ]?v[0-9]+)?["']?\s*[:=]\s*)`)
	privatePayloadPattern    = regexp.MustCompile(`(?i)\b((?:raw[ _-]?|request[ _-]?|response[ _-]?)?(?:body|payload)|prompt|messages|input|output|content)\s*["\']?\s*[:=]\s*`)
	privateDataPattern       = regexp.MustCompile(`(?i)\bdata:[^\s"']+`)
)

// SanitizeLogText takes diagnostic text and returns bounded text with recognized
// credential forms, URL credentials/query/fragment, and labeled payloads removed.
// It preserves ordinary operational prose. Callers must still use explicit
// redaction for unlabelled private application data; text is not a DLP oracle.
func SanitizeLogText(value string) string {
	truncated := len(value) > maxPrivateTextBytes
	if truncated {
		value = value[:maxPrivateTextBytes]
	}
	value = strings.ToValidUTF8(value, "�")
	if len(value) > maxPrivateTextBytes {
		value = value[:maxPrivateTextBytes]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	if truncated {
		value += " [truncated]"
	}
	value = privateDataPattern.ReplaceAllString(value, "[REDACTED data URL]")
	value = privateURLPattern.ReplaceAllStringFunc(value, sanitizePrivateURL)
	value = privateHeaderPattern.ReplaceAllString(value, "$1"+privateReplacement)
	value = privateAuthPattern.ReplaceAllString(value, "$1 "+privateReplacement)
	value = privateKeyPattern.ReplaceAllString(value, privateReplacement)
	value = redactPrivateAssignments(value)
	// Every labeled payload in generic text is private, including read/reading
	// prefixes. Typed wrapping-error context is projected only by ErrorType.
	if match := privatePayloadPattern.FindStringIndex(value); match != nil {
		value = value[:match[1]] + privateReplacement
	}

	return value
}

// privateFieldName takes a field name and returns its case/separator-insensitive
// identity for exact sensitive names and known credential/payload suffixes.
func privateFieldName(key string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r == '.' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, key)
}

// privateSensitiveField reports explicitly credential- or payload-bearing names.
// Counts and operational metadata such as prompt_tokens and content_length are
// deliberately distinct from prompt/content values.
func privateSensitiveField(key string) bool {
	compact := privateVersionPattern.ReplaceAllString(privateFieldName(key), "")
	switch compact {
	case "authorization", "proxyauthorization", "auth", "apikey", "key",
		"password", "passwd", "secret", "clientsecret", "sessionsecret",
		"token", "accesstoken", "refreshtoken", "idtoken", "credential", "credentials",
		"cookie", "setcookie", "privatekey", "headers", "requestheaders", "responseheaders",
		"prompt", "messages", "input", "output", "content", "body", "payload",
		"request", "response", "requestbody", "responsebody", "rawbody",
		"requestpayload", "responsepayload", "requestdata", "responsedata", "dsn", "rawquery",
		"query", "querytext", "querystring", "searchquery":
		return true
	}
	for _, suffix := range []string{
		"apikey", "apitoken", "accesskey", "accesstoken", "refreshtoken", "authtoken",
		"idtoken", "pushtoken", "clientsecret", "authorization", "authheader", "credential",
		"credentials", "cookieheader", "bearertoken", "sessiontoken", "password",
		"passwd", "passwordhash", "privatekey", "secretkey", "tokenprefix", "apikeyprefix",
		"requestheaders", "responseheaders", "requestbody", "responsebody", "rawbody",
		"requestpayload", "responsepayload", "rawpayload",
	} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	return false
}

// SanitizeLogAttribute takes a structured value and returns a safe scalar or
// false for unsupported containers/bytes and malformed keys. It never serializes
// arbitrary objects. Sensitive scalar fields retain their key and a redaction
// marker, while ordinary strings, booleans and finite numbers retain their types.
func SanitizeLogAttribute(kv attribute.KeyValue) (attribute.KeyValue, bool) {
	key := string(kv.Key)
	if len(key) == 0 || len(key) > 128 || !utf8.ValidString(key) {
		return kv, false
	}
	for _, r := range key {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return kv, false
		}
	}
	if privateReservedField(key) {
		return kv, false
	}
	if privateSensitiveField(key) {
		return attribute.String(key, privateReplacement), true
	}
	switch privateFieldName(key) {
	case "url", "requesturl", "responseurl", "upstreamurl", "endpoint", "upstreamendpoint", "imageurl", "uri", "requesturi":
		if kv.Value.Type() != attribute.STRING {
			return kv, false
		}
		return attribute.String(key, sanitizePrivateURL(kv.Value.AsString())), true
	}
	if key == "status" && kv.Value.Type() == attribute.STRING {
		return attribute.String(key, SanitizeLogText(kv.Value.AsString())), true
	}
	if privateNumericFields[key] {
		return kv, PrivateAttribute(kv)
	}
	switch kv.Value.Type() {
	case attribute.STRING:
		return attribute.String(key, SanitizeLogText(kv.Value.AsString())), true
	case attribute.BOOL, attribute.INT64:
		return kv, true
	case attribute.FLOAT64:
		return kv, finitePrivateNumber(kv.Value.AsFloat64())
	}
	return kv, false
}

// sanitizePrivateURL takes an absolute or relative endpoint and returns its route
// without credentials, query values or fragment; malformed endpoints fail closed.
func sanitizePrivateURL(raw string) string {
	if len(raw) > maxPrivateTextBytes {
		return "[REDACTED URL]"
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return "[REDACTED data URL]"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[REDACTED URL]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	if parsed.Opaque != "" {
		return "[REDACTED URL]"
	}
	parts := strings.Split(parsed.Path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if privateSensitiveField(parts[i]) {
			parts[i+1] = "REDACTED"
		}
	}
	parsed.Path = privateKeyPattern.ReplaceAllString(redactPrivateAssignments(strings.Join(parts, "/")), "REDACTED")
	parsed.RawPath = ""
	return parsed.String()
}

// redactPrivateAssignments removes recognized labeled values, including quoted
// spaces/escapes. An unterminated quote consumes the remainder instead of leaking
// a secret suffix when input was malformed or cut by the sanitizer's byte cap.
func redactPrivateAssignments(value string) string {
	var result strings.Builder
	for {
		match := privateAssignmentPattern.FindStringIndex(value)
		if match == nil {
			result.WriteString(value)
			return result.String()
		}
		result.WriteString(value[:match[1]])
		result.WriteString(privateReplacement)
		rest := value[match[1]:]
		end := 0
		if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
			quote := rest[0]
			end = len(rest)
			for i := 1; i < len(rest); i++ {
				if rest[i] == '\\' {
					i++
					continue
				}
				if rest[i] == quote {
					end = i + 1
					break
				}
			}
		} else {
			for end < len(rest) && !strings.ContainsRune(" \t\r\n,;", rune(rest[end])) {
				end++
			}
		}
		value = rest[end:]
	}
}

// privateReservedField prevents record attributes from impersonating fixed
// deployment identity, trusted Zap caller metadata or sanitizer status.
func privateReservedField(key string) bool {
	switch privateFieldName(key) {
	case "servicename", "serviceversion", "hostname", "deploymentenvironment",
		"deploymentenvironmentname", "telemetrysdkname", "telemetrysdklanguage",
		"telemetrysdkversion", "processruntimename", "processruntimeversion", "processpid",
		"codefilepath", "codefunctionname", "codelinenumber", "logfieldsomitted":
		return true
	}
	return false
}
