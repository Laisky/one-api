package openai

import (
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"

	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// meteredRealtimePump forwards an OpenAI session and returns server-only billing
// receipts after both pumps stop. Non-OpenAI providers retain the legacy pump.
// An idle session returns an explicit empty ledger, not an estimated token bill.
func meteredRealtimePump(client, upstream *websocket.Conn, lg glog.Logger) *rmodel.Usage {
	ledger := realtime.NewLedger()
	errc := make(chan error, 2)
	go func() { errc <- copyMeteredRealtimeUpstream(upstream, client, ledger) }()
	go func() { errc <- copyRealtimeClientToUpstream(client, upstream, true) }()
	if err := <-errc; err != nil && lg != nil {
		lg.Debug("metered realtime first direction closed", zap.Error(err))
	}
	_ = client.Close()
	_ = upstream.Close()
	if err := <-errc; err != nil && lg != nil {
		lg.Debug("metered realtime second direction closed", zap.Error(err))
	}
	ledger.Finish()
	return realtimeLedgerUsage(ledger)
}

// copyMeteredRealtimeUpstream observes only upstream text frames, before delivery
// to the client. Recoverable accounting errors do not block valid frames. At the
// ledger limit the boundary receipt is preserved and forwarded, then the session
// is closed instead of forwarding more work that the bounded meter cannot retain.
func copyMeteredRealtimeUpstream(src, dst *websocket.Conn, ledger *realtime.Ledger) error {
	for {
		kind, message, err := src.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				_ = dst.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(closeErr.Code, closeErr.Text), time.Now().Add(time.Second))
				return nil
			}
			return errors.WithStack(err)
		}
		var accountingErr error
		if kind == websocket.TextMessage {
			accountingErr = ledger.Observe(message)
		}
		if err := dst.WriteMessage(kind, message); err != nil {
			return errors.WithStack(err)
		}
		if errors.Is(accountingErr, realtime.ErrLedgerLimit) {
			_ = dst.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "realtime_billing_capacity"), time.Now().Add(time.Second))
			return accountingErr
		}
	}
}

// realtimeLedgerUsage builds legacy log counters without flattening the billing
// evidence. It is called once, after the ledger's single writer has terminated.
func realtimeLedgerUsage(ledger *realtime.Ledger) *rmodel.Usage {
	u := &rmodel.Usage{Realtime: ledger, PromptTokens: int(ledger.InputTokens),
		CompletionTokens: int(ledger.OutputTokens), TotalTokens: int(ledger.InputTokens + ledger.OutputTokens)}
	if len(ledger.Records) == 0 {
		return u
	}
	u.PromptTokensDetails = &rmodel.UsagePromptTokensDetails{CachedTokensDetails: &rmodel.UsageCachedTokensDetails{}}
	u.CompletionTokensDetails = &rmodel.UsageCompletionTokensDetails{}
	for _, record := range ledger.Records {
		t := record.Tokens
		p, o := u.PromptTokensDetails, u.CompletionTokensDetails
		p.TextTokens += int(t.Text)
		p.AudioTokens += int(t.Audio)
		p.ImageTokens += int(t.Image)
		p.CachedTokens += int(t.CachedText + t.CachedAudio + t.CachedImage + t.CachedUnallocated)
		p.CachedTokensDetails.TextTokens += int(t.CachedText)
		p.CachedTokensDetails.AudioTokens += int(t.CachedAudio)
		p.CachedTokensDetails.ImageTokens += int(t.CachedImage)
		o.TextTokens += int(t.OutputText)
		o.AudioTokens += int(t.OutputAudio)
	}
	return u
}
