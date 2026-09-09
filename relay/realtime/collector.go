package realtime

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/Laisky/errors/v2"
)

// Ledger is a per-connection, single-writer collector. Only upstream server
// frames may be passed to Observe. Read it after the upstream pump has stopped.
type Ledger struct {
	Records               []Record `json:"records,omitempty"`
	Issues                []string `json:"issues,omitempty"`
	InputTokens           int64    `json:"input_tokens"`
	OutputTokens          int64    `json:"output_tokens"`
	transcriptionModel    string
	itemModels            map[string]string
	seen                  map[string]struct{}
	pendingResponses      map[string]struct{}
	pendingTranscriptions map[string]struct{}
	unkeyedUsageGap       bool
	stopped               bool
}

type wireDetails struct {
	Text   int64        `json:"text_tokens"`
	Audio  int64        `json:"audio_tokens"`
	Image  int64        `json:"image_tokens"`
	Cached *int64       `json:"cached_tokens"`
	Split  *wireDetails `json:"cached_tokens_details"`
}

type wireUsage struct {
	Type          string       `json:"type"`
	Input         *int64       `json:"input_tokens"`
	Output        *int64       `json:"output_tokens"`
	Total         *int64       `json:"total_tokens"`
	InputDetails  *wireDetails `json:"input_token_details"`
	OutputDetails *wireDetails `json:"output_token_details"`
	Seconds       *float64     `json:"seconds"`
}

type wireEvent struct {
	Type         string          `json:"type"`
	EventID      string          `json:"event_id"`
	ItemID       string          `json:"item_id"`
	ContentIndex *int64          `json:"content_index"`
	Usage        *wireUsage      `json:"usage"`
	Session      json.RawMessage `json:"session"`
	Item         struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	} `json:"item"`
	Response *struct {
		ID    string     `json:"id"`
		Usage *wireUsage `json:"usage"`
	} `json:"response"`
}

// NewLedger returns an empty ledger with independent response and transcription
// deduplication namespaces. A connected but idle session has no chargeable records.
func NewLedger() *Ledger {
	return &Ledger{itemModels: make(map[string]string), seen: make(map[string]struct{}),
		pendingResponses: make(map[string]struct{}), pendingTranscriptions: make(map[string]struct{})}
}

// Observe processes one upstream frame and returns an actionable accounting
// error without retaining transcripts, audio, credentials, or full server frames.
// ErrAmbiguousCache retains a validated receipt with a lower-bound charge.
// ErrLedgerLimit requires the transport to stop; no records may be silently evicted.
func (l *Ledger) Observe(message []byte) error {
	if l.stopped {
		return errors.WithStack(ErrLedgerLimit)
	}
	var event wireEvent
	if err := json.Unmarshal(message, &event); err != nil {
		l.unkeyedUsageGap = true
		return l.issue("invalid realtime server event")
	}
	if len(event.EventID) > MaxIdentifierBytes || len(event.ItemID) > MaxIdentifierBytes ||
		len(event.Item.ID) > MaxIdentifierBytes || (event.Response != nil && len(event.Response.ID) > MaxIdentifierBytes) {
		l.unkeyedUsageGap = true
		_ = l.issue("realtime identity exceeds billing limit")
		return l.stop()
	}
	switch event.Type {
	case "session.created", "session.updated", "transcription_session.created", "transcription_session.updated":
		return l.updateSession(event.Session)
	case "input_audio_buffer.committed":
		return l.bindItem(event.ItemID)
	case "conversation.item.created", "conversation.item.added":
		if event.Item.Role == "user" {
			for _, content := range event.Item.Content {
				if content.Type == "input_audio" {
					return l.bindItem(event.Item.ID)
				}
			}
		}
		return nil
	case "response.created":
		if event.Response != nil && event.Response.ID != "" {
			key := "response:" + event.Response.ID
			if _, done := l.seen[key]; !done {
				return l.trackResponse(key)
			}
		}
		return nil
	case "conversation.item.input_audio_transcription.failed":
		delete(l.pendingTranscriptions, event.ItemID)
		return nil
	case "response.done":
		if event.Response == nil {
			l.unkeyedUsageGap = true
			return l.issue("response.done lacks response")
		}
		key := "response:" + event.Response.ID
		if event.Response.ID == "" {
			key = "event:" + event.EventID
		}
		if event.Response.ID == "" && event.EventID == "" {
			l.unkeyedUsageGap = true
			return l.issue("response.done lacks identity")
		}
		if _, ok := l.seen[key]; ok {
			return nil
		}
		record, err := decodeUsage(event.Response.Usage, false)
		if err != nil && !errors.Is(err, ErrAmbiguousCache) {
			_ = l.recordIssue(err)
			if limitErr := l.trackResponse(key); limitErr != nil {
				return limitErr
			}
			return err
		}
		return l.acceptRecord(record, key, l.pendingResponses, key, err)
	case "conversation.item.input_audio_transcription.completed":
		if event.ItemID == "" || event.ContentIndex == nil || *event.ContentIndex < 0 {
			if _, bound := l.itemModels[event.ItemID]; bound {
				l.pendingTranscriptions[event.ItemID] = struct{}{}
			} else {
				l.unkeyedUsageGap = true
			}
			return l.recordIssue(errors.Wrap(ErrInvalidUsage, "transcription lacks item/content identity"))
		}
		key := "transcription:" + event.ItemID + ":" + strconv.FormatInt(*event.ContentIndex, 10)
		if _, ok := l.seen[key]; ok {
			return nil
		}
		model, bound := l.itemModels[event.ItemID]
		if !bound || model == "" {
			l.unkeyedUsageGap = true
			return l.issue("transcription lacks acknowledged item model")
		}
		record, err := decodeUsage(event.Usage, true)
		if err != nil && !errors.Is(err, ErrAmbiguousCache) {
			// The item is already bounded by itemModels. A later valid receipt
			// clears this gap without keeping an obsolete reservation floor.
			l.pendingTranscriptions[event.ItemID] = struct{}{}
			return l.recordIssue(err)
		}
		record.Model = model
		return l.acceptRecord(record, key, l.pendingTranscriptions, event.ItemID, err)
	default:
		// Transcript/audio deltas, playback, truncation, deletion, rate limits,
		// tool events and client-shaped frames are not billing receipts.
		return nil
	}
}

// acceptRecord stores record, updates deduplication and clears its pending key.
// Uncertainty is retained but does not discard measured tokens. At capacity the
// last accepted receipt remains billable before the transport is told to stop.
func (l *Ledger) acceptRecord(record Record, key string, pending map[string]struct{}, pendingKey string, uncertainty error) error {
	if err := l.appendRecord(record); err != nil {
		l.unkeyedUsageGap = true
		return err
	}
	l.seen[key] = struct{}{}
	delete(pending, pendingKey)
	if uncertainty != nil {
		_ = l.recordIssue(uncertainty)
	}
	if len(l.Records) >= MaxRecords {
		return l.stop()
	}
	return uncertainty
}

// Finish records unresolved work at disconnect without fabricating provider
// usage. Settlement separately decides whether the reservation must be retained.
func (l *Ledger) Finish() {
	if len(l.pendingResponses) != 0 {
		_ = l.issue("connection ended before final response usage")
	}
	if len(l.pendingTranscriptions) != 0 {
		_ = l.issue("connection ended before final transcription usage")
	}
}

// HasUsageGap reports missing evidence, not mere uncertainty about a cache split.
// A corrected receipt clears its keyed gap even though diagnostic history remains.
func (l *Ledger) HasUsageGap() bool {
	return l != nil && (l.unkeyedUsageGap || len(l.pendingResponses) > 0 || len(l.pendingTranscriptions) > 0)
}

// issue records a bounded diagnostic under a classifiable sentinel error.
func (l *Ledger) issue(message string) error {
	return l.recordIssue(errors.Wrap(ErrIncompleteUsage, message))
}

// recordIssue retains err's classification and bounded, payload-free log text.
func (l *Ledger) recordIssue(err error) error {
	if len(l.Issues) < MaxIssues {
		l.Issues = append(l.Issues, boundedDiagnostic(err.Error()))
	}
	return err
}

// bindItem snapshots acknowledged transcription settings once for an input item.
// A later session update cannot relabel asynchronously completed transcription.
func (l *Ledger) bindItem(id string) error {
	if id == "" {
		return nil
	}
	if _, exists := l.itemModels[id]; exists {
		return nil
	}
	l.itemModels[id] = l.transcriptionModel
	if l.transcriptionModel != "" {
		l.pendingTranscriptions[id] = struct{}{}
	}
	if len(l.itemModels) >= MaxItems {
		return l.stop()
	}
	return nil
}

// updateSession reads GA nested or legacy flat transcription settings from a
// server acknowledgement. An explicit null disables transcription.
func (l *Ledger) updateSession(raw json.RawMessage) error {
	var session struct {
		Transcription json.RawMessage `json:"input_audio_transcription"`
		Audio         struct {
			Input struct {
				Transcription json.RawMessage `json:"transcription"`
			} `json:"input"`
		} `json:"audio"`
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return l.issue("invalid realtime session acknowledgement")
	}
	selected := session.Audio.Input.Transcription
	if len(selected) == 0 {
		selected = session.Transcription
	}
	if len(selected) == 0 {
		return nil
	}
	var transcription *struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(selected, &transcription); err != nil {
		return l.issue("invalid acknowledged transcription model")
	}
	if transcription != nil && len(transcription.Model) > MaxModelBytes {
		l.unkeyedUsageGap = true
		_ = l.issue("realtime transcription model exceeds billing limit")
		return l.stop()
	}
	l.transcriptionModel = ""
	if transcription != nil {
		l.transcriptionModel = transcription.Model
	}
	return nil
}

// appendRecord stores a validated receipt and updates aggregate log counts with
// overflow checks. The caller marks deduplication only after this succeeds.
func (l *Ledger) appendRecord(record Record) error {
	if len(l.Records) >= MaxRecords {
		return l.stop()
	}
	if record.Tokens.Input > math.MaxInt64-l.InputTokens || record.Tokens.Output > math.MaxInt64-l.OutputTokens {
		return l.recordIssue(errors.Wrap(ErrQuotaOverflow, "realtime session token count overflow"))
	}
	input, output := l.InputTokens+record.Tokens.Input, l.OutputTokens+record.Tokens.Output
	if input > math.MaxInt64-output {
		return l.recordIssue(errors.Wrap(ErrQuotaOverflow, "realtime session total token overflow"))
	}
	l.Records = append(l.Records, record)
	l.InputTokens, l.OutputTokens = input, output
	return nil
}

// decodeUsage normalizes one server receipt. Missing modality overhead is text.
// Mixed cache totals survive as unallocated cache and ErrAmbiguousCache, allowing
// conservative pricing without presenting an inferred split as authoritative.
func decodeUsage(usage *wireUsage, transcription bool) (Record, error) {
	if usage == nil {
		return Record{}, errors.Wrap(ErrIncompleteUsage, "final realtime event lacks usage")
	}
	if usage.Type == "duration" {
		if !transcription || usage.Seconds == nil || *usage.Seconds < 0 || math.IsNaN(*usage.Seconds) || math.IsInf(*usage.Seconds, 0) {
			return Record{}, errors.Wrap(ErrInvalidUsage, "invalid realtime duration receipt")
		}
		return Record{Duration: true, Seconds: *usage.Seconds}, nil
	}
	if usage.Type != "" && usage.Type != "tokens" {
		return Record{}, errors.Wrap(ErrInvalidUsage, "unsupported realtime usage type")
	}
	if usage.Input == nil || usage.Output == nil {
		return Record{}, errors.Wrap(ErrIncompleteUsage, "realtime usage lacks input/output counts")
	}
	t := Tokens{Input: *usage.Input, Output: *usage.Output}
	if t.Input < 0 || t.Output < 0 || t.Input > math.MaxInt64-t.Output {
		return Record{}, errors.Wrap(ErrInvalidUsage, "invalid realtime aggregate token count")
	}
	if usage.Total != nil && *usage.Total != t.Input+t.Output {
		return Record{}, errors.Wrap(ErrInvalidUsage, "inconsistent realtime total tokens")
	}
	if d := usage.InputDetails; d != nil {
		if d.Text < 0 || d.Audio < 0 || d.Image < 0 || d.Text > t.Input || d.Audio > t.Input-d.Text || d.Image > t.Input-d.Text-d.Audio {
			return Record{}, errors.Wrap(ErrInvalidUsage, "invalid realtime input detail")
		}
		t.Audio, t.Image = d.Audio, d.Image
	}
	t.Text = t.Input - t.Audio - t.Image
	if d := usage.OutputDetails; d != nil {
		if d.Text < 0 || d.Audio < 0 || d.Text > t.Output || d.Audio > t.Output-d.Text {
			return Record{}, errors.Wrap(ErrInvalidUsage, "invalid realtime output detail")
		}
		t.OutputAudio = d.Audio
	}
	t.OutputText = t.Output - t.OutputAudio
	if transcription && t.Input > 0 && usage.InputDetails == nil {
		return Record{}, errors.Wrap(ErrIncompleteUsage, "transcription token usage lacks audio/text split")
	}
	if d := usage.InputDetails; d != nil {
		if d.Cached != nil && (*d.Cached < 0 || *d.Cached > t.Input) {
			return Record{}, errors.Wrap(ErrInvalidUsage, "invalid realtime cache total")
		}
		if d.Split != nil {
			t.CachedText, t.CachedAudio, t.CachedImage = d.Split.Text, d.Split.Audio, d.Split.Image
			if err := t.Validate(); err != nil {
				return Record{}, err
			}
			if d.Cached != nil && *d.Cached != t.CachedText+t.CachedAudio+t.CachedImage {
				return Record{}, errors.Wrap(ErrInvalidUsage, "inconsistent realtime cache total")
			}
		} else if d.Cached != nil && *d.Cached != 0 {
			cached := *d.Cached
			switch {
			case cached == t.Input:
				t.CachedText, t.CachedAudio, t.CachedImage = t.Text, t.Audio, t.Image
			case t.Text == t.Input:
				t.CachedText = cached
			case t.Audio == t.Input:
				t.CachedAudio = cached
			case t.Image == t.Input:
				t.CachedImage = cached
			default:
				t.CachedUnallocated = cached
			}
		}
	}
	if err := t.Validate(); err != nil {
		return Record{}, err
	}
	if t.CachedUnallocated > 0 {
		return Record{Tokens: t}, errors.WithStack(ErrAmbiguousCache)
	}
	return Record{Tokens: t}, nil
}
