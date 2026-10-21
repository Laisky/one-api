package controller

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestToolchainX509CertificateTrust verifies with t that the runtime rejects a
// publicly documented compromised CA while preserving ordinary certificate
// validation. It returns no values.
func TestToolchainX509CertificateTrust(t *testing.T) {
	t.Parallel()

	// Pin the verification time inside every fixture's validity period so that
	// certificate expiration cannot accidentally satisfy the distrust assertion.
	verifyTime := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)

	t.Run("distrusted_public_intermediate", func(t *testing.T) {
		t.Parallel()

		root := readToolchainX509Certificate(t, "emsign_root_tls_ca_g3.pem",
			"7dd78d5f4f13459a83dff9abbba62edbaf6f2d102bf257fd712f4d9f2746ed8d")
		intermediate := readToolchainX509Certificate(t, "em_dv_tls_ca_g3b.pem",
			"42c4accd12c4f59597925e1a08a7a90102608543809308d50ec427e118ba1493")
		spkiHash := sha256.Sum256(intermediate.RawSubjectPublicKeyInfo)
		require.Equal(t, "ebed6475c2c2d80fcb199f3fe8dd6e6b3ce60b77eb15d4e42c3dec0c6f5b5105", hex.EncodeToString(spkiHash[:]))
		require.NoError(t, root.CheckSignatureFrom(root))
		require.NoError(t, intermediate.CheckSignatureFrom(root))
		for _, cert := range []*x509.Certificate{root, intermediate} {
			require.True(t, cert.IsCA)
			require.True(t, verifyTime.After(cert.NotBefore))
			require.True(t, verifyTime.Before(cert.NotAfter))
		}

		roots := x509.NewCertPool()
		roots.AddCert(root)
		// Verify the real signed intermediate-to-root chain directly. This needs
		// only public certificates, never a compromised private key or server.
		// GO-2026-6809 applies to Certificate.Verify for any chain position.
		chains, err := intermediate.Verify(x509.VerifyOptions{
			Roots:       roots,
			CurrentTime: verifyTime,
			KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		require.Error(t, err, "GO-2026-6809: the toolchain must reject the compromised public intermediate")
		var invalid x509.CertificateInvalidError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, x509.NoValidChains, invalid.Reason)
		require.Empty(t, chains)
	})

	t.Run("valid_owned_certificate_chain", func(t *testing.T) {
		t.Parallel()

		root, rootKey := createToolchainX509Certificate(t, &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "one-api regression root"},
			NotBefore:             verifyTime.Add(-time.Hour),
			NotAfter:              verifyTime.Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}, nil, nil)
		intermediate, intermediateKey := createToolchainX509Certificate(t, &x509.Certificate{
			SerialNumber:          big.NewInt(2),
			Subject:               pkix.Name{CommonName: "one-api regression intermediate"},
			NotBefore:             verifyTime.Add(-time.Hour),
			NotAfter:              verifyTime.Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}, root, rootKey)
		leaf, _ := createToolchainX509Certificate(t, &x509.Certificate{
			SerialNumber: big.NewInt(3),
			NotBefore:    verifyTime.Add(-time.Hour),
			NotAfter:     verifyTime.Add(time.Hour),
			DNSNames:     []string{"one-api-regression.test"},
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}, intermediate, intermediateKey)
		roots := x509.NewCertPool()
		roots.AddCert(root)
		intermediates := x509.NewCertPool()
		intermediates.AddCert(intermediate)
		options := x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   verifyTime,
			DNSName:       "one-api-regression.test",
		}
		chains, err := leaf.Verify(options)
		require.NoError(t, err)
		require.Len(t, chains, 1)
		require.Equal(t, []*x509.Certificate{leaf, intermediate, root}, chains[0])

		// The valid control must still enforce the caller's configured trust.
		options.Roots = x509.NewCertPool()
		_, err = leaf.Verify(options)
		var unknownAuthority x509.UnknownAuthorityError
		require.ErrorAs(t, err, &unknownAuthority)
	})
}

// readToolchainX509Certificate reads the public PEM fixture named by filename,
// verifies its expected SHA-256 fingerprint with t, and returns the parsed cert.
func readToolchainX509Certificate(t *testing.T, filename, expectedSHA256 string) *x509.Certificate {
	t.Helper()

	data, err := os.ReadFile("testdata/toolchain_x509/" + filename)
	require.NoError(t, err)
	block, rest := pem.Decode(data)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)
	require.Empty(t, rest)
	fingerprint := sha256.Sum256(block.Bytes)
	require.Equal(t, expectedSHA256, hex.EncodeToString(fingerprint[:]))
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

// createToolchainX509Certificate creates a certificate from template with a fresh
// owned key, signed by parent and parentKey or self-signed when parent is nil.
// It reports failures through t and returns the parsed certificate and its key.
func createToolchainX509Certificate(t *testing.T, template, parent *x509.Certificate, parentKey crypto.Signer) (*x509.Certificate, crypto.Signer) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}
