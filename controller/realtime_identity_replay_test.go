package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewMalformedTranscriptionReplayDoesNotRestoreReservation verifies both
// event orders against production settlement. A malformed duplicate must not
// recreate pending work after the authoritative item receipt has been accepted.
func TestReviewMalformedTranscriptionReplayDoesNotRestoreReservation(t *testing.T) {
	t.Parallel()
	for _, index := range []string{"", `,"content_index":null`, `,"content_index":-1`} {
		for _, invalidFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/invalid_first_%t", index, invalidFirst), func(t *testing.T) {
				ledger := realtime.NewLedger()
				require.NoError(t, ledger.Observe([]byte(`{"type":"session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`)))
				require.NoError(t, ledger.Observe([]byte(`{"type":"input_audio_buffer.committed","item_id":"item"}`)))
				valid := []byte(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"item","content_index":0,"usage":{"type":"duration","seconds":0.5}}`)
				invalid := []byte(fmt.Sprintf(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"item"%s,"usage":{"type":"duration","seconds":100}}`, index))
				if invalidFirst {
					require.Error(t, ledger.Observe(invalid))
					require.NoError(t, ledger.Observe(valid))
				} else {
					require.NoError(t, ledger.Observe(valid))
					require.Error(t, ledger.Observe(invalid))
				}
				require.NoError(t, ledger.Observe(valid), "authoritative replay must remain idempotent")
				ledger.Finish()
				require.Len(t, ledger.Records, 1)
				require.False(t, ledger.HasUsageGap(), "a rejected replay cannot undo successful completion")
				result, metadata := prepareRealtimeReceiptSettlement(quota.ComputeInput{
					Usage: &rmodel.Usage{Realtime: ledger}, ModelName: "gpt-realtime",
					ModelRatio: 2, GroupRatio: 1, PricingAdaptor: &openai.Adaptor{},
				}, 999, nil)
				require.Equal(t, int64(25), result.TotalQuota, "charge only the measured half-second transcription")
				require.NotContains(t, metadata, "estimated_charge")
				require.NotEmpty(t, result.BillingIssues, "keep the malformed-event diagnostic without inventing usage")
			})
		}
	}
}
