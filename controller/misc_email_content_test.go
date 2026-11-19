package controller

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/Laisky/one-api/common/config"
)

// resetAnchors parses an email body and returns the attributes of every <a>
// element. Parameters: t is the test handle and body is the rendered HTML.
// Returns: one attribute map per anchor.
func resetAnchors(t *testing.T, body string) []map[string]string {
	t.Helper()
	tokenizer := html.NewTokenizer(strings.NewReader(body))
	var anchors []map[string]string
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return anchors
		case html.StartTagToken:
			token := tokenizer.Token()
			if token.Data != "a" {
				continue
			}
			attrs := map[string]string{}
			for _, attr := range token.Attr {
				attrs[attr.Key] = attr.Val
			}
			anchors = append(anchors, attrs)
		}
	}
}

// TestPasswordResetEmailContentEncodesAddress reproduces two defects of the
// reset email, which interpolated the raw address into the link: a valid
// plus-addressed email decoded as "a b@..." in the reset page (the "+" became a
// space), and a valid quoted local part containing `"` broke out of the href
// attribute and injected markup. The address must round-trip exactly and the
// anchor must keep only its own attributes. Parameters: t is the test handle.
// Returns: none.
func TestPasswordResetEmailContentEncodesAddress(t *testing.T) {
	previousServer, previousName := config.ServerAddress, config.SystemName
	config.ServerAddress, config.SystemName = "https://gateway.example", "R&D <Gateway>"
	t.Cleanup(func() { config.ServerAddress, config.SystemName = previousServer, previousName })

	for _, email := range []string{
		"alice+billing@example.com",
		`"a\" onmouseover=alert(1) x=\""@example.com`,
		`"a&token=attacker"@example.com`,
	} {
		body := passwordResetEmailContent("Reset", email, "token-123")
		anchors := resetAnchors(t, body)
		require.Len(t, anchors, 1, "email=%q", email)
		require.ElementsMatch(t, []string{"href", "style"}, keysOf(anchors[0]), "email=%q", email)

		link, err := url.Parse(anchors[0]["href"])
		require.NoError(t, err)
		require.Equal(t, "gateway.example", link.Host)
		require.Equal(t, "/user/reset", link.Path)
		require.Equal(t, url.Values{"email": {email}, "token": {"token-123"}}, link.Query(), "email=%q", email)
		// The link is HTML-escaped wherever it is rendered: the query separator
		// is serialized as "&amp;" in both the href attribute and the text.
		require.Equal(t, 2, strings.Count(body, "&amp;token=token-123"), "email=%q", email)
		require.NotContains(t, body, "<Gateway>")
		require.Contains(t, body, "R&amp;D &lt;Gateway&gt;")
	}
}

// TestEmailVerificationContentEscapesSystemName verifies the operator name is
// rendered as text in the verification email. Parameters: t is the test
// handle. Returns: none.
func TestEmailVerificationContentEscapesSystemName(t *testing.T) {
	previousName := config.SystemName
	config.SystemName = `<img src=x onerror=alert(1)>`
	t.Cleanup(func() { config.SystemName = previousName })

	body := emailVerificationContent("Verify", "123456")
	require.NotContains(t, body, "<img")
	require.Contains(t, body, "&lt;img src=x onerror=alert(1)&gt;")
	require.Contains(t, body, ">123456</p>")
}

// keysOf returns the keys of an attribute map. Parameters: attrs is the map.
// Returns: its keys in unspecified order.
func keysOf(attrs map[string]string) []string {
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	return keys
}
