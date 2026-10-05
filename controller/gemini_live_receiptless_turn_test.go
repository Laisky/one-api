package controller

import (
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestGeminiLiveReceiptlessTurnKeepsItsEvidence checks a turn the provider
// ends without usageMetadata before the next turn's receipt. That receipt
// prices only its own generation, so the ended turn's context, input and
// streamed audio must stay in the settlement as an estimate instead of being
// erased. A session whose single turn carries its receipt is the control.
// Parameters: t owns real loopback sockets and the in-memory ledger. Returns:
// none.
func TestGeminiLiveReceiptlessTurnKeepsItsEvidence(t *testing.T) {
	const measured = int64(30_000)
	charges := make(map[string]int64)
	transcript := []byte(`{"serverContent":{"outputTranscription":{"text":"next"}}}`)
	for _, tc := range []struct {
		name        string
		receiptless bool
		between     [][]byte // frames between the bare turnComplete and the next turn
		combined    bool     // the next turn's output, receipt and turnComplete share one frame
	}{
		{name: "receipted_turn"},
		{name: "turn_without_receipt_before_next_receipt", receiptless: true},
		{name: "transcription_after_turn_without_receipt", receiptless: true, between: [][]byte{transcript}},
		{name: "repeated_transcriptions_after_turn_without_receipt", receiptless: true, between: [][]byte{transcript, transcript, transcript}},
		{name: "next_turn_in_one_frame", receiptless: true, combined: true},
	} {
		receiptless := tc.receiptless
		t.Run(tc.name, func(t *testing.T) {
			env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: 1_000_000, TokenQuota: 1_000_000, Group: 1}, func(conn *websocket.Conn) error {
				if _, _, err := conn.ReadMessage(); err != nil {
					return errors.Wrap(err, "read initial input")
				}
				if err := conn.WriteMessage(websocket.TextMessage, liveAudioOutputFrame(4)); err != nil {
					return errors.Wrap(err, "stream the first turn")
				}
				if receiptless {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"serverContent":{"turnComplete":true}}`)); err != nil {
						return errors.Wrap(err, "end the first turn without usage")
					}
					for _, frame := range tc.between {
						if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
							return errors.Wrap(err, "send a frame between turns")
						}
					}
					if tc.combined {
						frame := []byte(`{"serverContent":{"modelTurn":{"parts":[{"text":"next"}]},"turnComplete":true},"usageMetadata":{"promptTokenCount":8000,"responseTokenCount":12000,"totalTokenCount":20000,"promptTokensDetails":[{"modality":"TEXT","tokenCount":8000}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":12000}]}}`)
						if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
							return errors.Wrap(err, "send the next turn in one frame")
						}
						return nil
					}
					if err := conn.WriteMessage(websocket.TextMessage, liveAudioOutputFrame(1)); err != nil {
						return errors.Wrap(err, "stream the next turn")
					}
				}
				if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(8000, 0, 0, 12000, 0, 0)); err != nil {
					return errors.Wrap(err, "send the final receipt")
				}
				return nil
			})
			conn := env.connectSetup(`{"setup":{"model":"` + liveBudgetModel + `"}}`)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(liveTextFrame(600))))
			_ = awaitClose(conn, 3*time.Second)
			env.finish()

			logs := env.consumeLogs()
			require.Len(t, logs, 1)
			for _, rows := range logs {
				require.Len(t, rows, 1, "session settles once")
				row := rows[0]
				if !receiptless {
					require.EqualValues(t, measured, row.Quota)
					require.Equal(t, true, row.Metadata["realtime_billing_complete"])
					require.Nil(t, row.Metadata[model.LogMetadataKeyEstimatedCharge])
					continue
				}
				charges[tc.name] = int64(row.Quota)
				require.EqualValues(t, measured, row.Metadata["realtime_observed_quota"], "the receipt stays authoritative for its own turn")
				require.Greater(t, int64(row.Quota), measured, "the receipt-less turn's incurred work is still charged")
				require.LessOrEqual(t, float64(row.Quota), row.Metadata["realtime_budget_reserved"].(float64), "estimates never exceed the prepaid reservation")
				require.Equal(t, false, row.Metadata["realtime_billing_complete"])
				require.Equal(t, true, row.Metadata[model.LogMetadataKeyEstimatedCharge])
			}
		})
	}
	single, repeated := charges["transcription_after_turn_without_receipt"], charges["repeated_transcriptions_after_turn_without_receipt"]
	require.Positive(t, single)
	require.Less(t, repeated-single, int64(100), "the ended turn is rolled over once, not once per later frame")
}
