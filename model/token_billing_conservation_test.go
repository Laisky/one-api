package model

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestBillingAuditMultipleTokensShareOneBalance proves that different tokens,
// including unlimited tokens, cannot each reserve the same user's last funds.
func TestBillingAuditMultipleTokensShareOneBalance(t *testing.T) {
	for _, unlimited := range []bool{false, true} {
		t.Run(fmt.Sprint(unlimited), func(t *testing.T) {
			setupTestDatabase(t)
			user, first := billingAuditRows(t, 1000, false)
			second := &Token{UserId: user.Id, Key: first.Key + "-second", Name: first.Name + "-second",
				Status: TokenStatusEnabled, RemainQuota: 1000, UnlimitedQuota: unlimited}
			require.NoError(t, DB.Create(second).Error)
			tokens := []*Token{first, second}
			var successes [2]atomic.Int64
			var workers sync.WaitGroup
			start := make(chan struct{})
			for i := range 40 {
				index := i % 2
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					if err := PreConsumeTokenQuota(context.Background(), tokens[index].Id, 60); err == nil {
						successes[index].Add(1)
					}
				}()
			}
			close(start)
			workers.Wait()
			require.EqualValues(t, 16, successes[0].Load()+successes[1].Load())
			for index, token := range tokens {
				used := successes[index].Load() * 60
				if token.UnlimitedQuota {
					used = 0
				}
				requireBillingAuditBalances(t, user, token, 40, 1000-used, used)
			}
		})
	}
}

// TestBillingAuditSequenceConservation compares real SQL balances against an
// independent integer ledger after every admission, incremental charge, refund,
// and final settlement. The seeded sequence is deterministic and reproducible.
func TestBillingAuditSequenceConservation(t *testing.T) {
	for _, unlimited := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			t.Run(fmt.Sprintf("unlimited=%v/batch=%v", unlimited, batch), func(t *testing.T) {
				setupTestDatabase(t)
				previous := config.BatchUpdateEnabled
				config.BatchUpdateEnabled = batch
				t.Cleanup(func() { config.BatchUpdateEnabled = previous })
				const initial = int64(5000)
				user, token := billingAuditRows(t, initial, unlimited)
				var charged int64
				admitted, rejected := 0, 0
				rng := rand.New(rand.NewSource(20260917))
				assertBalances := func() {
					used := charged
					if unlimited {
						used = 0
					}
					requireBillingAuditBalances(t, user, token, initial-charged, initial-used, used)
				}
				for range 200 {
					reservation := int64(rng.Intn(50) + 1)
					before := charged
					err := PreConsumeTokenQuota(context.Background(), token.Id, reservation)
					if initial-before < reservation {
						require.Error(t, err)
						rejected++
						assertBalances()
						continue
					}
					require.NoError(t, err)
					admitted++
					charged += reservation
					assertBalances()
					increment := int64(rng.Intn(10))
					err = PostConsumeTokenQuota(context.Background(), token.Id, increment)
					if initial-charged < increment {
						require.Error(t, err)
						increment = 0
					} else {
						require.NoError(t, err)
						charged += increment
					}
					assertBalances()
					actual := int64(rng.Intn(150))
					require.NoError(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, actual-reservation-increment))
					charged = before + actual
					assertBalances()
				}
				require.Positive(t, admitted)
				require.Positive(t, rejected)
			})
		}
	}
}
