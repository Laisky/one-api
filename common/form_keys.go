package common

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// ErrAmbiguousFormKey marks a form body that repeats a scalar field or spells
// one field in more than one way. No standard says which copy of a repeated
// form field wins, and parsers disagree: Go and gin keep the first value,
// Starlette/FastAPI, Django, PHP and Rails the last, Express builds an array
// and ASP.NET matches names case-insensitively. The gateway validates and
// bills one reading while forwarding the raw body, so it refuses such bodies
// instead of picking a copy (OWASP ASVS 15.3.7, HTTP parameter pollution).
var ErrAmbiguousFormKey = errors.New("ambiguous form request parameter")

// maxReportedFormKeys and maxReportedFormKeyBytes bound the client-chosen
// field names echoed in an ErrAmbiguousFormKey message.
const (
	maxReportedFormKeys     = 8
	maxReportedFormKeyBytes = 64
)

// formArrayFields lists the array-typed form fields of the relayed endpoints,
// which clients legitimately repeat with or without a "[]" suffix: multi-image
// edits and the transcription list parameters. Every other field is a scalar.
var formArrayFields = map[string]bool{
	"image":                    true,
	"include":                  true,
	"timestamp_granularities":  true,
	"known_speaker_names":      true,
	"known_speaker_references": true,
	"languages":                true,
	"keywords":                 true,
}

// extendedNameParam matches an RFC 2231/5987 extended or continued "name"
// parameter (name*=, name*0=), which Go and other multipart parsers decode
// differently. An extended filename (filename*=, sent by .NET clients) does not
// name a field, so it stays allowed.
var extendedNameParam = regexp.MustCompile(`(?i);\s*name\*`)

// IsAmbiguousRequestError reports whether err rejects a request body that two
// parsers could read differently. Such a body is a client error on every
// endpoint, including endpoints that do not require a model. Parameters: err
// is a request decoding error. Returns: true for the ambiguity sentinels.
func IsAmbiguousRequestError(err error) bool {
	return errors.Is(err, ErrAmbiguousRequestBody) ||
		errors.Is(err, ErrAmbiguousFormKey) ||
		errors.Is(err, ErrAmbiguousJSONKey)
}

// validateParsedFormKeys checks the form fields of req's body after gin bound
// them. Parameters: req is the request and body its cached raw body. Returns:
// nil, or an error wrapping ErrAmbiguousFormKey. Multipart bodies are walked
// part by part, because Go's form parser silently drops parts that other
// parsers still read.
func validateParsedFormKeys(req *http.Request, body []byte) error {
	if req.MultipartForm == nil {
		counts := make(map[string]int, len(req.PostForm))
		for name, values := range req.PostForm {
			counts[name] += len(values)
		}
		return validateFormKeys(counts)
	}

	contentType := req.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" || strings.Contains(contentType, `\`) {
		// Go keeps a backslash before a non-special character while other
		// parsers drop it, so an escaped boundary splits the body differently.
		return errors.Wrap(ErrAmbiguousFormKey, "multipart body without an unambiguous boundary")
	}
	if multipartHeadersFolded(body, params["boundary"]) {
		return errors.Wrap(ErrAmbiguousFormKey, "folded multipart part header")
	}
	counts, err := multipartFieldCounts(body, params["boundary"])
	if err != nil {
		return err
	}
	return validateFormKeys(counts)
}

// multipartFieldCounts walks the raw parts of a multipart body. Parameters:
// body is the raw body and boundary its delimiter. Returns: the occurrences
// of each field name, or an error wrapping ErrAmbiguousFormKey for a part
// whose name a different parser could read differently: a missing, repeated,
// unparsable or non-form-data Content-Disposition, an extended parameter, or
// a transfer encoding that some parsers decode.
func multipartFieldCounts(body []byte, boundary string) (map[string]int, error) {
	counts := make(map[string]int)
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		// NextRawPart keeps quoted-printable parts encoded, as many parsers do.
		part, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			return counts, nil
		}
		if err != nil {
			return nil, errors.Wrap(ErrAmbiguousFormKey, "unreadable multipart part")
		}
		dispositions := part.Header.Values("Content-Disposition")
		// A backslash escape is kept by Go before a non-special character and
		// dropped by other parsers, so `name="\n"` names different fields.
		if len(dispositions) != 1 || extendedNameParam.MatchString(dispositions[0]) || strings.Contains(dispositions[0], `\`) {
			return nil, errors.Wrap(ErrAmbiguousFormKey, "ambiguous multipart Content-Disposition")
		}
		disposition, params, err := mime.ParseMediaType(dispositions[0])
		if err != nil || disposition != "form-data" || params["name"] == "" {
			return nil, errors.Wrap(ErrAmbiguousFormKey, "ambiguous multipart Content-Disposition")
		}
		switch strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Transfer-Encoding"))) {
		case "", "7bit", "8bit", "binary":
		default:
			return nil, errors.Wrap(ErrAmbiguousFormKey, "encoded multipart part")
		}
		counts[params["name"]]++
	}
}

// multipartHeadersFolded reports whether any part header line continues the
// previous line (obsolete line folding). Go joins folded lines while other
// parsers end the header there, so a folded parameter can name a field for
// one parser only. Parameters: body is the raw body and boundary its
// delimiter. Returns: true when any part header block contains a folded line.
func multipartHeadersFolded(body []byte, boundary string) bool {
	segments := bytes.Split(body, []byte("--"+boundary))
	for _, segment := range segments[min(1, len(segments)):] {
		if bytes.HasPrefix(segment, []byte("--")) {
			break // the close delimiter; the epilogue holds no parts
		}
		end := len(segment)
		if i := bytes.Index(segment, []byte("\r\n\r\n")); i >= 0 {
			end = i
		}
		if i := bytes.Index(segment, []byte("\n\n")); i >= 0 && i < end {
			end = i
		}
		// The first line holds the rest of the delimiter line, not a header.
		for _, line := range bytes.Split(segment[:end], []byte("\n"))[1:] {
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				return true
			}
		}
	}
	return false
}

// validateFormKeys rejects form fields that two parsers could read
// differently. Parameters: counts maps each exact field name to its number of
// occurrences. Returns: nil, or an error wrapping ErrAmbiguousFormKey when a
// name contains a control character, a scalar field carries the "[]" array
// suffix, or a scalar field occurs more than once, counting spellings that
// differ only in letter case or a trailing "[]" as the same field.
func validateFormKeys(counts map[string]int) error {
	occurrences := make(map[string]int, len(counts))
	var ambiguous []string
	for name, count := range counts {
		if strings.IndexFunc(name, isFormControlRune) >= 0 {
			return errors.Wrap(ErrAmbiguousFormKey, "form parameter name contains a control character")
		}
		// Upper then lower folds the case pairs ASP.NET-style matching merges,
		// including U+017F (long s) and U+212A (Kelvin sign).
		base := strings.TrimSuffix(name, "[]")
		key := strings.ToLower(strings.ToUpper(base))
		occurrences[key] += count
		if base != name && !formArrayFields[key] {
			// PHP, Rails and qs read "seconds[]" as the field "seconds", which
			// gin never binds from that spelling.
			ambiguous = append(ambiguous, key)
		}
	}
	for key, count := range occurrences {
		if count > 1 && !formArrayFields[key] {
			ambiguous = append(ambiguous, key)
		}
	}
	if len(ambiguous) == 0 {
		return nil
	}
	return errors.Wrapf(ErrAmbiguousFormKey, "form parameters sent more than once, in more than one spelling, or as an array: %s", reportFormKeys(ambiguous))
}

// isFormControlRune reports whether r is a control character. Parameters: r is
// one rune of a field name. Returns: true for C0, DEL and C1 controls.
func isFormControlRune(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

// reportFormKeys formats client-chosen field names for an error message.
// Parameters: keys are folded names, possibly repeated. Returns: a sorted,
// de-duplicated, quoted and bounded list.
func reportFormKeys(keys []string) string {
	sort.Strings(keys)
	unique := keys[:0]
	for i, key := range keys {
		if i == 0 || key != keys[i-1] {
			unique = append(unique, key)
		}
	}
	if len(unique) > maxReportedFormKeys {
		unique = unique[:maxReportedFormKeys]
	}
	quoted := make([]string, len(unique))
	for i, key := range unique {
		if len(key) > maxReportedFormKeyBytes {
			key = key[:maxReportedFormKeyBytes] + "..."
		}
		quoted[i] = strconv.Quote(key)
	}
	return strings.Join(quoted, ", ")
}
