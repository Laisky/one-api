package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestQuotaReminderEmailEscapesServerAddress verifies that an operator-supplied
// server address containing quotes or markup cannot break out of the top-up
// link's href attribute or inject markup into the reminder email.
func TestQuotaReminderEmailEscapesServerAddress(t *testing.T) {
	const hostile = `https://example.com/" onmouseover="alert(1)' data-x='<b>x</b>`
	content := quotaReminderEmailContent("Quota Reminder", "Your quota is about to be exhausted", 42, hostile)
	require.NotContains(t, content, `" onmouseover="`, "a double quote must not close the href attribute")
	require.NotContains(t, content, `' data-x='`, "a single quote must not open a new attribute")
	require.NotContains(t, content, `<b>x</b>`, "markup in the address must be escaped")
	require.Contains(t, content, `href="https://example.com/&#34; onmouseover=&#34;alert(1)&#39; data-x=&#39;&lt;b&gt;x&lt;/b&gt;/topup"`)

	plain := quotaReminderEmailContent("Quota Reminder", "Your quota has been exhausted", 0, "https://one-api.example.com")
	require.Contains(t, plain, `href="https://one-api.example.com/topup"`, "a normal address is rendered unchanged")
	require.Contains(t, plain, "<strong>0</strong>")
}
