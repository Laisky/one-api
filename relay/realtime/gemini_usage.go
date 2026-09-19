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
// Missing modality breakdowns stay unallocated and use minimum-rate pricing.
// Classify aggregate inclusion before refining partitions: a missing modality
// count that happens to equal a tool/thinking counter is not evidence of overlap.
// Transcriptions are already in the provider's TEXT partition and are never
// tokenized again. Thinking uses unallocated output before adding proven excess.
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
	// prompt/response only as zero; the aggregate must still reconcile.
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
	tool, toolRemainder, err := geminiPartition(u.ToolDetails, u.ToolPrompt, false)
	if err != nil {
		return Record{}, err
	}
	in, inRemainder, err := geminiPartition(u.PromptDetails, *u.Prompt, false)
	if err != nil {
		return Record{}, err
	}
	out, outRemainder, err := geminiPartition(u.ResponseDetails, *u.Response, true)
	if err != nil {
		return Record{}, err
	}
	t := Tokens{Input: *u.Prompt, Output: *u.Response, Text: in[0], Audio: in[1], Image: in[2], Video: in[3],
		Unallocated: inRemainder, OutputText: out[0], OutputAudio: out[1], OutputUnallocated: outRemainder}
	base := t.Input + t.Output
	switch {
	case *u.Total == base:
		if u.ToolPrompt > t.Input {
			return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini tool subset exceeds prompt count")
		}
		// A known tool modality is a lower bound for the same prompt modality.
		// Fill only its deficit from unallocated input; other tool tokens may
		// already overlap attributed prompt tokens. Never invent extra input.
		parents := []*int64{&t.Text, &t.Audio, &t.Image, &t.Video}
		for i, parent := range parents {
			missing := max(tool[i]-*parent, 0)
			if missing > t.Unallocated {
				return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini subset exceeds parent count")
			}
			*parent += missing
			t.Unallocated -= missing
		}
		// Live receipts sometimes exclude thinking even from totalTokenCount.
		// Existing output text and unknown output can already contain thoughts;
		// only thinking beyond that capacity proves additional output tokens.
		if remaining := u.Thoughts - t.OutputText; remaining > 0 {
			reclassified := min(remaining, t.OutputUnallocated)
			t.OutputText += reclassified
			t.OutputUnallocated -= reclassified
			if remaining -= reclassified; remaining > 0 {
				t.Output += remaining
				t.OutputText += remaining
			}
		}
	case *u.Total == base+u.Thoughts+u.ToolPrompt:
		// The aggregate proves these counters are additional. Incomplete prompt
		// or response details cannot contradict that merely by numerical equality.
		t.Input += u.ToolPrompt
		t.Text += tool[0]
		t.Audio += tool[1]
		t.Image += tool[2]
		t.Video += tool[3]
		t.Unallocated += toolRemainder
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
