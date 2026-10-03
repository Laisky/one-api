package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// withDashboardCacheTTL installs a dashboard cache TTL for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - seconds: the TTL to install.
//
// Return values: none.
func withDashboardCacheTTL(t *testing.T, seconds int) {
	t.Helper()
	prev := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = seconds
	t.Cleanup(func() { config.DashboardCacheTTLSec = prev })
}

// TestGetSiteWideQuotaStatsCachedServesFromCache verifies the expensive
// full-table aggregate runs at most once per TTL.
func TestGetSiteWideQuotaStatsCachedServesFromCache(t *testing.T) {
	setupTestDatabase(t)
	withDashboardCacheTTL(t, 60)
	ResetSiteWideQuotaStatsCache()
	t.Cleanup(ResetSiteWideQuotaStatsCache)

	total1, used1, status1, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)

	// Insert a user; a cached read must not observe it yet.
	user := &User{Username: "test-sitewide-cache", Password: "x", Quota: 12345, Status: UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, user.Id) })

	total2, used2, status2, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)
	require.Equal(t, total1, total2, "a cached read must not recompute")
	require.Equal(t, used1, used2)
	require.Equal(t, status1, status2)

	// Expire the snapshot; the next read must recompute and see the new user.
	siteWideQuotaMu.Lock()
	siteWideQuotaCache.expiresAt = time.Now().Add(-time.Second)
	siteWideQuotaMu.Unlock()

	total3, _, _, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)
	require.Equal(t, total1+12345, total3, "an expired snapshot must recompute")
}

// TestGetSiteWideQuotaStatsCachedDisabled verifies a zero TTL restores the
// pre-proposal behavior of recomputing on every call.
func TestGetSiteWideQuotaStatsCachedDisabled(t *testing.T) {
	setupTestDatabase(t)
	withDashboardCacheTTL(t, 0)
	ResetSiteWideQuotaStatsCache()
	t.Cleanup(ResetSiteWideQuotaStatsCache)

	total1, _, _, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)

	user := &User{Username: "test-sitewide-nocache", Password: "x", Quota: 777, Status: UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, user.Id) })

	total2, _, _, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)
	require.Equal(t, total1+777, total2, "with caching disabled every call must recompute")

	siteWideQuotaMu.Lock()
	cached := siteWideQuotaCache
	siteWideQuotaMu.Unlock()
	require.Nil(t, cached, "a disabled cache must not populate a snapshot")
}
