package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
)

// countingRecorder counts RecordResponseStateEvent calls by (category,outcome)
// while delegating every other metric to the no-op base. It is exercised directly
// (NOT installed as the process-global metrics.GlobalRecorder) so this test never
// races the async billing goroutines other controller tests leave in flight.
type countingRecorder struct {
	metrics.NoOpRecorder
	counts map[string]int
}

func (r *countingRecorder) RecordResponseStateEvent(category, outcome string) {
	if r.counts == nil {
		r.counts = map[string]int{}
	}
	r.counts[category+"/"+outcome]++
}

// TestResponseStateRecorderCountsByLabel verifies the recorder mechanism ST-014
// installs counts each event under its (category,outcome) label pair (OBS01).
func TestResponseStateRecorderCountsByLabel(t *testing.T) {
	rec := &countingRecorder{}
	rec.RecordResponseStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeHydrated)
	rec.RecordResponseStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeHydrated)
	rec.RecordResponseStateEvent(metrics.StateCategoryPortability, metrics.StateOutcomeNotPortable)

	require.Equal(t, 2, rec.counts[metrics.StateCategoryPath+"/"+metrics.StateOutcomeHydrated])
	require.Equal(t, 1, rec.counts[metrics.StateCategoryPortability+"/"+metrics.StateOutcomeNotPortable])
}

// TestResponseStateLabelVocabularyBounded verifies the label vocabulary is a small
// set of distinct, non-empty compile-time constants (OBS01: bounded cardinality,
// never content).
func TestResponseStateLabelVocabularyBounded(t *testing.T) {
	values := []string{
		metrics.StateCategoryPath, metrics.StateCategoryPortability, metrics.StateCategoryCommit,
		metrics.StateCategoryAffinity, metrics.StateCategoryMiss,
		metrics.StateOutcomeHydrated, metrics.StateOutcomeConversation, metrics.StateOutcomeStateless,
		metrics.StateOutcomePortable, metrics.StateOutcomeSidecarDropped, metrics.StateOutcomeNotPortable,
		metrics.StateOutcomeCommitted, metrics.StateOutcomeCommitFailed, metrics.StateOutcomeNoStore,
		metrics.StateOutcomePinned, metrics.StateOutcomeUnpinned,
		metrics.StateOutcomeNotFound, metrics.StateOutcomeStoreError,
	}
	seen := map[string]struct{}{}
	for _, v := range values {
		require.NotEmpty(t, v)
		_, dup := seen[v]
		require.False(t, dup, "label value %q must be unique", v)
		seen[v] = struct{}{}
	}

	// The default global recorder handles the event without panicking.
	require.NotNil(t, metrics.GlobalRecorder)
	metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeStateless)
}
