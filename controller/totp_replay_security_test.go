package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/stretchr/testify/require"
)

// TestSecurityTotpCodeSingleUseUnderConcurrency proves a valid TOTP code is
// accepted exactly once when many logins race with it. The replay cache must
// be safe for concurrent use and must check-and-mark atomically; otherwise
// several concurrent requests accept the same code (or the process crashes on
// a concurrent map write when Redis is disabled).
func TestSecurityTotpCodeSingleUseUnderConcurrency(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	t.Cleanup(cleanup)
	const secret = "JBSWY3DPEHPK3PXP"
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{Base32Secret: secret})
	require.NoError(t, err)

	for attempt := range 3 {
		uid := int(time.Now().UTC().UnixNano()%1_000_000_000) + 2_000_000 + attempt
		code := totp.Key()
		var accepted atomic.Int32
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				<-start
				if verifyTotpCode(context.Background(), uid, secret, code) {
					accepted.Add(1)
				}
			})
		}
		close(start)
		wg.Wait()
		if totp.Key() != code {
			// The 30-second window rolled over mid-test; retry with a fresh code.
			continue
		}
		require.Equal(t, int32(1), accepted.Load(), "a TOTP code was accepted more than once")
		return
	}
	t.Fatal("TOTP window rolled over on every attempt")
}
