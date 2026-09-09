package realtime

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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
	ContentIndex int64           `json:"content_index"`
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
// Only final response and transcription usage is billable; response status does
// not erase authoritative usage from cancelled, incomplete, or failed responses.
func (l *Ledger) Observe(message []byte) error {
	var event wireEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return l.issue("invalid realtime server event")
	}
	switch event.Type {
	case "session.created", "session.updated", "transcription_session.created", "transcription_session.updated":
		return l.updateSession(event.Session)
	case "input_audio_buffer.committed":
		l.bindItem(event.ItemID)
		return nil
	case "conversation.item.created", "conversation.item.added":
		if event.Item.Role == "user" {
			for _, content := range event.Item.Content {
				if content.Type == "input_audio" {
					l.bindItem(event.Item.ID)
					break
				}
			}
		}
		return nil
	case "response.created":
		if event.Response != nil && event.Response.ID != "" {
			if _, done := l.seen["response:"+event.Response.ID]; !done {
				l.pendingResponses[event.Response.ID] = struct{}{}
			}
		}
		return nil
	case "conversation.item.input_audio_transcription.failed":
		delete(l.pendingTranscriptions, event.ItemID)
		return nil
	case "response.done":
		if event.Response == nil {
			return l.issue("response.done lacks response")
		}
		key := "response:" + event.Response.ID
		if event.Response.ID == "" {
			key = "event:" + event.EventID
		}
		if event.Response.ID == "" && event.EventID == "" {
			return l.issue("response.done lacks identity")
		}
		if _, ok := l.seen[key]; ok {
			return nil
		}
		record, err := decodeUsage(event.Response.Usage, false)
		if err != nil {
			return l.issue(err.Error())
		}
		if err := l.appendRecord(record); err != nil {
			return err
		}
		l.seen[key] = struct{}{}
		delete(l.pendingResponses, event.Response.ID)
		return nil
	case "conversation.item.input_audio_transcription.completed":
		if event.ItemID == "" || event.ContentIndex < 0 {
			return l.issue("transcription lacks item identity")
		}
		key := "transcription:" + event.ItemID + ":" + strconv.FormatInt(event.ContentIndex, 10)
		if _, ok := l.seen[key]; ok {
			return nil
		}
		model, bound := l.itemModels[event.ItemID]
		if !bound {
			return l.issue("transcription lacks acknowledged item model")
		}
		if model == "" {
			return l.issue("transcription completed for an unconfigured item")
		}
		record, err := decodeUsage(event.Usage, true)
		if err != nil {
			return l.issue(err.Error())
		}
		record.Model = model
		if err := l.appendRecord(record); err != nil {
			return err
		}
		l.seen[key] = struct{}{}
		delete(l.pendingTranscriptions, event.ItemID)
		return nil
	default:
		// Transcript/audio deltas, playback acknowledgements, truncation, deletion,
		// rate limits, tool events, and client-shaped frames are not billing receipts.
		return nil
	}
}

// Finish records unresolved work at disconnect; it never fabricates usage from
// session duration, transcript length, or the proxy's pre-consumption estimate.
func (l *Ledger) Finish() {
	if len(l.pendingResponses) != 0 {
		_ = l.issue("connection ended before final response usage")
	}
	if len(l.pendingTranscriptions) != 0 {
		_ = l.issue("connection ended before final transcription usage")
	}
}

// issue retains a bounded, payload-free diagnostic and returns it as an error.
func (l *Ledger) issue(message string) error {
	if len(l.Issues) < 16 {
		l.Issues = append(l.Issues, message)
	}
	return fmt.Errorf("%s", message)
}

// bindItem snapshots acknowledged transcription settings once for an input item.
// A later session update cannot relabel asynchronously completed transcription.
func (l *Ledger) bindItem(id string) {
	if id == "" {
		return
	}
	if _, exists := l.itemModels[id]; exists {
		return
	}
	l.itemModels[id] = l.transcriptionModel
	if l.transcriptionModel != "" {
		l.pendingTranscriptions[id] = struct{}{}
	}
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
	l.transcriptionModel = ""
	if transcription != nil {
		l.transcriptionModel = transcription.Model
	}
	return nil
}

// appendRecord stores a validated receipt and updates aggregate log counts with
// overflow checks. The caller marks deduplication only after this succeeds.
func (l *Ledger) appendRecord(record Record) error {
	if record.Tokens.Input > math.MaxInt64-l.InputTokens || record.Tokens.Output > math.MaxInt64-l.OutputTokens {
		return l.issue("realtime session token count overflow")
	}
	input, output := l.InputTokens+record.Tokens.Input, l.OutputTokens+record.Tokens.Output
	if input > math.MaxInt64-output {
		return l.issue("realtime session total token overflow")
	}
	l.Records = append(l.Records, record)
	l.InputTokens, l.OutputTokens = input, output
	return nil
}

// decodeUsage normalizes one server receipt. Missing modality overhead is text;
// missing mixed-modality cache splits are ambiguous and require reconciliation.
func decodeUsage(usage *wireUsage, transcription bool) (Record, error) {
	if usage == nil {
		return Record{}, fmt.Errorf("final realtime event lacks usage")
	}
	if usage.Type == "duration" {
		if !transcription || usage.Seconds == nil || *usage.Seconds < 0 || math.IsNaN(*usage.Seconds) || math.IsInf(*usage.Seconds, 0) {
			return Record{}, fmt.Errorf("invalid realtime duration receipt")
		}
		return Record{Duration: true, Seconds: *usage.Seconds}, nil
	}
	if usage.Type != "" && usage.Type != "tokens" {
		return Record{}, fmt.Errorf("unsupported realtime usage type")
	}
	if usage.Input == nil || usage.Output == nil {
		return Record{}, fmt.Errorf("realtime usage lacks input/output counts")
	}
	t := Tokens{Input: *usage.Input, Output: *usage.Output}
	if t.Input < 0 || t.Output < 0 || t.Input > math.MaxInt64-t.Output {
		return Record{}, fmt.Errorf("invalid realtime aggregate token count")
	}
	if usage.Total != nil && *usage.Total != t.Input+t.Output {
		return Record{}, fmt.Errorf("inconsistent realtime total tokens")
	}
	if d := usage.InputDetails; d != nil {
		if d.Text < 0 || d.Audio < 0 || d.Image < 0 || d.Text > t.Input || d.Audio > t.Input-d.Text || d.Image > t.Input-d.Text-d.Audio {
			return Record{}, fmt.Errorf("invalid realtime input detail")
		}
		t.Audio, t.Image = d.Audio, d.Image
	}
	t.Text = t.Input - t.Audio - t.Image
	if d := usage.OutputDetails; d != nil {
		if d.Text < 0 || d.Audio < 0 || d.Text > t.Output || d.Audio > t.Output-d.Text {
			return Record{}, fmt.Errorf("invalid realtime output detail")
		}
		t.OutputAudio = d.Audio
	}
	t.OutputText = t.Output - t.OutputAudio
	if transcription && t.Input > 0 && usage.InputDetails == nil {
		return Record{}, fmt.Errorf("transcription token usage lacks audio/text split")
	}
	if d := usage.InputDetails; d != nil {
		if d.Split != nil {
			t.CachedText, t.CachedAudio, t.CachedImage = d.Split.Text, d.Split.Audio, d.Split.Image
			if err := t.Validate(); err != nil {
				return Record{}, err
			}
			if d.Cached != nil && *d.Cached != t.CachedText+t.CachedAudio+t.CachedImage {
				return Record{}, fmt.Errorf("inconsistent realtime cache total")
			}
		} else if d.Cached != nil && *d.Cached != 0 {
			cached := *d.Cached
			switch {
			case t.Text == t.Input:
				t.CachedText = cached
			case t.Audio == t.Input:
				t.CachedAudio = cached
			case t.Image == t.Input:
				t.CachedImage = cached
			default:
				return Record{}, fmt.Errorf("mixed realtime cache lacks modality split")
			}
		}
	}
	if err := t.Validate(); err != nil {
		return Record{}, err
	}
	return Record{Tokens: t}, nil
}
