package message

import (
	"html"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEscapeHTMLMatchesStandardLibrary verifies EscapeHTML escapes exactly like
// html.EscapeString, including both quote characters.
func TestEscapeHTMLMatchesStandardLibrary(t *testing.T) {
	for _, value := range []string{"", "plain", `a"b'c<d>e&f`, `https://x.test/?a=1&b="2"`, "你好 <script>"} {
		require.Equal(t, html.EscapeString(value), EscapeHTML(value), value)
	}
}
