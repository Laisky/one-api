package model

// Observability tests for compact UUID storage (AUTO-T24; evidence for AUTO-A06 and AUTO-A10).
//
// The observability schema in proposal section 11 is normative, and it is a statement about the
// series a SCRAPE sees. So these tests install the REAL Prometheus recorder and read the series
// back through the REAL default gatherer: a hand-rolled recorder asserting against its own
// bookkeeping would prove nothing about what a scrape reads.
//
// State transitions are driven through the real worker, not published by hand where a real
// driver exists — the trigger drops, invalid rows, and closed handles below are genuine faults,
// because the point is that a real fault becomes a real series within one scrape.
//
// The static forbidden-DDL half of this work lives in compact_uuid_forbidden_ddl_test.go.

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
	oneapiprom "github.com/Laisky/one-api/monitor/prometheus"
)

// compactMetricPrefix is the shared name prefix of every compact UUID metric family.
const compactMetricPrefix = "oneapi_compact_uuid_"

// Normative metric family names from proposal section 11.
const (
	// compactMetricState is the exactly-one-current-state-per-role gauge.
	compactMetricState = compactMetricPrefix + "state"
	// compactMetricBacklog is the last bounded gap/mismatch/blocker observation gauge.
	compactMetricBacklog = compactMetricPrefix + "backlog_rows"
	// compactMetricActions is the side-effect outcome counter.
	compactMetricActions = compactMetricPrefix + "actions_total"
	// compactMetricFallback is the runtime lookup fallback counter.
	compactMetricFallback = compactMetricPrefix + "lookup_fallback_total"
	// compactMetricLastProgress is the UTC last-durable-progress gauge.
	compactMetricLastProgress = compactMetricPrefix + "last_progress_unixtime"
	// compactMetricDuration is the timed-operation histogram.
	compactMetricDuration = compactMetricPrefix + "duration_seconds"
)

// compactUUIDShapePattern matches any canonical or bare-hex UUID rendering.
//
// It exists so the leak assertion catches a UUID this test never seeded — a value derived from
// a row the fixtures do not know about is still a leak.
var compactUUIDShapePattern = regexp.MustCompile(
	`(?i)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\b[0-9a-f]{32}\b)`)

// compactMetricSample is one gathered series reduced to a comparable form.
type compactMetricSample struct {
	// family is the metric family name.
	family string
	// labels are the series' label names and values.
	labels map[string]string
	// value is the gauge/counter value, or the observation count for a histogram.
	value float64
}

// compactMetricFamilyLabels returns the normative label name set of each family.
// Parameters: none.
//
// Return values:
//   - map[string][]string: family name to its exact ordered label names.
func compactMetricFamilyLabels() map[string][]string {
	return map[string][]string{
		compactMetricState:        {"role", "state"},
		compactMetricBacklog:      {"role", "target", "kind"},
		compactMetricActions:      {"role", "action", "result"},
		compactMetricFallback:     {"role", "reason"},
		compactMetricLastProgress: {"role"},
		compactMetricDuration:     {"role", "operation"},
	}
}

// compactAllowedLabelValues returns the compile-time bounded value set of each label name.
//
// Every entry is read from the constants and registries the production code publishes from, not
// copied as a literal list: a new state or registry target must automatically widen this set
// rather than silently fail the bounded-label assertion.
// Parameters: none.
//
// Return values:
//   - map[string][]string: label name to its complete allowed value set.
func compactAllowedLabelValues() map[string][]string {
	states := make([]string, 0, len(compactAllStates()))
	for _, state := range compactAllStates() {
		states = append(states, string(state))
	}
	return map[string][]string{
		"role":   {string(uuidRolePrimary), string(uuidRoleLog)},
		"state":  states,
		"target": compactTargetIDs(),
		"kind":   {compactBacklogGap, compactBacklogMismatch, compactBacklogBlocker},
		"action": {compactActionCycle, compactActionAudit},
		"result": {uuidResultSuccess, uuidResultFailure},
		"reason": {
			compactFallbackMissing, compactFallbackMismatch,
			compactFallbackExpiredHealth, compactFallbackCapability,
		},
		"operation": {compactOperationLock, compactOperationCycle, compactOperationAudit},
	}
}

// withCompactPrometheusRecorder installs the real Prometheus recorder for one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//
// Return values: none.
func withCompactPrometheusRecorder(t *testing.T) {
	t.Helper()
	original := metrics.Recorder()
	t.Cleanup(func() { metrics.SetRecorder(original) })
	metrics.SetRecorder(&oneapiprom.PrometheusRecorder{})
}

// gatherCompactMetrics scrapes the default registry and returns every compact UUID series.
//
// The promauto collectors register on the default registerer at package init, so the default
// gatherer is what a real /metrics scrape reads.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - []compactMetricSample: every currently exported compact UUID series.
func gatherCompactMetrics(t *testing.T) []compactMetricSample {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, errors.Wrap(err, "gather default prometheus registry"))

	samples := []compactMetricSample{}
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), compactMetricPrefix) {
			continue
		}
		for _, metric := range family.GetMetric() {
			sample := compactMetricSample{family: family.GetName(), labels: map[string]string{}}
			for _, label := range metric.GetLabel() {
				sample.labels[label.GetName()] = label.GetValue()
			}
			switch {
			case metric.GetGauge() != nil:
				sample.value = metric.GetGauge().GetValue()
			case metric.GetCounter() != nil:
				sample.value = metric.GetCounter().GetValue()
			case metric.GetHistogram() != nil:
				sample.value = float64(metric.GetHistogram().GetSampleCount())
			}
			samples = append(samples, sample)
		}
	}
	return samples
}

// compactSampleValue finds one exact series among a scrape.
// Parameters:
//   - samples: a gathered scrape.
//   - family: metric family name.
//   - labels: the complete label set identifying the series.
//
// Return values:
//   - float64: the series value, or zero when absent.
//   - bool: true when the series exists.
func compactSampleValue(samples []compactMetricSample, family string, labels map[string]string) (float64, bool) {
	for _, sample := range samples {
		if sample.family != family || len(sample.labels) != len(labels) {
			continue
		}
		matched := true
		for name, value := range labels {
			if sample.labels[name] != value {
				matched = false
				break
			}
		}
		if matched {
			return sample.value, true
		}
	}
	return 0, false
}

// requireCompactSeriesGrew asserts one counter or histogram series advanced between two scrapes.
// Parameters:
//   - t: test handle used for assertions.
//   - before: scrape taken before the transition.
//   - after: scrape taken after the transition.
//   - family: metric family name.
//   - labels: the complete label set identifying the series.
//
// Return values: none.
func requireCompactSeriesGrew(t *testing.T, before []compactMetricSample, after []compactMetricSample,
	family string, labels map[string]string) {
	t.Helper()
	previous, _ := compactSampleValue(before, family, labels)
	current, found := compactSampleValue(after, family, labels)
	require.True(t, found, "%s%v must exist after the transition", family, labels)
	require.Greater(t, current, previous,
		"%s%v must change within one scrape of the transition", family, labels)
}

// requireCompactExactlyOneState asserts the state gauge names exactly one current state per role.
//
// This is the load-bearing assertion of the section 11 state contract: publishing a state must
// leave the gauge for that state at 1 and every other known state at 0, so a stale 1 from an
// earlier state cannot make two states look simultaneously current.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: explicitly constructed database topology.
//   - active: the state that must be the only current one.
//
// Return values: none.
func requireCompactExactlyOneState(t *testing.T, topology *databaseTopology, active compactState) {
	t.Helper()
	samples := gatherCompactMetrics(t)
	for _, role := range topology.markerRoles() {
		current := []string{}
		for _, candidate := range compactAllStates() {
			value, found := compactSampleValue(samples, compactMetricState, map[string]string{
				"role": string(role), "state": string(candidate)})
			require.True(t, found,
				"every known state must be published for role %s, but %s is absent", role, candidate)
			expected := 0.0
			if candidate == active {
				expected = 1
			}
			require.Equal(t, expected, value, "role %s state %s gauge", role, candidate)
			if value == 1 {
				current = append(current, string(candidate))
			}
		}
		require.Equal(t, []string{string(active)}, current,
			"role %s must report exactly one current state", role)
	}
}

// compactStateIsCurrent reports whether a state is the current one for every marker role.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: explicitly constructed database topology.
//   - want: state to look for.
//
// Return values:
//   - bool: true when every marker role's gauge for want is 1.
func compactStateIsCurrent(t *testing.T, topology *databaseTopology, want compactState) bool {
	t.Helper()
	samples := gatherCompactMetrics(t)
	for _, role := range topology.markerRoles() {
		value, found := compactSampleValue(samples, compactMetricState, map[string]string{
			"role": string(role), "state": string(want)})
		if !found || value != 1 {
			return false
		}
	}
	return true
}

// driveCompactWorkerUntil runs real worker cycles until the state gauge reports want.
//
// It drives runCompactWorkerCycle, not the bare coordinator cycle, because publishing state,
// duration, and action metrics is the worker's job: a test that called runCompactCycle directly
// would silently observe no metrics at all.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the cycles.
//   - topology: explicitly constructed database topology.
//   - coordinator: coordinator under test.
//   - want: state the worker must publish.
//
// Return values: none.
func driveCompactWorkerUntil(t *testing.T, ctx context.Context, topology *databaseTopology,
	coordinator *compactCoordinator, want compactState) {
	t.Helper()
	const maxCycles = 200
	for cycle := 0; cycle < maxCycles; cycle++ {
		runCompactWorkerCycle(ctx, coordinator)
		if compactStateIsCurrent(t, topology, want) {
			return
		}
	}
	t.Fatalf("compact worker never published state %q within %d cycles", want, maxCycles)
}

// driveCompactWorkerPeak runs real worker cycles until want is published, scraping one series
// after every cycle and returning its highest observed value.
//
// Sampling every cycle is required rather than tidy: backlog_rows is explicitly a LAST bounded
// observation, so the cycle that fills a gap publishes it and the next cycle publishes zero.
// Two scrapes taken around the whole run would see 0 and 0 and prove nothing.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the cycles.
//   - topology: explicitly constructed database topology.
//   - coordinator: coordinator under test.
//   - want: state the worker must publish.
//   - family: metric family to sample.
//   - labels: the complete label set identifying the sampled series.
//
// Return values:
//   - float64: the highest value the sampled series ever reported.
func driveCompactWorkerPeak(t *testing.T, ctx context.Context, topology *databaseTopology,
	coordinator *compactCoordinator, want compactState, family string, labels map[string]string) float64 {
	t.Helper()
	const maxCycles = 200
	peak := 0.0
	for cycle := 0; cycle < maxCycles; cycle++ {
		runCompactWorkerCycle(ctx, coordinator)
		samples := gatherCompactMetrics(t)
		if value, found := compactSampleValue(samples, family, labels); found && value > peak {
			peak = value
		}
		if compactStateIsCurrent(t, topology, want) {
			return peak
		}
	}
	t.Fatalf("compact worker never published state %q within %d cycles", want, maxCycles)
	return peak
}

func TestCompactUUIDObservability(t *testing.T) {
	// AUTO-T24/AUTO-A10: metrics must change within one scrape for every required transition,
	// every label must be bounded, and no sensitive value may leak.

	t.Run("exactly one active state per role", func(t *testing.T) {
		// The headline section 11 invariant, checked for every state in the precedence list and
		// for both a unified (one role) and a split (two roles) topology.
		withCompactPrometheusRecorder(t)
		withCompactTestSettings(t)

		_, unified := newUnifiedTestTopology(t)
		for _, state := range compactAllStates() {
			publishCompactStateMetrics(unified, state)
			requireCompactExactlyOneState(t, unified, state)
		}

		_, _, split := newSplitTestTopology(t)
		require.Len(t, split.markerRoles(), 2, "a split topology must carry two marker roles")
		for _, state := range compactAllStates() {
			publishCompactStateMetrics(split, state)
			requireCompactExactlyOneState(t, split, state)
		}
	})

	t.Run("success and complete transitions change metrics within one scrape", func(t *testing.T) {
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		coordinator := newCompactCoordinator(topology)

		role := string(uuidRolePrimary)
		gapLabels := map[string]string{"role": role, "target": "users.uuid", "kind": compactBacklogGap}

		before := gatherCompactMetrics(t)
		peakGap := driveCompactWorkerPeak(t, ctx, topology, coordinator, compactStateReady,
			compactMetricBacklog, gapLabels)
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricActions,
			map[string]string{"role": role, "action": compactActionCycle, "result": uuidResultSuccess})
		requireCompactSeriesGrew(t, before, after, compactMetricDuration,
			map[string]string{"role": role, "operation": compactOperationLock})
		requireCompactSeriesGrew(t, before, after, compactMetricDuration,
			map[string]string{"role": role, "operation": compactOperationCycle})

		// The one seeded row's missing shadow must have been visible as a bounded gap
		// observation in the scrape after the cycle that filled it, and must read zero once
		// there is nothing left to fill. The gauge is a last observation, not a running total.
		require.Equal(t, 1.0, peakGap,
			"the seeded row's missing shadow must appear as a bounded gap observation")
		remainingGap, found := compactSampleValue(after, compactMetricBacklog, gapLabels)
		require.True(t, found, "the gap observation must remain published after completion")
		require.Equal(t, 0.0, remainingGap, "a completed migration must observe no remaining gap")

		// Completion is observable as the ready state and a durable progress stamp.
		requireCompactExactlyOneState(t, topology, compactStateReady)
		progress, found := compactSampleValue(after, compactMetricLastProgress,
			map[string]string{"role": role})
		require.True(t, found, "a completed migration must stamp its last durable progress")
		require.InDelta(t, float64(time.Now().UTC().Unix()), progress, 300,
			"last progress must be a recent UTC timestamp, never a row identifier")
	})

	t.Run("degrade and repair transitions change metrics within one scrape", func(t *testing.T) {
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		coordinator := newCompactCoordinator(topology)
		driveCompactToReady(t, coordinator)

		runCompactHealthAudit(ctx, topology)
		requireCompactExactlyOneState(t, topology, compactStateReady)

		// Drop a trigger behind the worker's back, exactly as a restore or an operator could.
		// This is a real drift, not a simulated one.
		before := gatherCompactMetrics(t)
		require.NoError(t, db.Exec("DROP TRIGGER "+compactInsertTriggerName("users")).Error)
		runCompactHealthAudit(ctx, topology)
		after := gatherCompactMetrics(t)

		role := string(uuidRolePrimary)
		requireCompactExactlyOneState(t, topology, compactStateDegraded)
		requireCompactSeriesGrew(t, before, after, compactMetricActions,
			map[string]string{"role": role, "action": compactActionAudit, "result": uuidResultSuccess})
		requireCompactSeriesGrew(t, before, after, compactMetricDuration,
			map[string]string{"role": role, "operation": compactOperationAudit})

		// Repair is automatic and equally observable: the worker restores the object and the
		// gauge returns to ready with no command.
		before = gatherCompactMetrics(t)
		driveCompactToReady(t, coordinator)
		driveCompactWorkerUntil(t, ctx, topology, coordinator, compactStateReady)
		after = gatherCompactMetrics(t)
		requireCompactExactlyOneState(t, topology, compactStateReady)
		requireCompactSeriesGrew(t, before, after, compactMetricActions,
			map[string]string{"role": role, "action": compactActionCycle, "result": uuidResultSuccess})
	})

	t.Run("block transition changes metrics within one scrape", func(t *testing.T) {
		// Invalid authoritative data must be observable, not silent (AUTO-A06).
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		seedCompactUser(t, db, 2, "not-a-canonical-uuid")
		coordinator := newCompactCoordinator(topology)

		before := gatherCompactMetrics(t)
		driveCompactWorkerUntil(t, ctx, topology, coordinator, compactStateBlockedValidation)
		after := gatherCompactMetrics(t)

		requireCompactExactlyOneState(t, topology, compactStateBlockedValidation)
		requireCompactSeriesGrew(t, before, after, compactMetricBacklog, map[string]string{
			"role": string(uuidRolePrimary), "target": "users.uuid", "kind": compactBacklogBlocker})
	})

	t.Run("retry transition changes metrics within one scrape", func(t *testing.T) {
		// A real transient database failure, produced by closing the handle under the worker.
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		coordinator := newCompactCoordinator(topology)

		sqlDB, err := db.DB()
		require.NoError(t, errors.Wrap(err, "unwrap the sqlite handle"))
		require.NoError(t, errors.Wrap(sqlDB.Close(), "close the sqlite handle"))

		before := gatherCompactMetrics(t)
		delay := runCompactWorkerCycle(ctx, coordinator)
		after := gatherCompactMetrics(t)

		require.Positive(t, delay, "a failed cycle must back off rather than spin")
		requireCompactExactlyOneState(t, topology, compactStateRetryWait)
		requireCompactSeriesGrew(t, before, after, compactMetricDuration, map[string]string{
			"role": string(uuidRolePrimary), "operation": compactOperationLock})
	})

	t.Run("lookup fallback is observable", func(t *testing.T) {
		// The runtime half of the observability contract: a process without a fresh healthy
		// audit falls back to legacy predicates, and the fallback is counted by reason.
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		driveCompactToReady(t, newCompactCoordinator(topology))
		resetCompactHealthForTest()

		target, err := compactLookupTarget("users")
		require.NoError(t, err)

		before := gatherCompactMetrics(t)
		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id, "the legacy text index must answer while health is absent")
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
			"role": string(uuidRolePrimary), "reason": compactFallbackExpiredHealth})
	})

	t.Run("labels are bounded and carry no sensitive value", func(t *testing.T) {
		// AUTO-A10. One fixture drives DDL, backfill, validation, markers, audit, drift, repair,
		// and a lookup fallback, then the whole scrape is inspected as a scrape.
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)

		seeded := []string{}
		for index := 1; index <= 4; index++ {
			text := compactUUIDTextFor(index)
			seeded = append(seeded, text)
			seedCompactUser(t, db, index, text)
		}
		coordinator := newCompactCoordinator(topology)
		driveCompactToReady(t, coordinator)
		driveCompactWorkerUntil(t, ctx, topology, coordinator, compactStateReady)
		runCompactHealthAudit(ctx, topology)

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		resetCompactHealthForTest()
		_, err = resolveIDByUUID(ctx, db, target, seeded[0])
		require.NoError(t, err)

		samples := gatherCompactMetrics(t)
		require.NotEmpty(t, samples, "the fixture must have exported compact series")

		allowedValues := compactAllowedLabelValues()
		familyLabels := compactMetricFamilyLabels()
		seenFamilies := map[string]bool{}

		for _, sample := range samples {
			expectedNames, known := familyLabels[sample.family]
			require.True(t, known, "unexpected compact metric family %q", sample.family)
			seenFamilies[sample.family] = true
			require.Len(t, sample.labels, len(expectedNames),
				"%s must carry exactly its fixed labels %v", sample.family, expectedNames)

			for _, name := range expectedNames {
				value, present := sample.labels[name]
				require.True(t, present, "%s must carry label %q", sample.family, name)
				require.Contains(t, allowedValues[name], value,
					"label %s=%q on %s is not a compile-time constant", name, value, sample.family)
				requireCompactLabelIsInert(t, sample.family, name, value, seeded)
			}
		}

		for family := range familyLabels {
			require.True(t, seenFamilies[family],
				"the fixture must exercise every normative family, but %s was never exported", family)
		}
	})
}

// requireCompactLabelIsInert asserts one emitted label value carries nothing sensitive.
//
// The checks are deliberately independent of the bounded-set check above: a constant could be
// added to the allowed set that itself embeds a value, so the shape of the emitted string is
// verified on its own terms.
// Parameters:
//   - t: test handle used for assertions.
//   - family: metric family the label belongs to.
//   - name: label name.
//   - value: emitted label value.
//   - seeded: UUID texts the fixture wrote into rows.
//
// Return values: none.
func requireCompactLabelIsInert(t *testing.T, family string, name string, value string, seeded []string) {
	t.Helper()
	require.NotRegexp(t, compactUUIDShapePattern, value,
		"label %s=%q on %s looks like a UUID", name, value, family)
	for _, uuid := range seeded {
		require.NotContains(t, strings.ToLower(value), strings.ToLower(uuid),
			"label %s on %s leaked a seeded uuid", name, family)
		require.NotContains(t, strings.ToLower(value),
			strings.ToLower(strings.ReplaceAll(uuid, "-", "")),
			"label %s on %s leaked a seeded uuid's bytes", name, family)
	}
	for _, marker := range []string{"://", "@", "password", "secret", "sslmode", "dbname", "file:", ":memory:"} {
		require.NotContains(t, strings.ToLower(value), marker,
			"label %s=%q on %s looks like a DSN, credential, or row content", name, value, family)
	}
}
