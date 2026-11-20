package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// captureRootNotifications points the message pusher at a local server that
// records each notification's content, which notifyRootUser sends verbatim as
// the HTML email body when no pusher is configured. Parameters: t is the test
// handle. Returns: a channel of captured contents; the previous pusher address
// is restored on cleanup.
func captureRootNotifications(t *testing.T) <-chan string {
	t.Helper()
	contents := make(chan string, 4)
	pusher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			contents <- body.Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":""}`))
	}))
	previous := config.MessagePusherAddress
	config.MessagePusherAddress = pusher.URL
	t.Cleanup(func() {
		config.MessagePusherAddress = previous
		pusher.Close()
	})
	return contents
}

// TestChannelNotificationsEscapeUntrustedText reproduces HTML injection into
// the root user's channel notifications: the disable reason is provider error
// text (params.Err.Message on the relay path, or a probe error) and the channel
// label carries an administrator-entered name, and both were interpolated raw
// into the HTML body. Both must arrive HTML-escaped while the template markup is
// kept. Parameters: t is the test handle. Returns: none.
func TestChannelNotificationsEscapeUntrustedText(t *testing.T) {
	setupMonitorChannelDB(t)
	contents := captureRootNotifications(t)
	channel := insertMonitoredChannel(t, `ops <b onmouseover=alert(1)>`, model.ChannelStatusEnabled)
	providerReason := `credit exhausted: <a href="https://evil.example/login">re-authenticate</a>`

	NotifyChannelDisabled(channel.Id, channel.Name, providerReason)
	disabled := <-contents
	require.NotContains(t, disabled, `<a href="https://evil.example/login">`)
	require.NotContains(t, disabled, `<b onmouseover`)
	require.Contains(t, disabled, `credit exhausted: &lt;a href=&#34;https://evil.example/login&#34;&gt;re-authenticate&lt;/a&gt;`)
	require.Contains(t, disabled, `ops &lt;b onmouseover=alert(1)&gt;`)
	require.Contains(t, disabled, `<p>Reason for disabling:</p>`)

	NotifyChannelEnabled(channel.Id, channel.Name)
	enabled := <-contents
	require.NotContains(t, enabled, `<b onmouseover`)
	require.Contains(t, enabled, `ops &lt;b onmouseover=alert(1)&gt;`)
}

// TestChannelAnnouncementsNameTheStoredChannel verifies root notifications name
// the channel only as persisted: a caller-supplied (possibly stale) name is
// never rendered, and a row that cannot be read is announced by id alone.
// Parameters: t is the test handle. Returns: none.
func TestChannelAnnouncementsNameTheStoredChannel(t *testing.T) {
	setupMonitorChannelDB(t)
	contents := captureRootNotifications(t)
	channel := insertMonitoredChannel(t, "stored-name", model.ChannelStatusEnabled)

	NotifyChannelDisabled(channel.Id, "stale-caller-name", "invalid api key")
	stored := <-contents
	require.Contains(t, stored, "stored-name")
	require.NotContains(t, stored, "stale-caller-name")

	const missingID = 987654
	NotifyChannelDisabled(missingID, "caller-name-for-missing-row", "invalid api key")
	missing := <-contents
	require.Contains(t, missing, "channel#987654")
	require.NotContains(t, missing, "caller-name-for-missing-row")
}
