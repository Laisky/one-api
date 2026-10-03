package message

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
)

// ResendEmailRequest is the JSON body sent to POST https://api.resend.com/emails.
type ResendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
}

// ResendEmailResponse models both the success and error payloads returned by the Resend API.
// Success: {"id": "..."}. Error: {"statusCode": 422, "name": "validation_error", "message": "..."}.
type ResendEmailResponse struct {
	ID         string `json:"id,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Name       string `json:"name,omitempty"`
	Message    string `json:"message,omitempty"`
}

// resendAPIURL is the Resend emails endpoint. Tests may override it to point at httptest.
var resendAPIURL = "https://api.resend.com/emails"

// resendMaxRecipientsPerRequest is the maximum number of addresses Resend accepts
// in the "to" field of a single send request. Larger lists are split into batches.
// See https://resend.com/docs/api-reference/emails/send-email ("to ... Max 50").
const resendMaxRecipientsPerRequest = 50

// resendHTTPClient is shared across Resend sends to reuse TCP connections.
var resendHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// loginAuth implements the LOGIN authentication mechanism
type loginAuth struct {
	username, password string
}

// LoginAuth returns an Auth that implements the LOGIN authentication mechanism
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

type plainAuthCompat struct {
	identity, username, password, host string
}

func newPlainAuth(identity, username, password, host string) smtp.Auth {
	return &plainAuthCompat{identity: identity, username: username, password: password, host: host}
}

func isLocalhost(name string) bool {
	switch strings.ToLower(name) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// Start implements smtp.Auth for the PLAIN mechanism, validating the server identity before proceeding.
func (a *plainAuthCompat) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil {
		return "", nil, errors.New("missing SMTP server info for PLAIN auth")
	}
	if server.Name != a.host {
		return "", nil, errors.Errorf("unexpected SMTP server name: got %s, want %s", server.Name, a.host)
	}
	if !server.TLS && config.ForceEmailTLSVerify && !isLocalhost(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}

	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

// Next completes the PLAIN authentication exchange, returning an error on unexpected challenges.
func (a *plainAuthCompat) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.Errorf("unexpected server challenge: %s", string(fromServer))
	}
	return nil, nil
}

// Start implements smtp.Auth for the LOGIN mechanism.
// It refuses to proceed unless TLS is active to prevent sending credentials over plaintext connections.
func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil {
		return "", nil, errors.New("missing SMTP server info for LOGIN auth")
	}

	if !server.TLS && config.ForceEmailTLSVerify {
		return "", nil, errors.Errorf("refusing LOGIN without TLS")
	}

	return "LOGIN", []byte{}, nil
}

// Next responds to server challenges for the LOGIN mechanism.
// It supplies the username and password when prompted, and returns an error for unexpected challenges.
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:", "username:":
			return []byte(a.username), nil
		case "Password:", "password:":
			return []byte(a.password), nil
		default:
			return nil, errors.Errorf("unexpected server challenge: %s", string(fromServer))
		}
	}
	return nil, nil
}

func shouldAuth() bool {
	return config.SMTPAccount != "" || config.SMTPToken != ""
}

// resendSenderAddress builds the RFC 5322 "from" header for Resend.
// Resend requires the address to be on a verified domain. It prefers SMTPFrom,
// falls back to SMTPAccount, and attaches the SystemName as the display name when
// the base address does not already carry one. net/mail handles the quoting/encoding
// so display names containing commas, quotes, or non-ASCII characters stay valid
// (e.g. `"Acme, Inc." <noreply@acme.com>`); a raw concatenation would produce a
// malformed mailbox that Resend rejects with invalid_from_address.
func resendSenderAddress() (string, error) {
	fromAddr := strings.TrimSpace(config.SMTPFrom)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(config.SMTPAccount)
	}
	if fromAddr == "" {
		return "", errors.New("Resend sender address is not configured (set SMTPFrom or SMTPAccount)")
	}

	addr, err := mail.ParseAddress(fromAddr)
	if err != nil {
		return "", errors.Wrapf(err, "invalid Resend sender address %q", fromAddr)
	}
	if name := strings.TrimSpace(config.SystemName); name != "" && addr.Name == "" {
		addr.Name = name
	}

	return addr.String(), nil
}

// sendEmailViaResend posts an HTML email through the Resend HTTP API.
// It returns an error when the API key is missing, no recipients are valid,
// the sender address is unconfigured, the HTTP request fails, or Resend responds
// with a non-2xx status. Recipient lists longer than resendMaxRecipientsPerRequest
// are split into multiple requests so a large notification is never rejected wholesale.
func sendEmailViaResend(subject, receiver, content string) error {
	if config.ResendAPIKey == "" {
		return errors.New("Resend API key is not configured")
	}

	emails := []string{}
	for email := range strings.SplitSeq(receiver, ";") {
		email = strings.TrimSpace(email)
		if email != "" {
			emails = append(emails, email)
		}
	}

	if len(emails) == 0 {
		return errors.New("no valid recipient email addresses")
	}

	from, err := resendSenderAddress()
	if err != nil {
		return err
	}

	for start := 0; start < len(emails); start += resendMaxRecipientsPerRequest {
		end := start + resendMaxRecipientsPerRequest
		if end > len(emails) {
			end = len(emails)
		}
		if err := postResendEmail(from, emails[start:end], subject, content); err != nil {
			return err
		}
	}

	return nil
}

// postResendEmail sends a single Resend request for at most resendMaxRecipientsPerRequest recipients.
func postResendEmail(from string, to []string, subject, content string) error {
	reqBody := ResendEmailRequest{
		From:    from,
		To:      to,
		Subject: subject,
		Html:    content,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return errors.Wrap(err, "failed to marshal Resend request")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to create Resend request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.ResendAPIKey)

	resp, err := resendHTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send request to Resend API")
	}
	defer resp.Body.Close()

	// Read the body once so we can include it in error messages even when it isn't JSON
	// (e.g. an HTML error page from an upstream proxy or Railway edge).
	rawBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if readErr != nil {
		return errors.Wrapf(readErr, "failed to read Resend response (status %d)", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var res ResendEmailResponse
		if jsonErr := json.Unmarshal(rawBody, &res); jsonErr == nil && res.Message != "" {
			return errors.Errorf("Resend API error (status %d, %s): %s", resp.StatusCode, res.Name, res.Message)
		}
		return errors.Errorf("Resend API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var res ResendEmailResponse
	if err := json.Unmarshal(rawBody, &res); err != nil {
		return errors.Wrapf(err, "failed to decode Resend success response: %s", strings.TrimSpace(string(rawBody)))
	}
	if res.ID == "" {
		return errors.Errorf("Resend API returned success status %d but no message id; body: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	return nil
}

// dialSMTPClient establishes a connection to the SMTP server, preferring implicit TLS when available.
// It falls back to a plain connection with STARTTLS when the server does not accept immediate TLS.
// localName is used for the EHLO/HELO greeting and should be a hostname (e.g., the sender domain or "localhost").
// It returns the last observed AUTH mechanisms advertised by the server (if any) to aid mechanism selection.
func dialSMTPClient(ctx context.Context, addr, localName string) (*smtp.Client, string, bool, error) {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: !config.ForceEmailTLSVerify,
		ServerName:         config.SMTPServer,
	}

	tlsConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err == nil {
		client, clientErr := smtp.NewClient(tlsConn, config.SMTPServer)
		if clientErr != nil {
			tlsConn.Close()
			return nil, "", false, errors.Wrap(clientErr, "failed to create SMTP client with implicit TLS")
		}
		// Say EHLO/HELO before attempting any extensions
		if helloErr := client.Hello(localName); helloErr != nil {
			client.Close()
			return nil, "", false, errors.Wrap(helloErr, "failed to send EHLO on implicit TLS connection")
		}
		var authMechs string
		if ok, params := client.Extension("AUTH"); ok {
			authMechs = params
		}
		return client, authMechs, true, nil
	}

	conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
	if dialErr != nil {
		return nil, "", false, errors.Wrapf(dialErr, "failed to connect to SMTP server after TLS attempt: %v", err)
	}

	client, clientErr := smtp.NewClient(conn, config.SMTPServer)
	if clientErr != nil {
		conn.Close()
		return nil, "", false, errors.Wrap(clientErr, "failed to create SMTP client")
	}

	// Say EHLO/HELO to populate extensions
	if helloErr := client.Hello(localName); helloErr != nil {
		client.Close()
		return nil, "", false, errors.Wrap(helloErr, "failed to send EHLO to SMTP server")
	}

	var authMechs string
	if ok, params := client.Extension("AUTH"); ok {
		authMechs = params
	}

	usingTLS := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		if startTLSErr := client.StartTLS(tlsConfig); startTLSErr != nil {
			client.Close()
			return nil, "", false, errors.Wrap(startTLSErr, "failed to negotiate STARTTLS")
		}
		usingTLS = true
		// Note: net/smtp will internally handle the necessary EHLO state after STARTTLS.
	} else if shouldAuth() && config.ForceEmailTLSVerify {
		client.Close()
		return nil, "", false, errors.New("SMTP server does not advertise STARTTLS, refusing to authenticate without TLS")
	}

	return client, authMechs, usingTLS, nil
}

// SendEmail transmits an HTML email using Resend API or SMTP server.
// It returns an error when the message cannot be constructed or delivered.
func SendEmail(subject string, receiver string, content string) error {
	if receiver == "" {
		return errors.Errorf("receiver is empty")
	}

	// Resolve provider: explicit EmailProvider wins; otherwise fall back to legacy
	// auto-detection (Resend if API key present, else SMTP) for backwards compatibility.
	provider := strings.ToLower(strings.TrimSpace(config.EmailProvider))
	if provider == "" {
		if config.ResendAPIKey != "" {
			provider = "resend"
		} else {
			provider = "smtp"
		}
	}
	switch provider {
	case "resend":
		return sendEmailViaResend(subject, receiver, content)
	case "smtp":
		// fall through to SMTP path below
	default:
		return errors.Errorf("unknown email provider %q (expected \"smtp\" or \"resend\")", provider)
	}

	if config.SMTPFrom == "" { // for compatibility
		config.SMTPFrom = config.SMTPAccount
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))

	// Extract domain from SMTPFrom with fallback
	domain := "localhost"
	parts := strings.Split(config.SMTPFrom, "@")
	if len(parts) > 1 && parts[1] != "" {
		domain = parts[1]
	}

	// Generate a unique Message-ID
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return errors.Wrap(err, "failed to generate random bytes for Message-ID")
	}
	messageId := fmt.Sprintf("<%x@%s>", buf, domain)

	mail := fmt.Appendf(nil, "To: %s\r\n"+
		"From: %s<%s>\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+
		"Date: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, config.SystemName, config.SMTPFrom, encodedSubject, messageId, time.Now().Format(time.RFC1123Z), content)

	addr := net.JoinHostPort(config.SMTPServer, fmt.Sprintf("%d", config.SMTPPort))

	// Clean up recipient addresses
	receiverEmails := []string{}
	for email := range strings.SplitSeq(receiver, ";") {
		email = strings.TrimSpace(email)
		if email != "" {
			receiverEmails = append(receiverEmails, email)
		}
	}

	if len(receiverEmails) == 0 {
		return errors.New("no valid recipient email addresses")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// use the domain extracted above as EHLO local name; fallback is "localhost"
	client, preAuthMechs, usingTLS, err := dialSMTPClient(ctx, addr, domain)
	if err != nil {
		return errors.Wrap(err, "dial SMTP client")
	}
	defer client.Close()

	// Authenticate if credentials are provided
	if shouldAuth() {
		mechSet := make(map[string]struct{})
		addMechanisms := func(raw string) {
			for token := range strings.FieldsSeq(strings.ToUpper(raw)) {
				mechSet[token] = struct{}{}
			}
		}
		addMechanisms(preAuthMechs)
		if ok, params := client.Extension("AUTH"); ok {
			addMechanisms(params)
		}

		preferred := []string{"PLAIN", "LOGIN"}
		if !usingTLS {
			preferred = []string{"LOGIN", "PLAIN"}
		}

		var chosen string
		for _, candidate := range preferred {
			if _, ok := mechSet[candidate]; ok {
				chosen = candidate
				break
			}
		}
		if chosen == "" {
			if len(mechSet) > 0 {
				for mech := range mechSet {
					chosen = mech
					break
				}
			} else {
				chosen = preferred[0]
			}
		}

		var auth smtp.Auth
		switch chosen {
		case "LOGIN":
			auth = LoginAuth(config.SMTPAccount, config.SMTPToken)
		case "PLAIN":
			auth = newPlainAuth("", config.SMTPAccount, config.SMTPToken, config.SMTPServer)
		default:
			auth = newPlainAuth("", config.SMTPAccount, config.SMTPToken, config.SMTPServer)
		}

		if err = client.Auth(auth); err != nil {
			var fallbackAuth smtp.Auth
			switch auth.(type) {
			case *loginAuth:
				fallbackAuth = newPlainAuth("", config.SMTPAccount, config.SMTPToken, config.SMTPServer)
			case *plainAuthCompat:
				fallbackAuth = LoginAuth(config.SMTPAccount, config.SMTPToken)
			}

			if fallbackAuth != nil {
				if retryErr := client.Auth(fallbackAuth); retryErr == nil {
					goto afterAuth
				}
			}
			return errors.Wrap(err, "SMTP authentication failed")
		}
	afterAuth:
	}

	if err = client.Mail(config.SMTPFrom); err != nil {
		return errors.Wrap(err, "failed to set MAIL FROM")
	}

	for _, receiver := range receiverEmails {
		if err = client.Rcpt(receiver); err != nil {
			return errors.Wrapf(err, "failed to add recipient: %s", receiver)
		}
	}

	w, err := client.Data()
	if err != nil {
		return errors.Wrap(err, "failed to create message data writer")
	}

	if _, err = w.Write(mail); err != nil {
		return errors.Wrap(err, "failed to write email content")
	}

	if err = w.Close(); err != nil {
		return errors.Wrap(err, "failed to close message data writer")
	}

	return nil
}
