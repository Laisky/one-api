package message

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// smtpWireReviewResult contains a completed plaintext session, not an optional
// observation. Tests fail if the server never reports completion.
type smtpWireReviewResult struct {
	lines []string
	err   error
}

// TestSMTPPlaintextAuthWireBoundary verifies the real SendEmail/dial path never
// sends AUTH, credentials, or mail when a server strips STARTTLS. Verification
// policy, advertised mechanism, and partially configured credentials cannot
// bypass the boundary. No timeout is interpreted as a successful observation.
func TestSMTPPlaintextAuthWireBoundary(t *testing.T) {
	for _, verify := range []bool{false, true} {
		for _, mechanisms := range []string{"PLAIN", "LOGIN", "PLAIN LOGIN"} {
			for _, credentials := range [][2]string{{"review-account-123", "review-secret-456"}, {"review-account-123", ""}, {"", "review-secret-456"}} {
				t.Run(fmt.Sprintf("verify-%t/%s/account-%t/token-%t", verify, mechanisms, credentials[0] != "", credentials[1] != ""), func(t *testing.T) {
					results := make(chan smtpWireReviewResult, 1)
					var connections atomic.Int32
					addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
						defer conn.Close()
						if connections.Add(1) == 1 {
							return // force the production implicit-TLS fallback path
						}
						result := smtpWireReviewResult{}
						defer func() { results <- result }()
						if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
							result.err = err
							return
						}
						rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
						if err := writeLine(rw.Writer, "220 localhost ESMTP"); err != nil {
							result.err = err
							return
						}
						for {
							line, err := readLine(rw.Reader)
							if err != nil {
								if !errors.Is(err, io.EOF) {
									result.err = err
								}
								return
							}
							result.lines = append(result.lines, line)
							if strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
								if err := writeLine(rw.Writer, "250-localhost"); err != nil {
									result.err = err
									return
								}
								if err := writeLine(rw.Writer, "250 AUTH "+mechanisms); err != nil {
									result.err = err
									return
								}
								continue
							}
							// An unsafe implementation reaches this response after
							// sending AUTH; the transcript assertions below fail.
							if err := writeLine(rw.Writer, "535 authentication rejected"); err != nil {
								result.err = err
								return
							}
						}
					})
					defer shutdown()
					host, port := mustSplitAddr(t, addr)
					restore := overrideSMTPConfig(host, port, "sender@example.com", credentials[0], credentials[1])
					defer restore()
					config.ForceEmailTLSVerify = verify
					err := SendEmail("TLS boundary", "recipient@example.com", "mail must not leave")
					require.ErrorContains(t, err, "SMTP server does not advertise STARTTLS")
					select {
					case result := <-results:
						require.NoError(t, result.err)
						require.Len(t, result.lines, 1, "only EHLO may be sent before rejecting plaintext authentication")
						require.True(t, strings.HasPrefix(strings.ToUpper(result.lines[0]), "EHLO "))
						wire := strings.Join(result.lines, "\n")
						require.NotContains(t, strings.ToUpper(wire), "AUTH ")
						for _, secret := range credentials {
							if secret != "" {
								require.NotContains(t, wire, secret)
								require.NotContains(t, wire, base64.StdEncoding.EncodeToString([]byte(secret)))
							}
						}
					case <-time.After(5 * time.Second):
						t.Fatal("SMTP wire capture did not finish; absence of evidence is not a passing security test")
					}
				})
			}
		}
	}
}

// TestSMTPAuthNoLoopbackException verifies neither localhost nor loopback IPs
// bypass the TLS rule; certificate verification remains an independent policy.
func TestSMTPAuthNoLoopbackException(t *testing.T) {
	previous := config.ForceEmailTLSVerify
	t.Cleanup(func() { config.ForceEmailTLSVerify = previous })
	for _, verify := range []bool{false, true} {
		config.ForceEmailTLSVerify = verify
		for _, host := range []string{"localhost", "127.0.0.1", "::1", "smtp.example.com"} {
			for _, auth := range []smtp.Auth{newPlainAuth("", "account", "secret", host), LoginAuth("account", "secret")} {
				name, initial, err := auth.Start(&smtp.ServerInfo{Name: host, TLS: false})
				require.Error(t, err)
				require.Empty(t, name)
				require.Empty(t, initial)
				name, _, err = auth.Start(&smtp.ServerInfo{Name: host, TLS: true})
				require.NoError(t, err)
				require.NotEmpty(t, name)
			}
		}
	}
}
