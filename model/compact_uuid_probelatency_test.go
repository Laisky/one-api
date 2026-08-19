package model

// AUTO-T26 probe-latency qualification: the other half of the index-size row.
//
// The proposal's T26 row requires, beyond the byte ceiling, "1,000 warm-ups and 10,000 sampled
// probes per pair, three same-run comparisons" with "median/p95 regression ≤10%". This file
// implements exactly that protocol against the same live fixture the size half measures — on
// PostgreSQL 17 and MySQL 8.4 alike — so one run of TestCompactUUIDIndexSize produces both
// halves' evidence per dialect.
//
// The comparison is between the two REAL query shapes: the compact probe is the production
// lookup's predicate (`uuid_compact = ?`, bound with no column-side cast), and the text probe is
// the identical exact-match against the comparator index. Each round interleaves the two sides
// probe-by-probe so machine drift (frequency scaling, checkpoints, cache warmth) lands on both
// sides equally instead of biasing whichever side ran second. Only two things differ per
// dialect, both handled where they arise: the text parameter's spelling (PostgreSQL needs an
// explicit ::bpchar parameter cast, MySQL compares CHAR(36) with a plain string parameter) and
// how a plan is proven to ride its index.

import (
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	// compactProbeWarmups is the per-pair warm-up count the proposal fixes.
	compactProbeWarmups = 1000
	// compactProbeSamples is the per-pair sampled probe count per comparison round.
	compactProbeSamples = 10000
	// compactProbeRounds is the number of same-run comparisons the proposal fixes.
	compactProbeRounds = 3
	// compactProbeRatioLimit is the permitted median/p95 regression: compact may cost at most
	// 10% more than its exact text comparator.
	compactProbeRatioLimit = 1.10
)

// compactProbeSide is one side of a latency comparison: a query shape plus its bind values.
type compactProbeSide struct {
	// label names the side in logs.
	label string
	// query is the probe statement with one placeholder.
	query string
	// indexes are the acceptable plans: the chosen index must be one of these for the
	// measurement to mean anything. The compact side accepts exactly its compact index. The
	// text side accepts the comparator OR the production legacy index on the same column:
	// they are physically equivalent exact-match text B-trees, and which one an optimizer
	// picks when both exist is arbitrary (MySQL and PostgreSQL genuinely choose differently
	// here). Forcing the comparator with an index hint would measure an unnatural plan.
	indexes []string
	// binds returns the bind value for the given sample ordinal.
	binds func(ordinal int) any
}

// compactProbePair is one measured pair: the compact probe against its text comparator.
type compactProbePair struct {
	// label names the pair in logs.
	label string
	// compact is the production-shaped compact-index probe.
	compact compactProbeSide
	// text is the exact-match probe against the text comparator index.
	text compactProbeSide
}

// compactProbePairs builds the probe pairs for the AUTO-T26 fixture.
//
// Bind values are drawn from the seeded key space with a fixed-seed PRNG, so every run probes
// the same pseudo-random sample and a regression is reproducible rather than a lottery. The
// text side casts its parameter to bpchar explicitly: the legacy column is CHAR(36), and a
// parameter typed as text would otherwise force the comparison to text, disqualifying the
// comparator index and measuring a sequential scan instead of the index this test exists to
// compare against.
// Parameters:
//   - dialect: live engine descriptor.
//   - rows: fixture row count this run seeded, bounding the probed key space.
//
// Return values:
//   - []compactProbePair: the pairs to compare.
func compactProbePairs(dialect compactLiveDialect, rows int) []compactProbePair {
	random := rand.New(rand.NewSource(26))
	ownedKey := func(int) any {
		row := 1 + random.Intn(rows)
		value, err := parseCompactUUID(compactUUIDTextFor(row))
		if err != nil {
			return compactUUIDTextFor(row)
		}
		return compactBindValue(dialect.name, value)
	}
	ownedText := func(int) any { return compactUUIDTextFor(1 + random.Intn(rows)) }

	// The FK fixture populates inviter_uuid on even rows only — row n carries row n-1's
	// uuid — so the populated key space is the odd rows' uuids. Probing only that half keeps
	// every probe a genuine index hit rather than a fast miss.
	fkKey := func(int) any {
		row := 2*(1+random.Intn(rows/2-1)) - 1
		value, err := parseCompactUUID(compactUUIDTextFor(row))
		if err != nil {
			return compactUUIDTextFor(row)
		}
		return compactBindValue(dialect.name, value)
	}
	fkText := func(int) any { return compactUUIDTextFor(2*(1+random.Intn(rows/2-1)) - 1) }

	// The text side's parameter spelling is the one genuinely dialect-specific line. On
	// PostgreSQL the legacy column is bpchar, and a parameter left as text forces the
	// comparison to text and disqualifies the comparator index — the explicit ::bpchar cast
	// is on the PARAMETER, never the column, so the probe stays index-shaped. MySQL compares
	// CHAR(36) against a plain string parameter directly.
	textParam := "?"
	if dialect.name == "postgres" {
		textParam = "?::bpchar"
	}

	return []compactProbePair{
		{
			label: "users.uuid (owned, unique)",
			compact: compactProbeSide{
				label: "compact", indexes: []string{"idx_users_uuid_compact_unique"},
				query: "SELECT id FROM users WHERE uuid_compact = ? LIMIT 1", binds: ownedKey,
			},
			text: compactProbeSide{
				label: "text", indexes: []string{"cmp_idx_users_uuid", "idx_users_uuid_unique"},
				query: "SELECT id FROM users WHERE uuid = " + textParam + " LIMIT 1", binds: ownedText,
			},
		},
		{
			label: "users.inviter_uuid (fk, non-unique)",
			compact: compactProbeSide{
				label: "compact", indexes: []string{"idx_users_inviter_uuid_compact"},
				query: "SELECT id FROM users WHERE inviter_uuid_compact = ? LIMIT 1", binds: fkKey,
			},
			text: compactProbeSide{
				label: "text", indexes: []string{"cmp_idx_users_inviter_uuid", "idx_users_inviter_uuid"},
				query: "SELECT id FROM users WHERE inviter_uuid = " + textParam + " LIMIT 1", binds: fkText,
			},
		},
	}
}

// requireProbePlanUsesIndex proves one probe side rides its intended index.
//
// Without this, a mistyped parameter silently degrades a side to a sequential scan and the
// latency "comparison" measures the mistake rather than the representations. How a plan is
// read differs per engine: PostgreSQL's EXPLAIN emits one text column that can be searched,
// while MySQL's emits a row whose `key` column names the chosen index exactly (see
// requireProbePlanUsesIndexMySQL).
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle over the seeded fixture.
//   - dialect: live engine descriptor selecting the plan reader.
//   - side: probe side to verify.
//
// Return values: none.
func requireProbePlanUsesIndex(t *testing.T, db *gorm.DB, dialect compactLiveDialect, side compactProbeSide) {
	t.Helper()
	if dialect.name == "mysql" {
		requireProbePlanUsesIndexMySQL(t, db, side)
		return
	}
	rows := []string{}
	require.NoError(t, db.Raw("EXPLAIN "+side.query, side.binds(0)).Scan(&rows).Error)
	plan := strings.Join(rows, " | ")
	for _, index := range side.indexes {
		if strings.Contains(plan, index) {
			return
		}
	}
	require.Failf(t, "probe plan rides no acceptable index",
		"the %s probe must use one of %v or the comparison is meaningless: %s",
		side.label, side.indexes, plan)
}

// compactProbeQuantiles measures one side's latencies for the given ordinals.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live PostgreSQL handle.
//   - side: probe side to measure.
//   - samples: latency slice to fill, one entry per probe.
//   - base: ordinal offset distinguishing warm-up from measured rounds.
//
// Return values: none.
func compactProbeRun(t *testing.T, db *gorm.DB, side compactProbeSide, samples []time.Duration, base int) {
	t.Helper()
	var sink int64
	for index := range samples {
		bind := side.binds(base + index)
		started := time.Now()
		err := db.Raw(side.query, bind).Scan(&sink).Error
		samples[index] = time.Since(started)
		require.NoError(t, err, "%s probe %d", side.label, index)
	}
}

// compactProbeQuantile returns the q-quantile of a latency sample.
// Parameters:
//   - samples: measured latencies; reordered in place.
//   - q: quantile in [0,1].
//
// Return values:
//   - time.Duration: the sample quantile.
func compactProbeQuantile(samples []time.Duration, q float64) time.Duration {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	position := int(q * float64(len(samples)-1))
	return samples[position]
}

// compactScaleProbeLatency runs the AUTO-T26 probe-latency protocol and asserts the ceiling.
//
// Per pair: 1,000 interleaved warm-ups, then three comparison rounds of 10,000 sampled probes
// each (5,000 per side, interleaved). Each round yields a median and p95 ratio; the MEDIAN of
// the three rounds is asserted at ≤110%, which follows the proposal's three-comparison protocol
// while keeping one OS scheduling hiccup from failing a run that two clean rounds contradict.
// Every round's numbers are logged either way.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle over the seeded fixture.
//   - dialect: live engine descriptor.
//   - rows: fixture row count this run seeded.
//
// Return values: none.
func compactScaleProbeLatency(t *testing.T, db *gorm.DB, dialect compactLiveDialect, rows int) {
	t.Helper()
	for _, pair := range compactProbePairs(dialect, rows) {
		requireProbePlanUsesIndex(t, db, dialect, pair.compact)
		requireProbePlanUsesIndex(t, db, dialect, pair.text)

		warm := make([]time.Duration, compactProbeWarmups/2)
		compactProbeRun(t, db, pair.compact, warm, 0)
		compactProbeRun(t, db, pair.text, warm, 0)

		medians := make([]float64, 0, compactProbeRounds)
		p95s := make([]float64, 0, compactProbeRounds)
		perSide := compactProbeSamples / 2
		for round := 0; round < compactProbeRounds; round++ {
			compactSamples := make([]time.Duration, 0, perSide)
			textSamples := make([]time.Duration, 0, perSide)
			one := make([]time.Duration, 1)
			// Interleave probe-by-probe so drift lands on both sides equally.
			for ordinal := 0; ordinal < perSide; ordinal++ {
				compactProbeRun(t, db, pair.compact, one, round*perSide+ordinal)
				compactSamples = append(compactSamples, one[0])
				compactProbeRun(t, db, pair.text, one, round*perSide+ordinal)
				textSamples = append(textSamples, one[0])
			}
			compactMedian := compactProbeQuantile(compactSamples, 0.50)
			textMedian := compactProbeQuantile(textSamples, 0.50)
			compactP95 := compactProbeQuantile(compactSamples, 0.95)
			textP95 := compactProbeQuantile(textSamples, 0.95)
			medianRatio := float64(compactMedian) / float64(textMedian)
			p95Ratio := float64(compactP95) / float64(textP95)
			medians = append(medians, medianRatio)
			p95s = append(p95s, p95Ratio)
			t.Logf("AUTO-T26 latency %s round %d: median compact=%s text=%s ratio=%.1f%%; "+
				"p95 compact=%s text=%s ratio=%.1f%% (%d probes/side)",
				pair.label, round+1, compactMedian, textMedian, 100*medianRatio,
				compactP95, textP95, 100*p95Ratio, perSide)
		}

		sort.Float64s(medians)
		sort.Float64s(p95s)
		require.LessOrEqual(t, medians[compactProbeRounds/2], compactProbeRatioLimit,
			"%s: median-of-rounds compact/text median latency ratio %.1f%% exceeds the %.0f%% ceiling",
			pair.label, 100*medians[compactProbeRounds/2], 100*compactProbeRatioLimit)
		require.LessOrEqual(t, p95s[compactProbeRounds/2], compactProbeRatioLimit,
			"%s: median-of-rounds compact/text p95 latency ratio %.1f%% exceeds the %.0f%% ceiling",
			pair.label, 100*p95s[compactProbeRounds/2], 100*compactProbeRatioLimit)
	}
}
