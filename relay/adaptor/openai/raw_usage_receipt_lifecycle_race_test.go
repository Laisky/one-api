package openai

import (
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// rawReceiptRaceIterations is the number of independent schedules each
// concurrency test explores under the race detector.
const rawReceiptRaceIterations = 300

// requireRawReceiptConsistentSnapshot asserts one finish result is a prefix
// state of the fixture stream rather than a torn mixture: a measured receipt
// is exactly the terminal one, and otherwise the input is either unseen or the
// provisional snapshot. Parameters: t asserts and usage is the snapshot.
func requireRawReceiptConsistentSnapshot(t *testing.T, usage *model.Usage) {
	t.Helper()
	if usage == nil {
		return
	}
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	if usage.BillingEstimateReason == "" {
		require.Equal(t, 20, usage.PromptTokens, "a measured snapshot must be the terminal receipt")
		require.Equal(t, 2, usage.CompletionTokens, "a measured snapshot must be the terminal receipt")
		return
	}
	require.Contains(t, []int{0, 20}, usage.PromptTokens, "an estimate's input must be unseen or the provisional snapshot")
	require.GreaterOrEqual(t, usage.CompletionTokens, 0)
}

// TestRawUsageReceiptConcurrentFinishReadClose explores many schedules of a
// provider writer, a client reader that keeps reading and then closes, two
// concurrent finish calls, and (on some schedules) a concurrent Close that
// cuts the read short. Under -race every schedule must be race-free, both
// finish calls must return the same snapshot as a finish after all goroutines
// stopped, and that snapshot must be a consistent prefix state of the stream.
func TestRawUsageReceiptConcurrentFinishReadClose(t *testing.T) {
	frames := rawReceiptProvisional() + rawReceiptDelta("alpha ") + rawReceiptDelta("beta") +
		rawReceiptCompleted("alpha beta", rawReceiptCompleteUsage) + "data: [DONE]\n\n"
	for iteration := range rawReceiptRaceIterations {
		mode := rawReceiptModes[iteration%len(rawReceiptModes)].mode
		c, _, info := rawReceiptContext(mode, true)
		reader, writer := io.Pipe()
		resp := rawReceiptStreamResponse(reader)
		receipt := observeRawUsageReceipt(resp, info)
		require.NotNil(t, receipt)
		// Cut the provider at a different byte on each schedule; a full write
		// ends with a clean EOF, a short one with an unexpected EOF.
		cut := len(frames) * (iteration % 11) / 10

		var wg sync.WaitGroup
		wg.Go(func() {
			for data := []byte(frames[:min(cut, len(frames))]); len(data) > 0; {
				n, err := writer.Write(data[:min(len(data), 11)])
				if err != nil {
					return
				}
				data = data[n:]
			}
			if cut >= len(frames) {
				_ = writer.Close()
				return
			}
			_ = writer.CloseWithError(io.ErrUnexpectedEOF)
		})
		wg.Go(func() {
			buffer := make([]byte, 7)
			for {
				if _, err := resp.Body.Read(buffer); err != nil {
					break
				}
			}
			_ = resp.Body.Close()
		})
		snapshots := make([]*model.Usage, 2)
		for i := range snapshots {
			wg.Go(func() {
				for range (iteration + i) % 7 {
					runtime.Gosched()
				}
				snapshots[i] = receipt.finish(c, nil, false)
			})
		}
		if iteration%3 == 0 {
			wg.Go(func() {
				runtime.Gosched()
				_ = receipt.Close()
			})
		}
		wg.Wait()

		final := receipt.finish(c, nil, false)
		for _, snapshot := range snapshots {
			require.Equal(t, final, snapshot, "iteration %d: finish must return one frozen snapshot", iteration)
		}
		requireRawReceiptConsistentSnapshot(t, final)
		if mode == relaymode.ClaudeMessages && final != nil {
			requireRawReceiptClaudeStored(t, c, final)
		}
	}
}

// TestClaudeStreamConverterObserverSurvivesAbandonedRead is a -race
// regression for the Claude Messages stream converter. When the client
// disconnects, the heartbeat line reader returns while its goroutine is still
// parked in the provider Read; bytes completing that Read used to be fed to
// the converter's usage accumulator (claudeUsageReader) and web-search counter
// without a lock, concurrently with Finish/record. Both observers now
// serialize with a mutex and ignore bytes after settlement. Rounds alternate
// gpt-6-cyber (raw usage receipt installed) and gpt-4o (no receipt); many
// rounds make a regression's race report effectively certain under -race.
func TestClaudeStreamConverterObserverSurvivesAbandonedRead(t *testing.T) {
	for round := range 64 {
		modelName := []string{"gpt-6-cyber", "gpt-4o"}[round%2]
		c, _, info, cancel := rawReceiptCancellableContext(t, relaymode.ClaudeMessages, "")
		info.ActualModelName, info.OriginModelName = modelName, modelName
		c.Set(ctxkey.ConvertedRequest, &ResponseAPIRequest{Model: modelName})
		clientCtx := c.Request.Context()
		reader, writer := io.Pipe()
		body := &rawReceiptStubbornBody{PipeReader: reader}
		resp := rawReceiptStreamResponse(body)
		done := rawReceiptRunDoResponse(c, resp, info)

		written, err := io.WriteString(writer, rawReceiptProvisional()+rawReceiptDelta("before the disconnect"))
		require.NoError(t, err)
		rawReceiptWaitPendingRead(t, body, written)

		// The in-flight bytes are ordered after the disconnect only, never after
		// the converter's Finish. The unterminated line forces one more Read,
		// which proves the abandoned goroutine's Observe returned in this round.
		lateWritten := make(chan int, 1)
		go func() {
			<-clientCtx.Done()
			late, _ := io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"in flight\"}")
			lateWritten <- written + late
		}()
		cancel()
		result := rawReceiptAwait(t, done)
		require.Nil(t, result.apiErr, "round %d", round)
		require.NotNil(t, result.usage, "round %d", round)
		select {
		case total := <-lateWritten:
			rawReceiptWaitPendingRead(t, body, total)
		case <-time.After(rawReceiptLifecycleTimeout):
			// The abandoned reader may stop after consuming only part of the
			// late bytes (observed once under a loaded -race run). The late
			// Observe has still run; release the blocked writer instead of
			// hanging the package until the global test timeout.
			_ = writer.CloseWithError(io.ErrClosedPipe)
			<-lateWritten
			t.Logf("round %d: abandoned reader consumed part of the late bytes; delivered=%d parked=%d",
				round, body.delivered.Load(), body.parked.Load())
		}
		_ = writer.Close()
	}
}
