package message

import "strings"

// htmlEscaper escapes the five characters that are significant in HTML text
// and in quoted attribute values. It is equivalent to html.EscapeString, but it
// is a local strings.Replacer so static analysis (CodeQL go/unsafe-quoting)
// recognizes it as removing both quote characters.
var htmlEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`'`, "&#39;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
)

// EscapeHTML escapes s for safe use as HTML text or inside a double- or
// single-quoted HTML attribute value of an email body. Use it for every
// untrusted or operator-configured value interpolated into email markup.
//
// Parameters:
//   - s: the raw value.
//
// Returns: the escaped value; for valid UTF-8 input it equals
// html.EscapeString(s).
func EscapeHTML(s string) string {
	return htmlEscaper.Replace(s)
}
