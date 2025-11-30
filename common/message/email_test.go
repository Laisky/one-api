package message

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
)

type smtpCapture struct {
	username string
	password string
	message  string
	err      error
}

func TestLoginAuthRequiresTLS(t *testing.T) {
	t.Parallel()

	prevVerify := config.ForceEmailTLSVerify
	defer func() {
		config.ForceEmailTLSVerify = prevVerify
	}()

	auth := LoginAuth("user", "pass")

	config.ForceEmailTLSVerify = true
	_, _, err := auth.Start(&smtp.ServerInfo{TLS: false})
	require.Error(t, err)

	mech, initial, err := auth.Start(&smtp.ServerInfo{TLS: true})
	require.NoError(t, err)
	require.Equal(t, "LOGIN", mech)
	require.Len(t, initial, 0)

	resp, err := auth.Next([]byte("Username:"), true)
	require.NoError(t, err)
	require.Equal(t, []byte("user"), resp)

	resp, err = auth.Next([]byte("Password:"), true)
	require.NoError(t, err)
	require.Equal(t, []byte("pass"), resp)

	resp, err = auth.Next(nil, false)
	require.NoError(t, err)
	require.Nil(t, resp)

	config.ForceEmailTLSVerify = false
	mech, initial, err = auth.Start(&smtp.ServerInfo{TLS: false})
	require.NoError(t, err)
	require.Equal(t, "LOGIN", mech)
	require.Len(t, initial, 0)
}

func TestSendEmailWithStartTLSFallback(t *testing.T) {
	cert := generateSelfSignedCert(t)

	var firstConn atomic.Int32
	captureCh := make(chan smtpCapture, 1)

	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		if firstConn.Add(1) == 1 {
			_ = conn.Close()
			return
		}

		capture, err := handleStartTLSSession(conn, cert)
		capture.err = err
		captureCh <- capture
	})
	defer shutdown()

	host, port := mustSplitAddr(t, addr)

	restore := overrideSMTPConfig(host, port, "sender@example.com", "user", "pass")
	defer restore()

	err := SendEmail("Test", "recipient@example.com", "hello world")
	require.NoError(t, err)

	select {
	case capture := <-captureCh:
		require.NoError(t, capture.err)
		require.Equal(t, "user", capture.username)
		require.Equal(t, "pass", capture.password)
		require.Contains(t, capture.message, "Subject: =?UTF-8?B?VGVzdA==?=")
		require.Contains(t, capture.message, "hello world")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for SMTP capture")
	}
}

func TestSendEmailWithoutSTARTTLSNoAuth(t *testing.T) {
	captureCh := make(chan smtpCapture, 1)
	var firstConn atomic.Int32

	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		if firstConn.Add(1) == 1 {
			_ = conn.Close()
			return
		}

		capture, err := handlePlainSession(conn)
		capture.err = err
		captureCh <- capture
	})
	defer shutdown()

	host, port := mustSplitAddr(t, addr)

	restore := overrideSMTPConfig(host, port, "sender@example.com", "", "")
	defer restore()

	err := SendEmail("NoTLS", "user@example.com", "body")
	require.NoError(t, err)

	select {
	case capture := <-captureCh:
		require.NoError(t, capture.err)
		require.Empty(t, capture.username)
		require.Empty(t, capture.password)
		require.Contains(t, capture.message, "Subject: =?UTF-8?B?Tm9UTFM=?=")
		require.Contains(t, capture.message, "body")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for SMTP capture")
	}
}

func TestSendEmailWithoutSTARTTLSAuthAllowed(t *testing.T) {
	captureCh := make(chan smtpCapture, 1)
	var firstConn atomic.Int32

	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		if firstConn.Add(1) == 1 {
			_ = conn.Close()
			return
		}

		capture, err := handlePlainSessionWithLogin(conn)
		capture.err = err
		captureCh <- capture
	})
	defer shutdown()

	host, port := mustSplitAddr(t, addr)

	restore := overrideSMTPConfig(host, port, "sender@example.com", "user", "pass")
	defer restore()

	err := SendEmail("LegacyAuth", "recipient@example.com", "legacy body")
	require.NoError(t, err)

	select {
	case capture := <-captureCh:
		require.NoError(t, capture.err)
		require.Equal(t, "user", capture.username)
		require.Equal(t, "pass", capture.password)
		require.Contains(t, capture.message, "Subject: =?UTF-8?B?TGVnYWN5QXV0aA==?=")
		require.Contains(t, capture.message, "legacy body")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for SMTP capture")
	}
}

func TestSendEmailWithoutSTARTTLSAuthFailsWhenVerifyEnabled(t *testing.T) {
	var firstConn atomic.Int32

	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		if firstConn.Add(1) == 1 {
			_ = conn.Close()
			return
		}
		_, _ = handlePlainSessionWithLogin(conn)
	})
	defer shutdown()

	host, port := mustSplitAddr(t, addr)

	restore := overrideSMTPConfig(host, port, "sender@example.com", "user", "pass")
	defer restore()

	config.ForceEmailTLSVerify = true

	err := SendEmail("LegacyAuth", "recipient@example.com", "legacy body")
	require.Error(t, err)
}

func TestSendEmailAuthPlainPreferred(t *testing.T) {
	cert := generateSelfSignedCert(t)

	captureCh := make(chan smtpCapture, 1)
	var firstConn atomic.Int32

	addr, shutdown := startMockSMTPServer(t, func(conn net.Conn) {
		if firstConn.Add(1) == 1 {
			_ = conn.Close()
			return
		}

		capture, err := handleStartTLSWithPlain(conn, cert)
		capture.err = err
		captureCh <- capture
	})
	defer shutdown()

	host, port := mustSplitAddr(t, addr)

	restore := overrideSMTPConfig(host, port, "sender@example.com", "user", "pass")
	defer restore()

	err := SendEmail("TestPlain", "recipient@example.com", "ping")
	require.NoError(t, err)

	select {
	case capture := <-captureCh:
		require.NoError(t, capture.err)
		require.Equal(t, "user", capture.username)
		require.Equal(t, "pass", capture.password)
		require.Contains(t, capture.message, "Subject: =?UTF-8?B?VGVzdFBsYWlu?=")
		require.Contains(t, capture.message, "ping")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for SMTP capture")
	}
}

func handleStartTLSWithPlain(rawConn net.Conn, cert tls.Certificate) (smtpCapture, error) {
	conn := rawConn
	defer func() { _ = conn.Close() }()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeLine(rw.Writer, "220 localhost ESMTP"); err != nil {
		return smtpCapture{}, err
	}

	line, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO, got %s", line)
	}

	for _, resp := range []string{"250-localhost", "250-STARTTLS", "250 AUTH PLAIN"} {
		if err := writeLine(rw.Writer, resp); err != nil {
			return smtpCapture{}, err
		}
	}

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.EqualFold(line, "STARTTLS") {
		return smtpCapture{}, fmt.Errorf("expected STARTTLS, got %s", line)
	}

	if err := writeLine(rw.Writer, "220 Ready to start TLS"); err != nil {
		return smtpCapture{}, err
	}

	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err := tlsConn.Handshake(); err != nil {
		return smtpCapture{}, err
	}
	if err := tlsConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return smtpCapture{}, err
	}

	conn = tlsConn
	rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO after TLS, got %s", line)
	}

	if err := writeLine(rw.Writer, "250 AUTH PLAIN"); err != nil {
		return smtpCapture{}, err
	}

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "AUTH PLAIN") {
		return smtpCapture{}, fmt.Errorf("expected AUTH PLAIN, got %s", line)
	}

	// Extract base64 part after AUTH PLAIN
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return smtpCapture{}, fmt.Errorf("malformed AUTH PLAIN: %s", line)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[2]))
	if err != nil {
		return smtpCapture{}, err
	}
	// PLAIN payload is: 0x00 username 0x00 password
	fields := bytes.Split(decoded, []byte{0})
	if len(fields) < 3 {
		return smtpCapture{}, fmt.Errorf("invalid PLAIN payload")
	}
	capture := smtpCapture{username: string(fields[1]), password: string(fields[2])}

	if err := writeLine(rw.Writer, "235 2.7.0 Authentication successful"); err != nil {
		return smtpCapture{}, err
	}

	message, err := handleDataExchange(rw)
	if err != nil {
		return smtpCapture{}, err
	}
	capture.message = message

	return capture, nil
}

func startMockSMTPServer(t *testing.T, handler func(net.Conn)) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					continue
				}
				return
			}

			go handler(conn)
		}
	}()

	shutdown := func() {
		_ = ln.Close()
		<-done
	}

	return ln.Addr().String(), shutdown
}

func handleStartTLSSession(rawConn net.Conn, cert tls.Certificate) (smtpCapture, error) {
	conn := rawConn
	defer func() {
		_ = conn.Close()
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeLine(rw.Writer, "220 localhost ESMTP"); err != nil {
		return smtpCapture{}, err
	}

	line, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO, got %s", line)
	}

	for _, resp := range []string{"250-localhost", "250-STARTTLS", "250 AUTH LOGIN"} {
		if err := writeLine(rw.Writer, resp); err != nil {
			return smtpCapture{}, err
		}
	}

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.EqualFold(line, "STARTTLS") {
		return smtpCapture{}, fmt.Errorf("expected STARTTLS, got %s", line)
	}

	if err := writeLine(rw.Writer, "220 Ready to start TLS"); err != nil {
		return smtpCapture{}, err
	}

	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err := tlsConn.Handshake(); err != nil {
		return smtpCapture{}, err
	}
	if err := tlsConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return smtpCapture{}, err
	}

	conn = tlsConn
	rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO after TLS, got %s", line)
	}

	if err := writeLine(rw.Writer, "250 AUTH LOGIN"); err != nil {
		return smtpCapture{}, err
	}

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "AUTH LOGIN") {
		return smtpCapture{}, fmt.Errorf("expected AUTH LOGIN, got %s", line)
	}

	if err := writeLine(rw.Writer, "334 VXNlcm5hbWU6"); err != nil {
		return smtpCapture{}, err
	}

	userLine, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	userBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(userLine))
	if err != nil {
		return smtpCapture{}, err
	}

	if err := writeLine(rw.Writer, "334 UGFzc3dvcmQ6"); err != nil {
		return smtpCapture{}, err
	}

	passLine, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	passBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(passLine))
	if err != nil {
		return smtpCapture{}, err
	}

	capture := smtpCapture{
		username: string(userBytes),
		password: string(passBytes),
	}

	if err := writeLine(rw.Writer, "235 2.7.0 Authentication successful"); err != nil {
		return smtpCapture{}, err
	}

	message, err := handleDataExchange(rw)
	if err != nil {
		return smtpCapture{}, err
	}
	capture.message = message

	return capture, nil
}

func handlePlainSession(rawConn net.Conn) (smtpCapture, error) {
	conn := rawConn
	defer func() {
		_ = conn.Close()
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeLine(rw.Writer, "220 localhost ESMTP"); err != nil {
		return smtpCapture{}, err
	}

	line, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO, got %s", line)
	}

	if err := writeLine(rw.Writer, "250 localhost"); err != nil {
		return smtpCapture{}, err
	}

	message, err := handleDataExchange(rw)
	if err != nil {
		return smtpCapture{}, err
	}

	return smtpCapture{message: message}, nil
}

func handlePlainSessionWithLogin(rawConn net.Conn) (smtpCapture, error) {
	conn := rawConn
	defer func() {
		_ = conn.Close()
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeLine(rw.Writer, "220 localhost ESMTP"); err != nil {
		return smtpCapture{}, err
	}

	line, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "EHLO ") {
		return smtpCapture{}, fmt.Errorf("expected EHLO, got %s", line)
	}

	for _, resp := range []string{"250-localhost", "250 AUTH LOGIN"} {
		if err := writeLine(rw.Writer, resp); err != nil {
			return smtpCapture{}, err
		}
	}

	line, err = readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "AUTH LOGIN") {
		return smtpCapture{}, fmt.Errorf("expected AUTH LOGIN, got %s", line)
	}

	if err := writeLine(rw.Writer, "334 VXNlcm5hbWU6"); err != nil {
		return smtpCapture{}, err
	}

	userLine, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	userBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(userLine))
	if err != nil {
		return smtpCapture{}, err
	}

	if err := writeLine(rw.Writer, "334 UGFzc3dvcmQ6"); err != nil {
		return smtpCapture{}, err
	}

	passLine, err := readLine(rw.Reader)
	if err != nil {
		return smtpCapture{}, err
	}
	passBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(passLine))
	if err != nil {
		return smtpCapture{}, err
	}

	capture := smtpCapture{
		username: string(userBytes),
		password: string(passBytes),
	}

	if err := writeLine(rw.Writer, "235 2.7.0 Authentication successful"); err != nil {
		return smtpCapture{}, err
	}

	message, err := handleDataExchange(rw)
	if err != nil {
		return smtpCapture{}, err
	}

	capture.message = message
	return capture, nil
}

func handleDataExchange(rw *bufio.ReadWriter) (string, error) {
	if err := expectCommand(rw, "MAIL FROM:", "250 2.0.0 Ok"); err != nil {
		return "", err
	}

	if err := expectCommand(rw, "RCPT TO:", "250 2.0.0 Ok"); err != nil {
		return "", err
	}

	if err := expectCommand(rw, "DATA", "354 End data with <CR><LF>.<CR><LF>"); err != nil {
		return "", err
	}

	var body bytes.Buffer
	for {
		line, err := readLine(rw.Reader)
		if err != nil {
			return "", err
		}
		if line == "." {
			break
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}

	if err := writeLine(rw.Writer, "250 2.0.0 Ok"); err != nil {
		return "", err
	}

	return body.String(), nil
}

func expectCommand(rw *bufio.ReadWriter, prefix, response string) error {
	line, err := readLine(rw.Reader)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToUpper(line), strings.ToUpper(prefix)) {
		return fmt.Errorf("expected %s, got %s", prefix, line)
	}

	return writeLine(rw.Writer, response)
}

func writeLine(w *bufio.Writer, line string) error {
	if _, err := w.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return w.Flush()
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func overrideSMTPConfig(host string, port int, from, account, token string) func() {
	prevServer := config.SMTPServer
	prevPort := config.SMTPPort
	prevFrom := config.SMTPFrom
	prevAccount := config.SMTPAccount
	prevToken := config.SMTPToken
	prevVerify := config.ForceEmailTLSVerify

	config.SMTPServer = host
	config.SMTPPort = port
	config.SMTPFrom = from
	config.SMTPAccount = account
	config.SMTPToken = token
	config.ForceEmailTLSVerify = false

	return func() {
		config.SMTPServer = prevServer
		config.SMTPPort = prevPort
		config.SMTPFrom = prevFrom
		config.SMTPAccount = prevAccount
		config.SMTPToken = prevToken
		config.ForceEmailTLSVerify = prevVerify
	}
}

func mustSplitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	return host, port
}

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := &bytes.Buffer{}
	require.NoError(t, pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}))

	keyPEM := &bytes.Buffer{}
	require.NoError(t, pem.Encode(keyPEM, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))

	cert, err := tls.X509KeyPair(certPEM.Bytes(), keyPEM.Bytes())
	require.NoError(t, err)

	return cert
}
