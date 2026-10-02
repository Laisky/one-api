package model

// Site-wide quota statistics caching (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.5).
//
// GetSiteWideQuotaStats is a SUM/COUNT over the whole users table with no
// predicate an index can serve. It was executed on every root dashboard load,
// so at a million users each refresh scanned a million rows before any of the
// consume-log aggregates even started.
//
// The cache is in-process on purpose: it must work in the default
// zero-dependency deployment, where there is no Redis, and the value is a
// process-wide aggregate that every node can compute identically.

import (
	"sync"
	"time"

	"github.com/Laisky/one-api/common/config"
)

// siteWideQuotaSnapshot is one cached result together with its expiry.
type siteWideQuotaSnapshot struct {
	totalQuota int64
	usedQuota  int64
	status     string
	expiresAt  time.Time
}

var (
	siteWideQuotaMu    sync.Mutex
	siteWideQuotaCache *siteWideQuotaSnapshot
	// siteWideQuotaNow is the clock, replaceable in tests.
	siteWideQuotaNow = time.Now
)

// GetSiteWideQuotaStatsCached returns site-wide quota statistics, recomputing
// them at most once per DASHBOARD_CACHE_TTL_SEC.
//
// A TTL of 0 disables caching and every call recomputes, preserving the
// pre-proposal behavior for operators who want it.
//
// Parameters: none.
//
// Return values:
//   - int64: total granted quota across non-deleted users.
//   - int64: total used quota across non-deleted users.
//   - string: a human-readable active-user summary.
//   - error: wrapped failure from the underlying aggregate.
func GetSiteWideQuotaStatsCached() (int64, int64, string, error) {
	ttl := config.DashboardCacheTTL()
	if ttl <= 0 {
		return GetSiteWideQuotaStats()
	}

	siteWideQuotaMu.Lock()
	defer siteWideQuotaMu.Unlock()

	now := siteWideQuotaNow()
	if siteWideQuotaCache != nil && now.Before(siteWideQuotaCache.expiresAt) {
		return siteWideQuotaCache.totalQuota, siteWideQuotaCache.usedQuota, siteWideQuotaCache.status, nil
	}

	totalQuota, usedQuota, status, err := GetSiteWideQuotaStats()
	if err != nil {
		// Serve a stale snapshot rather than failing the dashboard outright:
		// the aggregate is advisory, and a transient database hiccup should not
		// blank the page.
		if siteWideQuotaCache != nil {
			return siteWideQuotaCache.totalQuota, siteWideQuotaCache.usedQuota, siteWideQuotaCache.status, nil
		}
		return 0, 0, "", err
	}

	siteWideQuotaCache = &siteWideQuotaSnapshot{
		totalQuota: totalQuota,
		usedQuota:  usedQuota,
		status:     status,
		expiresAt:  now.Add(ttl),
	}
	return totalQuota, usedQuota, status, nil
}

// ResetSiteWideQuotaStatsCache clears the cached snapshot.
//
// Parameters: none.
//
// Return values: none.
func ResetSiteWideQuotaStatsCache() {
	siteWideQuotaMu.Lock()
	defer siteWideQuotaMu.Unlock()
	siteWideQuotaCache = nil
}
