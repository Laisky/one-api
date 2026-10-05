package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLiveBlockingFunctionsRequiresExplicitBlocking verifies that only an
// exact, explicit BLOCKING declaration is trusted, because the provider's
// default behavior differs by model. Parameters: t is a test. Returns: none.
func TestLiveBlockingFunctionsRequiresExplicitBlocking(t *testing.T) {
	t.Parallel()
	setup := []byte(`{"setup":{"model":"m","tools":[{"functionDeclarations":[
		{"name":"named","behavior":"BLOCKING"},
		{"name":"numbered","behavior":1},
		{"name":"absent"},
		{"name":"null","behavior":null},
		{"name":"unspecified","behavior":"UNSPECIFIED"},
		{"name":"zero","behavior":0},
		{"name":"async","behavior":"NON_BLOCKING"},
		{"name":"lower","behavior":"blocking"},
		{"name":"folded","Behavior":"BLOCKING"},
		{"name":"twice","behavior":"BLOCKING"}
	]},{"functionDeclarations":[{"name":"twice","behavior":"NON_BLOCKING"}]}]}}`)
	require.Equal(t, map[string]bool{"named": true, "numbered": true}, liveBlockingFunctions(setup))
	require.Empty(t, liveBlockingFunctions([]byte(`{"setup":{"model":"m"}}`)))
	require.Empty(t, liveBlockingFunctions([]byte(`not json`)))
}

// TestLiveToolCoverageNeverClaimsUnconsumedResults walks the coverage states:
// only a blocking result of the open turn that model output then proves
// consumed is covered by that turn's receipt; every other result needs a turn
// that starts after it. Parameters: t is a test. Returns: none.
func TestLiveToolCoverageNeverClaimsUnconsumedResults(t *testing.T) {
	t.Parallel()
	blocking := map[string]bool{"wait": true}

	t.Run("blocking_consumed_same_turn", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}})
		require.True(t, c.pending())
		c.modelOutput()
		c.receipted()
		c.turnEnded()
		require.False(t, c.pending())
	})

	t.Run("blocking_without_model_output", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}})
		c.receipted() // the turn closed before the provider resumed
		c.turnEnded()
		require.True(t, c.pending(), "only output after the result proves it was consumed")
		c.turnStarted()
		c.receipted()
		require.False(t, c.pending())
	})

	t.Run("output_before_admit_does_not_confirm", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.modelOutput()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}})
		c.receipted()
		require.True(t, c.pending())
	})

	t.Run("consumed_then_interrupted", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}})
		c.modelOutput()
		c.interrupt()
		c.receipted()
		require.False(t, c.pending(), "a consumed result stays in the interrupted turn's receipt")
	})

	t.Run("mixed_frame_is_deferred", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}, {name: "async", turn: c.currentTurn()}})
		c.modelOutput()
		c.receipted()
		c.turnEnded()
		require.True(t, c.pending(), "one non-blocking answer defers the whole frame")
		c.turnStarted()
		c.receipted()
		require.False(t, c.pending())
	})

	t.Run("blocking_after_turn_end", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		call := liveCall{name: "wait", turn: c.currentTurn()}
		c.turnEnded() // turnComplete before the receipt and before the result
		c.admit([]liveCall{call})
		c.modelOutput()
		c.receipted()
		c.turnEnded()
		require.True(t, c.pending(), "a result after its turn ended needs a later turn")
		c.turnStarted()
		c.receipted()
		require.False(t, c.pending())
	})

	t.Run("blocking_interrupted", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "wait", turn: c.currentTurn()}})
		c.interrupt()
		c.modelOutput()
		c.receipted()
		c.turnEnded()
		require.True(t, c.pending(), "output after an interruption proves nothing")
		c.turnStarted()
		c.receipted()
		require.False(t, c.pending())
	})

	t.Run("blocking_admitted_after_interrupt", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		turn := c.currentTurn()
		c.interrupt()
		c.admit([]liveCall{{name: "wait", turn: turn}})
		c.modelOutput()
		c.receipted()
		require.True(t, c.pending())
	})

	t.Run("deferred_needs_a_later_turn", func(t *testing.T) {
		c := newLiveToolCoverage(blocking)
		c.turnStarted()
		c.admit([]liveCall{{name: "async", turn: c.currentTurn()}})
		c.modelOutput()
		c.receipted()
		c.turnEnded()
		require.True(t, c.pending(), "the open turn's receipt never covers a deferred result")
		c.turnStarted()
		c.admit([]liveCall{{name: "async", turn: c.currentTurn()}})
		c.receipted()
		c.turnEnded()
		require.True(t, c.pending(), "a result admitted during the covering turn still waits")
		c.turnStarted()
		c.receipted()
		require.False(t, c.pending())
	})
}

// TestLiveToolStateRejectsAmbiguousResponseIdentity verifies that a response
// identity is read with the provider's exact keys: a second spelling of "id"
// or "name" is refused, while keys inside the response payload stay arbitrary.
// Parameters: t is a test. Returns: none.
func TestLiveToolStateRejectsAmbiguousResponseIdentity(t *testing.T) {
	t.Parallel()
	state := &liveToolState{}
	require.NoError(t, state.observe([]byte(`{"toolCall":{"functionCalls":[{"id":"a","name":"lookup"},{"id":"b","name":"lookup"}]}}`), 0))
	_, err := state.accept([]byte(`{"toolResponse":{"functionResponses":[{"id":"a","ID":"b","response":{}}]}}`))
	require.ErrorIs(t, err, ErrLiveProtocol)
	_, err = state.accept([]byte(`{"toolResponse":{"functionResponses":[{"id":"a","Name":"other","response":{}}]}}`))
	require.ErrorIs(t, err, ErrLiveProtocol)
	calls, err := state.accept([]byte(`{"toolResponse":{"functionResponses":[{"id":"a","response":{"ID":"x","Name":"y"}}]}}`))
	require.NoError(t, err, "keys inside the response payload are arbitrary")
	require.Equal(t, []liveCall{{name: "lookup", turn: 0}}, calls)
}
