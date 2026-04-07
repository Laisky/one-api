package message

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/config"
)

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

// SendEmail transmits an HTML email using the configured SMTP server, authenticating when credentials are provided.
// It returns an error when the message cannot be constructed or delivered.
func SendEmail(subject string, receiver string, content string) error {
	if receiver == "" {
		return errors.Errorf("receiver is empty")
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
