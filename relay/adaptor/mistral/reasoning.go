package mistral

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"

	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/model"
)

// reasoningRequest translates compatibility fields without mutating request.
// Mistral uses max_tokens, random_seed, and native thinking chunks in history.
// Sources: https://docs.mistral.ai/studio/conversations/reasoning and
// https://docs.mistral.ai/api/endpoint/chat (verified 2026-09-24).
func reasoningRequest(request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	out := *request
	out.Messages = slices.Clone(request.Messages)
	if out.ReasoningEffort == nil && out.Reasoning != nil {
		out.ReasoningEffort = out.Reasoning.Effort
	}
	out.Reasoning = nil
	// Keep the previous omission behavior for models without advertised effort
	// support. Use catalog metadata rather than a fixed list of hybrid aliases;
	// supported models retain caller values for upstream validation.
	if cfg, ok := ModelRatios[out.Model]; !ok || len(cfg.SupportedReasoningEfforts) == 0 {
		out.ReasoningEffort = nil
	}
	if out.MaxTokens == 0 && out.MaxCompletionTokens != nil {
		out.MaxTokens = *out.MaxCompletionTokens
	}
	out.MaxCompletionTokens = nil
	for i := range out.Messages {
		message := &out.Messages[i]
		if message.Role != "assistant" {
			continue
		}
		reasoning := message.ReasoningContent
		if reasoning == nil {
			reasoning = message.Reasoning
		}
		if reasoning == nil {
			reasoning = message.Thinking
		}
		if reasoning == nil {
			continue
		}
		encoded, err := json.Marshal(message.Content)
		if err != nil {
			return nil, errors.Wrap(err, "encode Mistral history")
		}
		var chunks []json.RawMessage
		var text string
		if len(encoded) > 0 && encoded[0] == '[' {
			if err := json.Unmarshal(encoded, &chunks); err != nil {
				return nil, err
			}
		} else if !bytes.Equal(encoded, []byte("null")) {
			if err := json.Unmarshal(encoded, &text); err != nil {
				return nil, errors.Wrap(err, "unsupported assistant history")
			}
			chunk, _ := json.Marshal(map[string]string{"type": "text", "text": text})
			chunks = append(chunks, chunk)
		}
		hasThinking := false
		for _, chunk := range chunks {
			var fields struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(chunk, &fields); err != nil {
				return nil, err
			}
			hasThinking = hasThinking || fields.Type == "thinking"
		}
		if !hasThinking && *reasoning != "" {
			chunk, _ := json.Marshal(map[string]any{"type": "thinking", "thinking": []map[string]string{{"type": "text", "text": *reasoning}}})
			chunks = append([]json.RawMessage{chunk}, chunks...)
		}
		message.Content = chunks
		message.ReasoningContent, message.Reasoning, message.Thinking = nil, nil, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, errors.Wrap(err, "encode Mistral request")
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		return nil, err
	}
	if seed, ok := wire["seed"]; ok {
		// An explicit native random_seed in extra_body has precedence.
		if _, native := wire["random_seed"]; !native {
			wire["random_seed"] = seed
		}
		delete(wire, "seed")
	}
	return wire, nil
}

// normalizeThinking converts only documented text/thinking response chunks to
// the shared OpenAI content/reasoning_content shape. Other fields, including
// tool calls, usage, finish reasons, and errors, remain intact. Unknown chunk
// types fail visibly rather than silently dropping information.
func normalizeThinking(raw []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.Wrap(err, "decode Mistral response")
	}
	if _, ok := body["choices"]; !ok {
		return raw, nil
	}
	var choices []map[string]json.RawMessage
	if err := json.Unmarshal(body["choices"], &choices); err != nil {
		return nil, err
	}
	changed := false
	for _, choice := range choices {
		for _, field := range []string{"message", "delta"} {
			var message map[string]json.RawMessage
			if len(choice[field]) == 0 {
				continue
			}
			if err := json.Unmarshal(choice[field], &message); err != nil {
				return nil, err
			}
			content := bytes.TrimSpace(message["content"])
			if len(content) == 0 || content[0] != '[' {
				continue
			}
			var chunks []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"thinking"`
			}
			if err := json.Unmarshal(content, &chunks); err != nil {
				return nil, err
			}
			var text, thinking strings.Builder
			for _, chunk := range chunks {
				switch chunk.Type {
				case "text":
					text.WriteString(chunk.Text)
				case "thinking":
					for _, nested := range chunk.Thinking {
						if nested.Type != "text" {
							return nil, errors.New("unsupported Mistral thinking chunk")
						}
						thinking.WriteString(nested.Text)
					}
				default:
					return nil, errors.New("unsupported Mistral response content chunk")
				}
			}
			message["content"], _ = json.Marshal(text.String())
			if thinking.Len() > 0 {
				var existing string
				if len(message["reasoning_content"]) > 0 {
					if err := json.Unmarshal(message["reasoning_content"], &existing); err != nil {
						return nil, err
					}
				}
				message["reasoning_content"], _ = json.Marshal(existing + thinking.String())
			}
			choice[field], _ = json.Marshal(message)
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	body["choices"], _ = json.Marshal(choices)
	return json.Marshal(body)
}

// thinkingBody lazily adapts one JSON body or one SSE event at a time. It uses
// no goroutine and closes the original body exactly once, including on errors.
type thinkingBody struct {
	upstream  io.ReadCloser
	lines     *commonsse.LineReader
	pending   []byte
	done      bool
	closeOnce sync.Once
	closeErr  error
}

const maxThinkingFrame = 16 << 20

// newThinkingBody wraps upstream in a streaming or ordinary response adapter.
func newThinkingBody(upstream io.ReadCloser, stream bool) *thinkingBody {
	body := &thinkingBody{upstream: upstream}
	if stream {
		body.lines = commonsse.NewLineReader(upstream, 0)
	}
	return body
}

// Close closes the original response and returns its close error idempotently.
func (b *thinkingBody) Close() error {
	b.closeOnce.Do(func() { b.closeErr = b.upstream.Close() })
	return b.closeErr
}

// Read reads normalized response bytes into p with bounded per-event buffering.
func (b *thinkingBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(b.pending) == 0 {
		if b.done {
			return 0, io.EOF
		}
		var raw []byte
		var err error
		if b.lines == nil {
			b.done = true
			raw, err = readThinkingFrame(b.upstream, maxThinkingFrame)
			if err == nil {
				b.pending, err = normalizeThinking(raw)
			}
		} else {
			b.pending, err = b.nextEvent()
		}
		if err != nil {
			b.done = true
			_ = b.Close()
			return 0, err
		}
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

// readThinkingFrame reads at most limit bytes and rejects an oversized frame.
func readThinkingFrame(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("Mistral response frame exceeds size limit")
	}
	return raw, nil
}

// nextEvent joins multi-line SSE data and preserves event/comment/id fields.
// A complete final event without a trailing blank line is accepted at EOF.
func (b *thinkingBody) nextEvent() ([]byte, error) {
	var data, other [][]byte
	size := 0
	for {
		line, err := b.lines.Next()
		if errors.Is(err, io.EOF) {
			b.done = true
			break
		}
		if err != nil {
			return nil, err
		}
		if line.Kind == commonsse.LineKindBlank {
			break
		}
		raw := line.Small
		if line.Oversized {
			raw, err = readThinkingFrame(line.Large, int64(maxThinkingFrame-size))
			if err != nil {
				return nil, err
			}
		} else if line.Kind == commonsse.LineKindData {
			raw = bytes.TrimPrefix(raw, []byte("data:"))
			raw = bytes.TrimPrefix(raw, []byte(" "))
		}
		size += len(raw) + 1
		if size > maxThinkingFrame {
			return nil, errors.New("Mistral SSE event exceeds size limit")
		}
		if line.Kind == commonsse.LineKindData {
			data = append(data, raw)
		} else {
			other = append(other, raw)
		}
	}
	if len(data) == 0 && len(other) == 0 {
		return nil, nil
	}
	var out bytes.Buffer
	for _, line := range other {
		out.Write(line)
		out.WriteByte('\n')
	}
	if len(data) > 0 {
		raw := bytes.Join(data, []byte("\n"))
		if len(bytes.TrimSpace(raw)) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
			var err error
			raw, err = normalizeThinking(raw)
			if err != nil {
				return nil, err
			}
		}
		out.WriteString("data: ")
		out.Write(raw)
		out.WriteByte('\n')
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// init advertises the documented effort toggles for the hybrid models rather
// than copying an unrelated provider's effort vocabulary. No request allowlist
// is introduced: explicit values remain subject to upstream validation.
func init() {
	for _, id := range []string{"mistral-small-latest", "mistral-small-2603", "mistral-medium-latest", "mistral-medium-2604", "mistral-medium-3-5"} {
		cfg := ModelRatios[id].Clone()
		cfg.SupportedReasoningEfforts = []string{"none", "high"}
		if !slices.Contains(cfg.SupportedSamplingParameters, "reasoning_effort") {
			cfg.SupportedSamplingParameters = append(cfg.SupportedSamplingParameters, "reasoning_effort")
		}
		ModelRatios[id] = cfg
	}
}
