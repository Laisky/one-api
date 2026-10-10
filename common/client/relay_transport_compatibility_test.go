package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// compatibilityRelayClient initializes the real relay transport for a fixture proxy and restores shared state afterward.
func compatibilityRelayClient(t *testing.T, proxy string, roots *x509.CertPool) *http.Client {
	t.Helper()
	oldProxy, oldContentProxy, oldTimeout := config.RelayProxy, config.UserContentRequestProxy, config.RelayTimeout
	oldClient, oldImpatient, oldContent := HTTPClient, ImpatientHTTPClient, UserContentRequestHTTPClient
	config.RelayProxy, config.UserContentRequestProxy, config.RelayTimeout = proxy, "", 5
	Init()
	transport := HTTPClient.Transport.(*http.Transport)
	require.NotNil(t, transport.TLSNextProto)
	require.Empty(t, transport.TLSNextProto)
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		UserContentRequestHTTPClient.CloseIdleConnections()
		config.RelayProxy, config.UserContentRequestProxy, config.RelayTimeout = oldProxy, oldContentProxy, oldTimeout
		HTTPClient, ImpatientHTTPClient, UserContentRequestHTTPClient = oldClient, oldImpatient, oldContent
	})
	return HTTPClient
}

// compatibilityServer serves bounded synthetic success, abort, and cancellation responses over TLS with HTTP/2 available.
func compatibilityServer(t *testing.T) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reset":
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("prefix")); err != nil {
				t.Error(err)
				return
			}
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		case "/cancel":
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
				t.Error("fixture cancellation did not reach server")
			}
		default:
			if _, err := w.Write([]byte("synthetic success")); err != nil {
				t.Error(err)
			}
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return server, roots
}

// compatibilitySuccess checks the response protocol and returns the traced socket used for a synthetic request.
func compatibilitySuccess(t *testing.T, client *http.Client, target string, proto int, reused bool) net.Conn {
	t.Helper()
	connections := make(chan httptrace.GotConnInfo, 1)
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { connections <- info },
	}), http.MethodGet, target, nil)
	require.NoError(t, err)
	response, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, proto, response.ProtoMajor)
	require.Equal(t, "synthetic success", string(body))
	info := <-connections
	require.Equal(t, reused, info.Reused)
	return info.Conn
}

// TestRelayTransportProtocolCompatibility verifies actual relay HTTP/1.1 negotiation, socket reuse, abort, and cancellation.
func TestRelayTransportProtocolCompatibility(t *testing.T) {
	server, roots := compatibilityServer(t)
	client := compatibilityRelayClient(t, "", roots)
	first := compatibilitySuccess(t, client, server.URL, 1, false)
	require.Same(t, first, compatibilitySuccess(t, client, server.URL, 1, true))
	response, err := client.Get(server.URL + "/reset")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.Error(t, err)
	require.Equal(t, "prefix", string(body))
	require.NoError(t, response.Body.Close())
	compatibilitySuccess(t, client, server.URL, 1, false)
	compatibilityCancellation(t, client, server.URL, 1)
	compatibilitySuccess(t, client, server.URL, 1, false)
}

// compatibilityCancellation checks that canceling after headers interrupts body reading with the original context cause.
func compatibilityCancellation(t *testing.T, client *http.Client, target string, proto int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target+"/cancel", nil)
	require.NoError(t, err)
	response, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, proto, response.ProtoMajor)
	cancel()
	_, err = io.ReadAll(response.Body)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, response.Body.Close())
}

// TestHTTP2DependencyProtocolCompatibility exercises the explicit x/net transport without enabling HTTP/2 on shared relay clients.
func TestHTTP2DependencyProtocolCompatibility(t *testing.T) {
	server, roots := compatibilityServer(t)
	transport := &http2.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	first := compatibilitySuccess(t, client, server.URL, 2, false)
	require.Same(t, first, compatibilitySuccess(t, client, server.URL, 2, true))
	response, err := client.Get(server.URL + "/reset")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "INTERNAL_ERROR")
	require.Equal(t, "prefix", string(body))
	require.NoError(t, response.Body.Close())
	require.Same(t, first, compatibilitySuccess(t, client, server.URL, 2, true))
	compatibilityCancellation(t, client, server.URL, 2)
	require.Same(t, first, compatibilitySuccess(t, client, server.URL, 2, true))
}

// compatibilityProxy creates a single bounded CONNECT or SOCKS5 tunnel restricted to the exact localhost fixture destination.
func compatibilityProxy(t *testing.T, scheme, destination string) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	result := make(chan error, 1)
	go func() {
		result <- compatibilityProxySession(listener, scheme, destination)
	}()
	return scheme + "://" + listener.Addr().String(), result
}

// compatibilityProxySession validates a proxy handshake, relays only to its fixture destination, and reports tunnel errors.
func compatibilityProxySession(listener net.Listener, scheme, destination string) error {
	conn, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("accept fixture proxy: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set fixture proxy deadline: %w", err)
	}
	reader := bufio.NewReader(conn)
	if scheme == "http" {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return fmt.Errorf("read CONNECT: %w", err)
		}
		if req.Method != http.MethodConnect || req.Host != destination {
			return fmt.Errorf("unexpected CONNECT destination")
		}
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return fmt.Errorf("write CONNECT response: %w", err)
		}
	} else {
		greeting := make([]byte, 2)
		if _, err := io.ReadFull(reader, greeting); err != nil {
			return fmt.Errorf("read SOCKS greeting: %w", err)
		}
		methods := make([]byte, int(greeting[1]))
		if _, err := io.ReadFull(reader, methods); err != nil {
			return fmt.Errorf("read SOCKS methods: %w", err)
		}
		if greeting[0] != 5 || !bytes.Contains(methods, []byte{0}) {
			return fmt.Errorf("unexpected SOCKS authentication")
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			return fmt.Errorf("write SOCKS greeting: %w", err)
		}
		header := make([]byte, 4)
		if _, err := io.ReadFull(reader, header); err != nil {
			return fmt.Errorf("read SOCKS request: %w", err)
		}
		if !bytes.Equal(header, []byte{5, 1, 0, 1}) {
			return fmt.Errorf("unexpected SOCKS request")
		}
		address := make([]byte, 6)
		if _, err := io.ReadFull(reader, address); err != nil {
			return fmt.Errorf("read SOCKS address: %w", err)
		}
		if net.JoinHostPort(net.IP(address[:4]).String(), fmt.Sprint(binary.BigEndian.Uint16(address[4:]))) != destination {
			return fmt.Errorf("unexpected SOCKS destination")
		}
		if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
			return fmt.Errorf("write SOCKS response: %w", err)
		}
	}
	upstream, err := net.DialTimeout("tcp", destination, 3*time.Second)
	if err != nil {
		return fmt.Errorf("dial fixture target: %w", err)
	}
	defer upstream.Close()
	if err := upstream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set upstream fixture deadline: %w", err)
	}
	var wait sync.WaitGroup
	wait.Add(1)
	copyResult := make(chan error, 1)
	go func() {
		defer wait.Done()
		_, err := io.Copy(upstream, reader)
		copyResult <- err
		upstream.Close()
	}()
	_, downstreamErr := io.Copy(conn, upstream)
	conn.Close()
	wait.Wait()
	upstreamErr := <-copyResult
	for _, err := range []error{upstreamErr, downstreamErr} {
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("copy fixture tunnel: %w", err)
		}
	}
	return nil
}

// TestRelayTransportProxyCompatibility verifies real relay ProxyURL routing and reuse through CONNECT and SOCKS5 tunnels.
func TestRelayTransportProxyCompatibility(t *testing.T) {
	for _, scheme := range []string{"http", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			server, roots := compatibilityServer(t)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			proxy, result := compatibilityProxy(t, scheme, target.Host)
			client := compatibilityRelayClient(t, proxy, roots)
			first := compatibilitySuccess(t, client, server.URL, 1, false)
			require.Same(t, first, compatibilitySuccess(t, client, server.URL, 1, true))
			client.CloseIdleConnections()
			require.NoError(t, <-result)
		})
	}
}

// compatibilityFramedResponse writes a bounded HTTP/2 response with exact synthetic header fields over a single TLS socket.
func compatibilityFramedResponse(listener net.Listener, fields []hpack.HeaderField) error {
	conn, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("accept framed fixture: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set framed fixture deadline: %w", err)
	}
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		return fmt.Errorf("read HTTP/2 preface: %w", err)
	}
	if string(preface) != http2.ClientPreface {
		return fmt.Errorf("unexpected HTTP/2 preface")
	}
	framer := http2.NewFramer(conn, conn)
	if err := framer.WriteSettings(); err != nil {
		return fmt.Errorf("write fixture SETTINGS: %w", err)
	}
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return fmt.Errorf("read fixture request frame: %w", err)
		}
		if headers, ok := frame.(*http2.HeadersFrame); ok {
			var block bytes.Buffer
			encoder := hpack.NewEncoder(&block)
			for _, field := range append([]hpack.HeaderField{{Name: ":status", Value: "200"}}, fields...) {
				if err := encoder.WriteField(field); err != nil {
					return fmt.Errorf("encode fixture header: %w", err)
				}
			}
			if err := framer.WriteHeaders(http2.HeadersFrameParam{StreamID: headers.StreamID, EndHeaders: true, EndStream: true, BlockFragment: block.Bytes()}); err != nil {
				return fmt.Errorf("write fixture headers: %w", err)
			}
			return nil
		}
	}
}

// TestHTTP2DependencyFramingHeaders checks exact patched sanitization semantics and a malformed header rejection control.
func TestHTTP2DependencyFramingHeaders(t *testing.T) {
	server, roots := compatibilityServer(t)
	for _, test := range []struct {
		name       string
		fields     []hpack.HeaderField
		wantLength string
		wantError  bool
	}{
		{name: "valid_length", fields: []hpack.HeaderField{{Name: "content-length", Value: "0"}}, wantLength: "0"},
		{name: "identical_lengths", fields: []hpack.HeaderField{{Name: "content-length", Value: "0"}, {Name: "content-length", Value: "0"}}, wantLength: "0"},
		{name: "conflicting_lengths", fields: []hpack.HeaderField{{Name: "content-length", Value: "0"}, {Name: "content-length", Value: "1"}}},
		{name: "invalid_length", fields: []hpack.HeaderField{{Name: "content-length", Value: "invalid"}}},
		{name: "connection_fields", fields: []hpack.HeaderField{{Name: "transfer-encoding", Value: "chunked"}, {Name: "connection", Value: "keep-alive"}, {Name: "keep-alive", Value: "timeout=5"}, {Name: "proxy-connection", Value: "keep-alive"}, {Name: "upgrade", Value: "h2c"}}},
		{name: "uppercase_rejected", fields: []hpack.HeaderField{{Name: "Invalid-Uppercase", Value: "synthetic"}}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rawListener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			require.NoError(t, rawListener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
			listener := tls.NewListener(rawListener, &tls.Config{Certificates: server.TLS.Certificates, NextProtos: []string{"h2"}, MinVersion: tls.VersionTLS12})
			t.Cleanup(func() { require.NoError(t, listener.Close()) })
			result := make(chan error, 1)
			go func() { result <- compatibilityFramedResponse(listener, test.fields) }()
			transport := &http2.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			response, err := client.Get("https://" + listener.Addr().String())
			if test.wantError {
				require.Error(t, err)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.Equal(t, 2, response.ProtoMajor)
				require.Equal(t, test.wantLength, response.Header.Get("Content-Length"))
				require.Equal(t, int64(0), response.ContentLength)
				require.LessOrEqual(t, len(response.Header.Values("Content-Length")), 1)
				for _, name := range []string{"Transfer-Encoding", "Connection", "Keep-Alive", "Proxy-Connection", "Upgrade"} {
					require.Empty(t, response.Header.Values(name))
				}
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.Empty(t, body)
				require.NoError(t, response.Body.Close())
			}
			require.NoError(t, <-result)
		})
	}
}
