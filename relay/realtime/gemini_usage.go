package realtime

import (
	"encoding/json"
	"math"

	"github.com/Laisky/errors/v2"
)

// GeminiModalityCount is a provider-reported partition, never a locally tokenized
// transcript or a duration estimate.
type GeminiModalityCount struct {
	Modality string `json:"modality"`
	Count    int64  `json:"tokenCount"`
}

// GeminiUsageMetadata preserves the independent usage counters in Google's Live
// API. Pointers distinguish an omitted aggregate from a reported zero.
type GeminiUsageMetadata struct {
	Prompt          *int64                `json:"promptTokenCount"`
	Response        *int64                `json:"responseTokenCount"`
	Total           *int64                `json:"totalTokenCount"`
	Cached          int64                 `json:"cachedContentTokenCount"`
	Thoughts        int64                 `json:"thoughtsTokenCount"`
	ToolPrompt      int64                 `json:"toolUsePromptTokenCount"`
	PromptDetails   []GeminiModalityCount `json:"promptTokensDetails"`
	ResponseDetails []GeminiModalityCount `json:"responseTokensDetails"`
	CacheDetails    []GeminiModalityCount `json:"cacheTokensDetails"`
	ToolDetails     []GeminiModalityCount `json:"toolUsePromptTokensDetails"`
}

// DecodeGeminiUsage validates and partitions a Live receipt. Parameters: raw is
// the upstream usageMetadata object. Returns: a record or a classifiable error.
// Missing modality breakdowns are NOT assumed to be cheap text: they stay in the
// dedicated unallocated buckets, which Cost prices at the cheapest chargeable
// modality. Google does not document promptTokensDetails as exhaustive, and live
// receipts routinely report a remainder (observed 2026-09-18: promptTokenCount
// 548 against TEXT 306 + AUDIO 222). Rejecting those receipts lost every real
// turn's billing and ended the session, so an unattributed remainder is now
// accounted rather than refused. Transcription and audible text are already in
// the server's TEXT output partition; never tokenize the transcript again.
// Thinking/tool totals are accepted only when the aggregate unambiguously
// establishes whether they are additional tokens.
func DecodeGeminiUsage(raw json.RawMessage) (Record, error) {
	if err := ValidateGeminiJSON(raw); err != nil {
		return Record{}, err
	}
	var u GeminiUsageMetadata
	if err := json.Unmarshal(raw, &u); err != nil {
		return Record{}, errors.Wrap(ErrInvalidUsage, "invalid Gemini usage metadata")
	}
	if u.Total == nil {
		return Record{}, errors.Wrap(ErrIncompleteUsage, "Gemini usage lacks aggregate counts")
	}
	// Protobuf JSON may omit zero-valued scalar fields. Accept an omitted
	// prompt/response only as zero; the aggregate must still reconcile. A missing
	// modality count is never invented from a residual: it stays unattributed.
	var zero int64
	if u.Prompt == nil {
		u.Prompt = &zero
	}
	if u.Response == nil {
		u.Response = &zero
	}
	for _, n := range []int64{*u.Prompt, *u.Response, *u.Total, u.Cached, u.Thoughts, u.ToolPrompt} {
		if n < 0 || n > math.MaxInt32 {
			return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini count outside int32 range")
		}
	}
	tool, _, err := geminiPartition(u.ToolDetails, u.ToolPrompt, false)
	if err != nil {
		return Record{}, err
	}
	in, inRemainder, err := geminiPartition(u.PromptDetails, *u.Prompt, false)
	if err != nil {
		return Record{}, err
	}
	// A remainder that is exactly the tool-use total means the modality details
	// describe the prompt without its tool tokens; that is an inclusive subset,
	// not an unattributed count.
	inputSubsetAdded := false
	if inRemainder > 0 && u.ToolPrompt == inRemainder {
		for i := range in {
			in[i] += tool[i]
		}
		inRemainder, inputSubsetAdded = 0, true
	}
	out, outRemainder, err := geminiPartition(u.ResponseDetails, *u.Response, true)
	if err != nil {
		return Record{}, err
	}
	outputSubsetAdded := false
	if outRemainder > 0 && u.Thoughts == outRemainder {
		out[0] += u.Thoughts
		outRemainder, outputSubsetAdded = 0, true
	}
	t := Tokens{Input: *u.Prompt, Output: *u.Response, Text: in[0], Audio: in[1], Image: in[2], Video: in[3],
		Unallocated: inRemainder, OutputText: out[0], OutputAudio: out[1], OutputUnallocated: outRemainder}
	base := t.Input + t.Output
	// The Live reference describes total as prompt + response, whereas the REST
	// reference and newer receipts add distinct thinking/tool-use counters. Do
	// not guess which modality owns an unexplained residual or double-charge a
	// subset that the aggregate already includes.
	switch {
	case *u.Total == base:
		if tool[0] > t.Text || tool[1] > t.Audio || tool[2] > t.Image || tool[3] > t.Video {
			return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini subset exceeds parent count")
		}
		// Google's Live reference documents total as prompt + response, while its
		// REST reference documents prompt + thoughts + response; live receipts use
		// both. When the reported output partition cannot contain the thinking
		// tokens, they are additional billable output text that the aggregate
		// omitted (observed 2026-09-18: thoughtsTokenCount 100 against an
		// AUDIO-only responseTokenCount of 61). Only the part that does not fit
		// is added, so an inclusive partition is never charged twice.
		if extra := u.Thoughts - t.OutputText; extra > 0 {
			t.Output += extra
			t.OutputText += extra
		}
	case *u.Total == base+u.Thoughts+u.ToolPrompt:
		if inputSubsetAdded || outputSubsetAdded {
			return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini counters contradict inclusive partitions")
		}
		t.Input += u.ToolPrompt
		t.Text += tool[0]
		t.Audio += tool[1]
		t.Image += tool[2]
		t.Video += tool[3]
		t.Output += u.Thoughts
		t.OutputText += u.Thoughts
	default:
		return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini total does not reconcile")
	}
	t.ReasoningTokens = u.Thoughts
	if _, _, err := geminiPartition(u.CacheDetails, u.Cached, false); err != nil {
		return Record{}, err
	}
	if u.Cached > 0 {
		// These Live models publish no context-cache discount. Retaining an
		// ambiguous cache receipt as ordinary input would silently overcharge.
		return Record{}, errors.Wrap(ErrIncompleteUsage, "Gemini Live cache pricing is not published")
	}
	if err := t.Validate(); err != nil {
		return Record{}, err
	}
	return Record{Tokens: t}, nil
}

// geminiPartition validates a disjoint modality partition. Parameters: details
// contains upstream counts, total is its parent, and output restricts the
// modalities a response may be attributed to. Returns: text/audio/image/video
// counts, the remainder of total that no known modality claimed, and an error
// for a structurally impossible partition. A modality Google adds later, such as
// DOCUMENT or MODALITY_UNSPECIFIED, stays in the remainder instead of ending a
// paid session; only counts that contradict their parent are rejected.
func geminiPartition(details []GeminiModalityCount, total int64, output bool) ([4]int64, int64, error) {
	var result [4]int64
	if len(details) > 8 {
		return result, 0, errors.Wrap(ErrInvalidUsage, "too many Gemini modalities")
	}
	var seen [4]bool
	var reported, attributed int64
	for _, d := range details {
		i := -1
		switch d.Modality {
		case "TEXT":
			i = 0
		case "AUDIO":
			i = 1
		case "IMAGE":
			i = 2
		case "VIDEO":
			i = 3
		}
		if output && i > 1 {
			// These models emit only text and audio. An unexpected response
			// modality is unattributed rather than silently priced as audio.
			i = -1
		}
		if d.Count < 0 || d.Count > total-reported || (i >= 0 && seen[i]) {
			return result, 0, errors.Wrap(ErrInvalidUsage, "invalid Gemini modality partition")
		}
		reported += d.Count
		if i >= 0 {
			seen[i] = true
			result[i] = d.Count
			attributed += d.Count
		}
	}
	// Counts under a modality this build does not price stay in the remainder
	// together with the counts the provider never broke down at all.
	return result, total - attributed, nil
}
