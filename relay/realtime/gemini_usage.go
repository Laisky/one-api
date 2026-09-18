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
// Missing modality breakdowns are NOT assumed to be cheap text. Transcription
// and audible text are already in the server's TEXT output partition; never
// tokenize the transcript again. Thinking/tool totals are accepted only when
// the aggregate unambiguously establishes whether they are additional tokens.
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
	// prompt/response only as zero; the partitions and total must still match.
	// Never infer a nonzero missing count from a residual.
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
	tool, err := geminiPartition(u.ToolDetails, u.ToolPrompt, false)
	if err != nil {
		return Record{}, err
	}
	in, err := geminiPartition(u.PromptDetails, *u.Prompt, false)
	inputSubsetAdded := false
	if err != nil && errors.Is(err, ErrIncompleteUsage) && u.ToolPrompt > 0 && u.ToolPrompt <= *u.Prompt {
		in, err = geminiPartition(u.PromptDetails, *u.Prompt-u.ToolPrompt, false)
		if err == nil {
			for i := range in {
				in[i] += tool[i]
			}
			inputSubsetAdded = true
		}
	}
	if err != nil {
		return Record{}, err
	}
	out, err := geminiPartition(u.ResponseDetails, *u.Response, true)
	outputSubsetAdded := false
	if err != nil && errors.Is(err, ErrIncompleteUsage) && u.Thoughts > 0 && u.Thoughts <= *u.Response {
		out, err = geminiPartition(u.ResponseDetails, *u.Response-u.Thoughts, true)
		if err == nil {
			out[0] += u.Thoughts
			outputSubsetAdded = true
		}
	}
	if err != nil {
		return Record{}, err
	}
	t := Tokens{Input: *u.Prompt, Output: *u.Response, Text: in[0], Audio: in[1], Image: in[2], Video: in[3], OutputText: out[0], OutputAudio: out[1]}
	base := t.Input + t.Output
	// The Live reference describes total as prompt + response, whereas newer
	// receipts can include distinct thinking/tool-use counters. Do not guess
	// which bucket owns an unexplained residual or double-charge subsets.
	switch {
	case *u.Total == base:
		if u.Thoughts > t.OutputText || tool[0] > t.Text || tool[1] > t.Audio || tool[2] > t.Image || tool[3] > t.Video {
			return Record{}, errors.Wrap(ErrInvalidUsage, "Gemini subset exceeds parent count")
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
	if _, err := geminiPartition(u.CacheDetails, u.Cached, false); err != nil {
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
// contains upstream counts, total is its parent, and output restricts allowed
// output modalities. Returns: text/audio/image/video counts or an error.
func geminiPartition(details []GeminiModalityCount, total int64, output bool) ([4]int64, error) {
	var result [4]int64
	if len(details) > 4 {
		return result, errors.Wrap(ErrInvalidUsage, "too many Gemini modalities")
	}
	var seen [4]bool
	var sum int64
	for _, d := range details {
		var i int
		switch d.Modality {
		case "TEXT":
			i = 0
		case "AUDIO":
			i = 1
		case "IMAGE":
			i = 2
		case "VIDEO":
			i = 3
		default:
			return result, errors.Wrap(ErrInvalidUsage, "unknown Gemini modality")
		}
		if seen[i] || d.Count < 0 || d.Count > total-sum || (output && i > 1) {
			return result, errors.Wrap(ErrInvalidUsage, "invalid Gemini modality partition")
		}
		seen[i] = true
		result[i] = d.Count
		sum += d.Count
	}
	if sum != total {
		return result, errors.Wrap(ErrIncompleteUsage, "Gemini modality partition is incomplete")
	}
	return result, nil
}
