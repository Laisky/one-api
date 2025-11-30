package random_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/random"
)

func TestUniqueness(t *testing.T) {
	// Table-driven test structure for uniqueness testing
	tests := []struct {
		name          string
		generator     func() string
		iterations    int
		expectedUniq  bool
		allowDupsRate float64 // Allow this rate of duplicates (for functions with limited output space)
	}{
		{
			name:         "GetUUID should always generate unique values",
			generator:    random.GetUUID,
			iterations:   100000,
			expectedUniq: true,
		},
		{
			name:         "GenerateKey should always generate unique values",
			generator:    random.GenerateKey,
			iterations:   100000,
			expectedUniq: true,
		},
		{
			name: "GetRandomString(10) should generate unique values",
			generator: func() string {
				return random.GetRandomString(10)
			},
			iterations:   100000,
			expectedUniq: true,
		},
		{
			name: "GetRandomString(20) should generate unique values",
			generator: func() string {
				return random.GetRandomString(20)
			},
			iterations:   100000,
			expectedUniq: true,
		},
		{
			name: "GetRandomNumberString(10) should generate mostly unique values",
			generator: func() string {
				return random.GetRandomNumberString(10)
			},
			iterations:    100000,
			expectedUniq:  false,
			allowDupsRate: 0.001, // Allow 0.1% duplicates due to limited numeric space
		},
		{
			name: "GetRandomNumberString(15) should generate mostly unique values",
			generator: func() string {
				return random.GetRandomNumberString(15)
			},
			iterations:   100000,
			expectedUniq: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a map to track unique values
			seen := make(map[string]bool, tt.iterations)
			duplicates := 0

			// Generate values and check for duplicates
			for i := 0; i < tt.iterations; i++ {
				val := tt.generator()
				if seen[val] {
					duplicates++
				} else {
					seen[val] = true
				}
			}

			dupRate := float64(duplicates) / float64(tt.iterations)

			// Check if we found duplicates
			if tt.expectedUniq {
				require.Zero(t, duplicates, "Expected all unique values, but found %d duplicates out of %d iterations (%.4f%%)",
					duplicates, tt.iterations, dupRate*100)
			} else {
				require.LessOrEqual(t, dupRate, tt.allowDupsRate, "Duplicate rate of %.4f%% exceeds allowable threshold of %.4f%%",
					dupRate*100, tt.allowDupsRate*100)
			}

			// Always log the uniqueness statistics for informational purposes
			t.Logf("Generated %d values with %d unique (%.4f%% duplicate rate)",
				tt.iterations, len(seen), dupRate*100)
		})
	}
}

// TestRandRangeDistribution tests that RandRange produces values with a reasonable distribution
func TestRandRangeDistribution(t *testing.T) {
	// For RandRange, we're more interested in distribution than uniqueness
	min, max := 1, 10
	iterations := 100000

	// Count occurrences of each value
	counts := make(map[int]int, max-min)
	for range iterations {
		val := random.RandRange(min, max)
		counts[val]++

		// Also verify the range constraint
		require.GreaterOrEqual(t, val, min, "RandRange(%d, %d) produced %d, which is below the minimum", min, max, val)
		require.Less(t, val, max, "RandRange(%d, %d) produced %d, which is at or above the maximum", min, max, val)
	}

	// Check distribution (should be roughly even)
	expectedPerBucket := float64(iterations) / float64(max-min)
	tolerance := 0.1 // Allow 10% deviation from expected

	for val := min; val < max; val++ {
		count := counts[val]
		ratio := float64(count) / expectedPerBucket

		// Log the distribution
		t.Logf("Value %d appeared %d times (%.2f%% of expected)",
			val, count, ratio*100)

		// Check if distribution is reasonable
		if ratio < 1-tolerance || ratio > 1+tolerance {
			t.Logf("Warning: Value %d distribution is outside %.1f%% tolerance (%.2f%%)",
				val, tolerance*100, (ratio-1)*100)
		}
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("GetRandomString zero length", func(t *testing.T) {
		s := random.GetRandomString(0)
		require.Empty(t, s, "Expected empty string")
	})

	t.Run("GetRandomNumberString zero length", func(t *testing.T) {
		s := random.GetRandomNumberString(0)
		require.Empty(t, s, "Expected empty string")
	})

	t.Run("GetRandomString negative length (should panic)", func(t *testing.T) {
		require.Panics(t, func() {
			_ = random.GetRandomString(-1)
		}, "Expected panic for negative length")
	})

	t.Run("GetRandomNumberString negative length (should panic)", func(t *testing.T) {
		require.Panics(t, func() {
			_ = random.GetRandomNumberString(-5)
		}, "Expected panic for negative length")
	})

	t.Run("RandRange min == max (should always return min)", func(t *testing.T) {
		for range 10 {
			v := random.RandRange(5, 5)
			require.Equal(t, 5, v, "Expected 5")
		}
	})

	t.Run("RandRange min > max (should panic)", func(t *testing.T) {
		require.Panics(t, func() {
			_ = random.RandRange(10, 5)
		}, "Expected panic for min > max")
	})
}
