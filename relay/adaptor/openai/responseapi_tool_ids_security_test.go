package openai

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceSecurityToolIDs is the legacy spelling oracle. It is deliberately
// independent and used only on small bounded fixtures, never untrusted input.
func referenceSecurityToolIDs(input []string) []string {
	var result []string
	taken := func(id string) bool {
		for _, prior := range result {
			if prior == id {
				return true
			}
		}
		return false
	}
	for _, id := range input {
		candidate := id
		if taken(candidate) {
			base := id
			if base == "" {
				base = "tool_call"
			}
			for n := 2; ; n++ {
				candidate = base + "_" + strconv.Itoa(n)
				if !taken(candidate) {
					break
				}
			}
		}
		result = append(result, candidate)
	}
	return result
}

// allocateSecurityToolIDs exercises one production allocator per assistant turn.
func allocateSecurityToolIDs(input []string) []string {
	var allocator responseToolCallIDAllocator
	var output []string
	for _, id := range input {
		output = append(output, allocator.allocate(id))
	}
	return output
}

// TestSecurityResponseToolIDCompatibility covers empty IDs, suffix collisions,
// case-sensitive IDs and Unicode without changing the legacy pairing vocabulary.
func TestSecurityResponseToolIDCompatibility(t *testing.T) {
	fixtures := [][]string{
		nil, {"a"}, {"a", "a", "a"}, {"a", "a_2", "a", "a_3", "a"},
		{"", "", "tool_call", "tool_call", "tool_call_2", ""},
		{"调用", "调用", "调用_2", "调用"}, {"A", "a", "A", "a"},
		{"x", "x_2", "x_2_2", "x_2", "x", "x_3", "x"},
	}
	for i, input := range fixtures {
		if got, want := allocateSecurityToolIDs(input), referenceSecurityToolIDs(input); !reflect.DeepEqual(got, want) {
			require.FailNowf(t, "security regression", "fixture %d: got %q want %q", i, got, want)
		}
	}
}

// TestSecurityResponseToolIDDifferential compares 10,000 deterministic bounded
// conversations with an independent oracle, not with hardcoded happy-path IDs.
func TestSecurityResponseToolIDDifferential(t *testing.T) {
	random := rand.New(rand.NewSource(476))
	vocabulary := []string{"", "x", "x_2", "x_3", "x_2_2", "tool_call", "tool_call_2", "调用", "X"}
	for sample := 0; sample < 10000; sample++ {
		input := make([]string, random.Intn(33))
		for i := range input {
			input[i] = vocabulary[random.Intn(len(vocabulary))]
		}
		got, want := allocateSecurityToolIDs(input), referenceSecurityToolIDs(input)
		if !reflect.DeepEqual(got, want) {
			require.FailNowf(t, "security regression", "sample %d input %q: got %q want %q", sample, input, got, want)
		}
	}
}

// TestSecurityResponseToolIDCursor advances instead of restarting suffix search.
func TestSecurityResponseToolIDCursor(t *testing.T) {
	var allocator responseToolCallIDAllocator
	for i := 1; i <= MaxResponseAPIFallbackInputItems; i++ {
		id := allocator.allocate("duplicate")
		want := "duplicate"
		if i > 1 {
			want += "_" + strconv.Itoa(i)
		}
		if id != want {
			require.FailNowf(t, "security regression", "allocation %d: %q", i, id)
		}
		if i > 1 && allocator.nextSuffix["duplicate"] != i+1 {
			require.FailNowf(t, "security regression", "cursor did not advance at %d", i)
		}
	}
	if len(allocator.used) != MaxResponseAPIFallbackInputItems {
		require.FailNow(t, "used-ID set lost entries")
	}
}

// TestSecurityResponseFallbackStructuralLimit tests the explicit boundary and
// retains legal zero/empty input behavior for the converter's existing checks.
func TestSecurityResponseFallbackStructuralLimit(t *testing.T) {
	for _, n := range []int{0, 1, MaxResponseAPIFallbackInputItems} {
		if err := ValidateResponseAPIFallbackInput(make([]any, n)); err != nil {
			require.FailNowf(t, "security regression", "valid item count %d rejected", n)
		}
	}
	if err := ValidateResponseAPIFallbackInput(make([]any, MaxResponseAPIFallbackInputItems+1)); err == nil {
		require.FailNow(t, "oversized fallback input accepted")
	}
}

// FuzzSecurityResponseToolIDCompatibility bounds the oracle's deliberately slow
// legacy algorithm while checking arbitrary ID bytes and suffix collisions.
func FuzzSecurityResponseToolIDCompatibility(f *testing.F) {
	for _, seed := range []string{"x\x00x\x00x_2", "\x00\x00tool_call\x00tool_call_2", "调用\x00调用"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 2048 {
			return
		}
		input := strings.Split(raw, "\x00")
		if len(input) > 32 {
			return
		}
		if got, want := allocateSecurityToolIDs(input), referenceSecurityToolIDs(input); !reflect.DeepEqual(got, want) {
			require.FailNowf(t, "security regression", "mismatch for %q: %q != %q", input, got, want)
		}
	})
}

// BenchmarkSecurityResponseToolIDAllocation records bounded input-size scaling;
// wall-clock time is diagnostic, not a flaky correctness assertion.
func BenchmarkSecurityResponseToolIDAllocation(b *testing.B) {
	for _, count := range []int{256, 512, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			input := make([]string, count)
			for i := range input {
				input[i] = "duplicate"
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				allocateSecurityToolIDs(input)
			}
		})
	}
}
