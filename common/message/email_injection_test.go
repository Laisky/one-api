package message

import (
	"bufio"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// lenientSMTPResult is what one lenient SMTP session observed.
type lenientSMTPResult struct {
	// greeted reports that the client sent EHLO/HELO; SendEmail's implicit-TLS
	// probe never does, so ungreeted sessions are not real deliveries.
	greeted bool
	rcpts   []string
	data    string
	err     error
}

// lenientSMTPSession serves one plaintext SMTP session that, like a permissive
// MTA, accepts every MAIL FROM and RCPT TO argument, and records the RCPT
// arguments and the DATA payload. Parameters: rawConn is the accepted
// connection. Returns: the recorded session.
func lenientSMTPSession(rawConn net.Conn) lenientSMTPResult {
	defer func() { _ = rawConn.Close() }()
	rw := bufio.NewReadWriter(bufio.NewReader(rawConn), bufio.NewWriter(rawConn))
	var result lenientSMTPResult
	if result.err = writeLine(rw.Writer, "220 localhost ESMTP"); result.err != nil {
		return result
	}
	for {
		line, err := readLine(rw.Reader)
		if err != nil {
			return result
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			result.greeted = true
			result.err = writeLine(rw.Writer, "250 localhost")
		case strings.HasPrefix(upper, "RCPT TO:"):
			result.rcpts = append(result.rcpts, line[len("RCPT TO:"):])
			result.err = writeLine(rw.Writer, "250 2.1.5 Ok")
		case upper == "DATA":
			if result.err = writeLine(rw.Writer, "354 End data with <CR><LF>.<CR><LF>"); result.err != nil {
				return result
			}
			var body strings.Builder
			for {
				dataLine, dataErr := readLine(rw.Reader)
				if dataErr != nil {
					result.err = dataErr
					return result
				}
				if dataLine == "." {
					break
				}
				body.WriteString(dataLine)
				body.WriteString("\n")
			}
			result.data = body.String()
			result.err = writeLine(rw.Writer, "250 2.0.0 Ok")
		case upper == "QUIT":
			_ = writeLine(rw.Writer, "221 Bye")
			return result
		default:
			result.err = writeLine(rw.Writer, "250 Ok")
		}
		if result.err != nil {
			return result
		}
	}
}

// startLenientSMTP starts a lenient SMTP server for SendEmail. SendEmail first
// probes for implicit TLS on every send; those probe connections never greet
// and are not reported. Parameters: t is the test handle. Returns: a channel
// receiving each greeted plaintext session and a counter of accepted
// connections; configuration and the listener are restored on cleanup.
func startLenientSMTP(t *testing.T) (<-chan lenientSMTPResult, *atomic.Int32) {
	t.Helper()
	sessions := make(chan lenientSMTPResult, 4)
	var connections atomic.Int32
	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		connections.Add(1)
		if session := lenientSMTPSession(conn); session.greeted {
			sessions <- session
		}
	})
	host, port := mustSplitAddr(t, addr)
	restoreSMTP := overrideSMTPConfig(host, port, "noreply@example.com", "", "")
	previousProvider, previousName := config.EmailProvider, config.SystemName
	config.EmailProvider = "smtp"
	t.Cleanup(func() {
		shutdown()
		restoreSMTP()
		config.EmailProvider, config.SystemName = previousProvider, previousName
	})
	return sessions, &connections
}

// awaitLenientSession waits for one recorded SMTP session. Parameters: t is
// the test handle and sessions is the server channel. Returns: the session.
func awaitLenientSession(t *testing.T, sessions <-chan lenientSMTPResult) lenientSMTPResult {
	t.Helper()
	select {
	case session := <-sessions:
		require.NoError(t, session.err)
		return session
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for the SMTP session")
		return lenientSMTPResult{}
	}
}

// messageHeaders returns the header block of a captured DATA payload, one
// header line per element. Parameters: data is the captured payload.
// Returns: the header lines before the first empty line.
func messageHeaders(data string) []string {
	head, _, _ := strings.Cut(data, "\n\n")
	return strings.Split(head, "\n")
}

// TestSendEmailRejectsRecipientHeaderInjection reproduces SMTP header and body
// injection through the recipient list: SendEmail split the receiver on ";",
// trimmed the CRLF that followed the separator, and wrote the raw receiver into
// the To header, so a permissive MTA delivered a message carrying an injected
// Bcc header or a forged body. Every recipient must now be a bare RFC 5322
// address, and an invalid one fails before any connection is made.
// Parameters: t is the test handle. Returns: none.
func TestSendEmailRejectsRecipientHeaderInjection(t *testing.T) {
	sessions, connections := startLenientSMTP(t)
	for _, receiver := range []string{
		"victim@example.com;\r\nBcc: hidden@evil.example",
		"victim@example.com;\r\n\r\n<h1>forged body</h1>",
		"victim@example.com\r\nBcc: hidden@evil.example",
		"Victim <victim@example.com>",
		"victim@example.com, other@example.com",
		"\"victim@example.com;x\"@evil.example",
	} {
		err := SendEmail("Subject", receiver, "<p>body</p>")
		require.Error(t, err, "receiver=%q", receiver)
	}
	require.Zero(t, connections.Load(), "invalid recipients must be rejected before dialing SMTP")
	select {
	case session := <-sessions:
		require.FailNow(t, "an invalid recipient reached SMTP", "data=%q", session.data)
	default:
	}
}

// TestSendEmailFormatsValidatedHeaders verifies a valid multi-recipient send
// renders RFC 5322 headers rebuilt by net/mail: the envelope and the To header
// list each validated address once, and the From display name is quoted, or
// RFC 2047 encoded when it holds non-ASCII text. Parameters: t is the test
// handle. Returns: none.
func TestSendEmailFormatsValidatedHeaders(t *testing.T) {
	sessions, _ := startLenientSMTP(t)
	for _, tc := range []struct {
		systemName string
		wantFrom   string
	}{
		{systemName: "R&D, Inc.", wantFrom: `From: "R&D, Inc." <noreply@example.com>`},
		{systemName: "网关", wantFrom: "From: =?utf-8?q?=E7=BD=91=E5=85=B3?= <noreply@example.com>"},
		{systemName: "", wantFrom: "From: <noreply@example.com>"},
	} {
		config.SystemName = tc.systemName
		require.NoError(t, SendEmail("Subject", " first@example.com ; second+tag@example.com ;", "<p>body</p>"))
		session := awaitLenientSession(t, sessions)
		require.Equal(t, []string{"<first@example.com>", "<second+tag@example.com>"}, session.rcpts)
		headers := messageHeaders(session.data)
		require.Contains(t, headers, "To: <first@example.com>, <second+tag@example.com>")
		require.Contains(t, headers, tc.wantFrom)
		require.Contains(t, session.data, "<p>body</p>")
	}
}

// TestSendEmailRejectsHeaderBreakingConfiguration reproduces header injection
// through operator settings: a SystemName or SMTPFrom containing CR/LF was
// written into the From and Message-ID headers verbatim. Such values are now
// refused before any connection is made. Parameters: t is the test handle.
// Returns: none.
func TestSendEmailRejectsHeaderBreakingConfiguration(t *testing.T) {
	_, connections := startLenientSMTP(t)
	config.SystemName = "Ops\r\nBcc: hidden@evil.example"
	require.Error(t, SendEmail("Subject", "victim@example.com", "<p>body</p>"))

	config.SystemName = "Ops"
	for _, sender := range []string{
		"noreply@example.com\r\nBcc: hidden@evil.example",
		"Ops <noreply@example.com>",
		"not-an-address",
	} {
		config.SMTPFrom = sender
		require.Error(t, SendEmail("Subject", "victim@example.com", "<p>body</p>"), "sender=%q", sender)
	}
	require.Zero(t, connections.Load())
}

// TestNotifyEscapesPlainTextForEmail reproduces HTML injection through Notify:
// its content is plain text (for example a provider's channel-test error) but
// was sent verbatim as a text/html body. It must arrive HTML-escaped.
// Parameters: t is the test handle. Returns: none.
func TestNotifyEscapesPlainTextForEmail(t *testing.T) {
	sessions, _ := startLenientSMTP(t)
	previousRoot := config.RootUserEmail
	config.RootUserEmail = "root@example.com"
	t.Cleanup(func() { config.RootUserEmail = previousRoot })

	providerText := `quota exceeded <a href="https://evil.example/login">sign in again</a>`
	require.NoError(t, Notify(ByEmail, "Channel test failed", "", providerText))
	session := awaitLenientSession(t, sessions)
	require.NotContains(t, session.data, `<a href="https://evil.example/login">`)
	require.Contains(t, session.data, `quota exceeded &lt;a href=&#34;https://evil.example/login&#34;&gt;sign in again&lt;/a&gt;`)
}

// TestEmailTemplateEscapesTextFields verifies the template treats its title and
// the operator SystemName as text while keeping the caller's HTML content.
// Parameters: t is the test handle. Returns: none.
func TestEmailTemplateEscapesTextFields(t *testing.T) {
	previousName := config.SystemName
	config.SystemName = `R&D <script>alert(1)</script>`
	t.Cleanup(func() { config.SystemName = previousName })

	rendered := EmailTemplate(`<img src=x onerror=alert(1)>`, "<p>trusted markup</p>")
	require.NotContains(t, rendered, "<img")
	require.NotContains(t, rendered, "<script>")
	require.Contains(t, rendered, "&lt;img src=x onerror=alert(1)&gt;")
	require.Contains(t, rendered, "R&amp;D &lt;script&gt;alert(1)&lt;/script&gt;")
	require.Contains(t, rendered, "<p>trusted markup</p>")
}
