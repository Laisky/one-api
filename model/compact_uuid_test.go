package model

// Codec, registry, and configuration tests for compact UUID storage (AUTO-001, AUTO-002,
// AUTO-011).
//
// The parity vectors here are the Go half of the codec/trigger contract: every vector in
// compactCodecVectors is also driven through each dialect's real trigger by
// TestCompactUUIDTriggerMatrix, so the two implementations are held to one accept/reject
// boundary rather than drifting apart.

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// compactCodecVector is one shared accept/reject parity vector.
type compactCodecVector struct {
	// name identifies the vector in test output.
	name string
	// input is the candidate legacy text.
	input string
	// accepted reports whether the contract accepts this input.
	accepted bool
	// hex is the expected uppercase derived hex when accepted.
	hex string
}

// compactCodecVectors returns the parity vectors required by proposal section 6.1.
//
// The list covers every case the proposal names: lowercase, uppercase, mixed case,
// whitespace-padded, zero, non-v7, wrong-variant, wrong-hyphen, wrong-length, and
// malformed-hex.
// Parameters: none.
//
// Return values:
//   - []compactCodecVector: shared vectors for the codec and the trigger matrix.
func compactCodecVectors() []compactCodecVector {
	return []compactCodecVector{
		{name: "lowercase golden", input: compactUUIDGoldenText, accepted: true,
			hex: strings.ToUpper(compactUUIDGoldenHex)},
		{name: "uppercase", input: strings.ToUpper(compactUUIDGoldenText), accepted: true,
			hex: strings.ToUpper(compactUUIDGoldenHex)},
		{name: "mixed case", input: "018F0000-0000-7000-8000-00000000000A", accepted: true,
			hex: "018F000000007000800000000000000A"},
		{name: "variant 9", input: "018f0000-0000-7000-9000-000000000001", accepted: true,
			hex: "018F0000000070009000000000000001"},
		{name: "variant b", input: "018f0000-0000-7000-b000-000000000001", accepted: true,
			hex: "018F000000007000B000000000000001"},

		{name: "leading whitespace", input: " " + compactUUIDGoldenText, accepted: false},
		{name: "trailing whitespace", input: compactUUIDGoldenText + " ", accepted: false},
		{name: "zero uuid", input: "00000000-0000-0000-0000-000000000000", accepted: false},
		{name: "non-v7 version 4", input: "018f0000-0000-4000-8000-000000000001", accepted: false},
		{name: "wrong variant c", input: "018f0000-0000-7000-c000-000000000001", accepted: false},
		{name: "wrong hyphen placement", input: "018f000-00000-7000-8000-000000000001", accepted: false},
		{name: "no hyphens", input: "018f0000000070008000000000000001", accepted: false},
		{name: "too short", input: "018f0000-0000-7000-8000-00000000001", accepted: false},
		{name: "too long", input: "018f0000-0000-7000-8000-0000000000012", accepted: false},
		{name: "malformed hex", input: "018f0000-0000-7000-8000-00000000000z", accepted: false},
		{name: "empty", input: "", accepted: false},
	}
}

func TestCompactUUIDCodecGoldenVector(t *testing.T) {
	// The proposal fixes this vector; if it ever changes, every dialect's derived bytes and
	// every stored shadow would silently disagree with the Go codec.
	value, err := parseCompactUUID(compactUUIDGoldenText)
	require.NoError(t, err)

	hex := ""
	for _, b := range value {
		const digits = "0123456789abcdef"
		hex += string([]byte{digits[b>>4], digits[b&0x0f]})
	}
	require.Equal(t, compactUUIDGoldenHex, hex, "golden text must derive the golden bytes")
	require.Equal(t, compactUUIDGoldenText, value.canonical(), "golden bytes must format back to golden text")
}

func TestCompactUUIDCodecParityVectors(t *testing.T) {
	for _, vector := range compactCodecVectors() {
		t.Run(vector.name, func(t *testing.T) {
			value, err := parseCompactUUID(vector.input)
			if !vector.accepted {
				require.Error(t, err)
				require.ErrorIs(t, err, errCompactUUIDInvalid)
				// No error may echo the offending value: it is often request input.
				// The empty vector is skipped because every string contains "".
				if vector.input != "" {
					require.NotContains(t, err.Error(), vector.input)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, vector.hex, strings.ToUpper(hexOf(value)))
			// Accepted input normalizes to lowercase regardless of the input's case.
			require.Equal(t, strings.ToLower(vector.input), value.canonical())
		})
	}
}

// hexOf renders compact bytes as lowercase hex for assertions.
// Parameters:
//   - value: compact value.
//
// Return values:
//   - string: lowercase hex.
func hexOf(value compactUUID) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 32)
	for _, b := range value {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}

func TestCompactUUIDCodecNullDistinctFromZero(t *testing.T) {
	// SQL NULL and the all-zero value must never collide: a zero-filled shadow would
	// otherwise read as "not yet derived".
	null := nullCompactUUID{}
	require.NoError(t, null.Scan(nil))
	require.False(t, null.valid)

	zero := nullCompactUUID{}
	require.NoError(t, zero.Scan(make([]byte, compactUUIDLen)))
	require.True(t, zero.valid, "16 zero bytes are a present value, not NULL")
	require.False(t, null.equal(zero), "NULL and all-zero must not compare equal")

	value, err := null.Value()
	require.NoError(t, err)
	require.Nil(t, value)
}

func TestCompactUUIDCodecScanCopiesDriverBuffer(t *testing.T) {
	// Drivers reuse their row buffer, so a scan that retained the slice would mutate an
	// already-scanned value when the next row landed.
	buffer := make([]byte, compactUUIDLen)
	golden, err := parseCompactUUID(compactUUIDGoldenText)
	require.NoError(t, err)
	copy(buffer, golden[:])

	scanned := nullCompactUUID{}
	require.NoError(t, scanned.Scan(buffer))
	for index := range buffer {
		buffer[index] = 0xff
	}
	require.Equal(t, golden, scanned.value, "scanned value must not alias the driver buffer")
}

func TestCompactUUIDCodecScanAcceptsDialectRepresentations(t *testing.T) {
	golden, err := parseCompactUUID(compactUUIDGoldenText)
	require.NoError(t, err)

	for name, src := range map[string]any{
		"postgres canonical text":  compactUUIDGoldenText,
		"postgres text bytes":      []byte(compactUUIDGoldenText),
		"mysql/sqlite raw bytes":   golden.bytes(),
		"pgtype fixed byte array":  [16]byte(golden),
		"uppercase canonical text": strings.ToUpper(compactUUIDGoldenText),
	} {
		t.Run(name, func(t *testing.T) {
			scanned := nullCompactUUID{}
			require.NoError(t, scanned.Scan(src))
			require.True(t, scanned.valid)
			require.Equal(t, golden, scanned.value)
		})
	}
}

func TestCompactUUIDBindValueHasNoColumnSideCast(t *testing.T) {
	golden, err := parseCompactUUID(compactUUIDGoldenText)
	require.NoError(t, err)

	// PostgreSQL binds canonical text, which the server accepts for a native uuid parameter.
	require.Equal(t, compactUUIDGoldenText, compactBindValue("postgres", golden))

	// MySQL and SQLite bind a freshly copied 16-byte slice.
	for _, dialect := range []string{"mysql", "sqlite"} {
		bound, ok := compactBindValue(dialect, golden).([]byte)
		require.True(t, ok, "%s must bind raw bytes", dialect)
		require.Len(t, bound, compactUUIDLen)
		require.Equal(t, golden.bytes(), bound)

		bound[0] = 0xff
		require.Equal(t, byte(0x01), golden[0], "bind value must be a copy, not an alias")
	}
}

func TestCompactUUIDDerivationTable(t *testing.T) {
	owned, err := compactTargetByID("users.uuid")
	require.NoError(t, err)
	fk, err := compactTargetByID("users.inviter_uuid")
	require.NoError(t, err)

	t.Run("valid owned uuid derives bytes", func(t *testing.T) {
		derived, blocked := deriveCompactFromLegacy(owned, nullString{value: compactUUIDGoldenText, valid: true})
		require.False(t, blocked)
		require.True(t, derived.valid)
	})
	t.Run("null owned uuid blocks completion", func(t *testing.T) {
		derived, blocked := deriveCompactFromLegacy(owned, nullString{})
		require.True(t, blocked, "a missing owned uuid cannot be repaired without inventing data")
		require.False(t, derived.valid)
	})
	t.Run("empty owned uuid blocks completion", func(t *testing.T) {
		_, blocked := deriveCompactFromLegacy(owned, nullString{value: "", valid: true})
		require.True(t, blocked)
	})
	t.Run("malformed owned uuid blocks completion", func(t *testing.T) {
		_, blocked := deriveCompactFromLegacy(owned, nullString{value: "nope", valid: true})
		require.True(t, blocked)
	})
	t.Run("null fk is a valid terminal state", func(t *testing.T) {
		derived, blocked := deriveCompactFromLegacy(fk, nullString{})
		require.False(t, blocked, "a nullable FK NULL is valid, not a blocker")
		require.False(t, derived.valid)
	})
	t.Run("empty fk is a valid terminal state", func(t *testing.T) {
		derived, blocked := deriveCompactFromLegacy(fk, nullString{value: "", valid: true})
		require.False(t, blocked)
		require.False(t, derived.valid)
	})
	t.Run("malformed populated fk blocks completion", func(t *testing.T) {
		_, blocked := deriveCompactFromLegacy(fk, nullString{value: "nope", valid: true})
		require.True(t, blocked)
	})

	t.Run("a space-padded empty fk is empty, not malformed", func(t *testing.T) {
		// Regression test for a real cross-dialect defect that only a live PostgreSQL run
		// exposed. The legacy columns are CHAR(36); PostgreSQL's CHAR is bpchar, so an empty
		// value comes back through the driver as 36 spaces rather than "". Classifying that
		// as a malformed populated FK blocked completion permanently — on data the derivation
		// table defines as a valid terminal state — and reported valid rows as corrupt.
		padded := strings.Repeat(" ", 36)
		derived, blocked := deriveCompactFromLegacy(fk, nullString{value: padded, valid: true})
		require.False(t, blocked, "a space-padded empty FK is absent, not malformed")
		require.False(t, derived.valid, "it must derive NULL")
	})

	t.Run("a space-padded empty owned uuid still blocks", func(t *testing.T) {
		// The padding fix must not weaken the owned contract: a missing owned UUID is a
		// blocker however the engine renders its absence.
		_, blocked := deriveCompactFromLegacy(owned, nullString{value: strings.Repeat(" ", 36), valid: true})
		require.True(t, blocked)
	})

	t.Run("a whitespace-padded uuid is still rejected", func(t *testing.T) {
		// The blank test must not become a general trim. The codec's accept boundary has to
		// stay byte-exact so it agrees with each dialect's trigger, and every trigger rejects
		// a padded UUID. This value is not blank, so it reaches the codec and is refused.
		_, blocked := deriveCompactFromLegacy(owned, nullString{
			value: " " + compactUUIDGoldenText[1:], valid: true,
		})
		require.True(t, blocked, "a padded uuid must not be silently trimmed into validity")
	})
}

func TestCompactUUIDRegistryMatchesPhysicalInventory(t *testing.T) {
	registry := compactRegistry()

	// The proposal's exact physical inventory: 12 owned + 15 FK = 27 targets.
	require.Len(t, registry, 27, "compact registry must cover all 27 targets")
	require.Len(t, registry, len(uuidOwnedRegistry())+len(uuidFKRegistry()),
		"compact registry must be derived from the owned and FK registries, not duplicated")

	owned := 0
	unique := 0
	for _, target := range registry {
		if target.kind == compactKindOwned {
			owned++
			require.True(t, target.unique(), "owned target %s must have a unique index", target.id())
			require.False(t, target.nullable(), "owned target %s must not treat NULL as terminal", target.id())
			require.Equal(t, "idx_"+target.table+"_uuid_compact_unique", target.indexName())
		} else {
			require.False(t, target.unique(), "fk target %s must have a non-unique index", target.id())
			require.True(t, target.nullable(), "fk target %s must treat NULL as terminal", target.id())
			require.Equal(t, "idx_"+target.table+"_"+target.legacyColumn+"_compact", target.indexName())
		}
		if target.unique() {
			unique++
		}
		require.Equal(t, target.legacyColumn+"_compact", target.compactColumn,
			"shadow naming is fixed by the proposal and must never change")
	}
	require.Equal(t, 12, owned, "12 owned UUID columns")
	require.Equal(t, 12, unique, "12 unique compact indexes")
	require.Equal(t, 15, len(registry)-owned, "15 denormalized FK UUID columns")
}

func TestCompactUUIDRegistrySplitRoleOwnership(t *testing.T) {
	// In split mode the primary owns 23 non-log targets and LOG_DB exclusively owns the four
	// log targets, which is what keeps a stale primary logs table unreachable.
	require.Len(t, compactTargetsForRole(uuidRolePrimary), 23)
	require.Len(t, compactTargetsForRole(uuidRoleLog), 4)

	for _, target := range compactTargetsForRole(uuidRoleLog) {
		require.Equal(t, "logs", target.table, "only logs may resolve through the log role")
	}
	for _, target := range compactTargetsForRole(uuidRolePrimary) {
		require.NotEqual(t, "logs", target.table, "a stale primary logs table must never be a target")
	}
}

func TestCompactUUIDRegistryIndexNamesAreUnique(t *testing.T) {
	// Two targets sharing an index name would silently make one of them unindexed.
	seen := map[string]string{}
	for _, target := range compactRegistry() {
		previous, clash := seen[target.indexName()]
		require.False(t, clash, "index name %s is shared by %s and %s",
			target.indexName(), previous, target.id())
		seen[target.indexName()] = target.id()
	}
	require.Len(t, seen, 27)
	require.Len(t, compactTargetIDs(), 27)
}

func TestCompactUUIDRegistryIsDeterministic(t *testing.T) {
	// Golden DDL, validation order, and the operational probe suite all assume one stable
	// order; a map-iteration-order dependency would make them flake.
	first := compactRegistry()
	for attempt := 0; attempt < 10; attempt++ {
		require.Equal(t, first, compactRegistry(), "registry order must be deterministic")
	}
}

func TestCompactUUIDConfiguration(t *testing.T) {
	t.Run("defaults satisfy zero-touch acceptance", func(t *testing.T) {
		t.Setenv(config.EnvCompactUUIDAutoMigrate, "")
		require.NoError(t, config.LoadCompactUUIDSettings())
		require.True(t, config.CompactUUIDAutoMigrate, "unset configuration must migrate automatically")
		require.Equal(t, 1000, config.CompactUUIDBatchSize)
		require.Equal(t, 10000, config.CompactUUIDMaxRowsPerCycle)
		require.Equal(t, 30*time.Second, config.CompactUUIDMaxCycleDuration)
		require.Equal(t, 5*time.Second, config.CompactUUIDActiveInterval)
		require.Equal(t, 5*time.Minute, config.CompactUUIDIdleInterval)
		require.Equal(t, 30*time.Second, config.CompactUUIDRetryInterval)
		require.Equal(t, 5*time.Second, config.CompactUUIDLockTimeout)
		require.Equal(t, 30*time.Minute, config.CompactUUIDDDLTimeout)
		require.Equal(t, 2*time.Hour, config.CompactUUIDValidationTimeout)
	})

	t.Run("invalid values fail before the worker is created", func(t *testing.T) {
		for name, value := range map[string]string{
			config.EnvCompactUUIDAutoMigrate:       "yes-please",
			config.EnvCompactUUIDBatchSize:         "0",
			config.EnvCompactUUIDMaxRowsPerCycle:   "999",
			config.EnvCompactUUIDMaxCycleDuration:  "31m",
			config.EnvCompactUUIDActiveInterval:    "500ms",
			config.EnvCompactUUIDIdleInterval:      "2h",
			config.EnvCompactUUIDRetryInterval:     "not-a-duration",
			config.EnvCompactUUIDLockTimeout:       "6s",
			config.EnvCompactUUIDDDLTimeout:        "30s",
			config.EnvCompactUUIDValidationTimeout: "25h",
		} {
			t.Run(name+"="+value, func(t *testing.T) {
				t.Setenv(name, value)
				err := config.LoadCompactUUIDSettings()
				require.Error(t, err, "%s=%s must be rejected", name, value)
				require.Contains(t, err.Error(), name, "the error must name the offending variable")
			})
		}
	})

	t.Run("batch size respects the bind ceiling", func(t *testing.T) {
		target, err := compactTargetByID("users.uuid")
		require.NoError(t, err)
		original := config.CompactUUIDBatchSize
		t.Cleanup(func() { config.CompactUUIDBatchSize = original })

		config.CompactUUIDBatchSize = MaxCompactUUIDBatchSizeForTest()
		rows := compactBatchRows(target)
		require.LessOrEqual(t, rows, 200, "the proposal caps a repair batch at 200 rows")
		require.LessOrEqual(t, rows*4+2, compactMaxBinds, "a statement must stay at or below 900 binds")
		require.LessOrEqual(t, rows, compactMaxMaterializedRows)
		require.Positive(t, rows)
	})
}

// MaxCompactUUIDBatchSizeForTest exposes the configured batch ceiling to the batch-size test.
// Parameters: none.
//
// Return values:
//   - int: the largest configurable batch size.
func MaxCompactUUIDBatchSizeForTest() int {
	return config.MaxCompactUUIDBatchSize
}
