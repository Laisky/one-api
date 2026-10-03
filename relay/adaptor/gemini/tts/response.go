package tts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Receipt contains validated upstream work, including work accepted before a disconnect.
// Missing input usage remains unpriced; only missing audio usage is estimated from received PCM.
type Receipt struct {
	PromptTokens    int
	OutputTokens    int
	CachedTokens    int
	AudioBytes      int
	Accepted        bool
	UsageComplete   bool
	EstimatedOutput bool
	// Truncated marks a valid terminal MAX_TOKENS response, not a failed receipt.
	Truncated bool
	// EncodingFailed requests a customer refund for a gateway codec failure before
	// delivery. Accepted remains true to prevent replay of paid upstream work.
	// Caller cancellation and downstream write errors never set this flag.
	EncodingFailed bool
}

// usageSnapshot contains cumulative token counters, never per-chunk increments.
type usageSnapshot struct {
	Prompt *int `json:"promptTokenCount"`
	Output *int `json:"candidatesTokenCount"`
	Cached *int `json:"cachedContentTokenCount"`
}

// envelope models only the native fields needed for a speech receipt.
type envelope struct {
	Error          json.RawMessage `json:"error"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Candidates []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text       string `json:"text"`
				InlineData *struct {
					MIME string `json:"mimeType"`
					Data string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Usage *usageSnapshot `json:"usageMetadata"`
}

// decoder accumulates bounded audio and authoritative cumulative usage for one request.
type decoder struct {
	plan       *Plan
	receipt    Receipt
	havePrompt bool
	haveOutput bool
	terminal   bool
	finalUsage bool
	deliver    func([]byte) error
}

// updateUsage validates one cumulative snapshot before replacing previous counters.
func (d *decoder) updateUsage(u *usageSnapshot) error {
	if u == nil {
		return nil
	}
	prompt, output, cached := d.receipt.PromptTokens, d.receipt.OutputTokens, d.receipt.CachedTokens
	if u.Prompt != nil {
		prompt = *u.Prompt
	}
	if u.Output != nil {
		output = *u.Output
	}
	if u.Cached != nil {
		cached = *u.Cached
	}
	if prompt < 0 || prompt > MaxInputTokens || output < 0 || output > d.plan.OutputLimit || cached < 0 || cached > prompt || cached < d.receipt.CachedTokens || prompt < d.receipt.PromptTokens || output < d.receipt.OutputTokens {
		return errors.New("invalid or regressing Gemini speech usage")
	}
	d.receipt.PromptTokens, d.receipt.OutputTokens, d.receipt.CachedTokens = prompt, output, cached
	d.havePrompt = d.havePrompt || u.Prompt != nil
	d.haveOutput = d.haveOutput || u.Output != nil
	return nil
}

// consume validates one native event and accounts audio before attempting downstream delivery.
func (d *decoder) consume(body []byte) error {
	var value envelope
	if err := json.Unmarshal(body, &value); err != nil {
		return errors.Wrap(err, "decode Gemini speech receipt")
	}
	if len(bytes.TrimSpace(body)) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return errors.New("empty Gemini speech receipt")
	}
	if len(value.Error) > 0 && !bytes.Equal(value.Error, []byte("null")) {
		return errors.New("Gemini rejected speech generation")
	}
	if value.PromptFeedback.BlockReason != "" && value.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED" {
		return errors.New("Gemini blocked speech generation")
	}
	if len(value.Candidates) > 1 {
		return errors.New("speech requires exactly one upstream candidate")
	}
	for _, candidate := range value.Candidates {
		if candidate.Index != 0 {
			return errors.New("unexpected speech candidate index")
		}
		switch candidate.FinishReason {
		case "", "FINISH_REASON_UNSPECIFIED", "STOP", "MAX_TOKENS":
		default:
			return errors.New("Gemini did not accept speech generation")
		}
		if len(candidate.Content.Parts) > 4096 {
			return errors.New("too many speech response parts")
		}
	}
	usageErr := d.updateUsage(value.Usage)
	for _, candidate := range value.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				return errors.New("speech model returned text instead of audio")
			}
			if part.InlineData == nil {
				continue
			}
			if d.terminal {
				return errors.New("audio received after terminal speech event")
			}
			raw := part.InlineData
			if len(raw.Data) > ((MaxAudioBytes+4098)/3)*4 {
				return errors.New("speech audio part exceeds size limit")
			}
			data, err := base64.StdEncoding.Strict().DecodeString(raw.Data)
			if err != nil {
				return errors.New("invalid base64 speech audio")
			}
			pcm, err := decodeAudio(data, raw.MIME)
			if err != nil {
				return err
			}
			if len(pcm) > MaxAudioBytes-d.receipt.AudioBytes || len(pcm) > d.plan.OutputLimit*1920-d.receipt.AudioBytes {
				return errors.New("speech audio exceeds requested output budget")
			}
			d.receipt.AudioBytes += len(pcm)
			d.receipt.Accepted = true
			if err := d.deliver(pcm); err != nil {
				return errors.Wrap(err, "deliver speech audio")
			}
		}
		if candidate.FinishReason == "STOP" || candidate.FinishReason == "MAX_TOKENS" {
			// A token limit terminates valid audio; keep it for buffered codecs and
			// continue reading any trailing authoritative usage event.
			d.terminal = true
			d.receipt.Truncated = candidate.FinishReason == "MAX_TOKENS"
		}
	}
	if usageErr != nil {
		return usageErr
	}
	if d.terminal && value.Usage != nil && value.Usage.Prompt != nil && value.Usage.Output != nil {
		d.finalUsage = true
	}
	return nil
}

// result returns authoritative final usage or an explicitly incomplete estimated receipt.
func (d *decoder) result(completed bool) Receipt {
	out := d.receipt
	out.UsageComplete = completed && d.terminal && d.finalUsage && d.havePrompt && d.haveOutput && out.PromptTokens > 0 && out.OutputTokens > 0
	if !out.UsageComplete {
		// One audio token per 1,920 PCM bytes at 24 kHz, PCM16, 25 tokens/second.
		out.OutputTokens = max(out.OutputTokens, (out.AudioBytes+1919)/1920)
		out.EstimatedOutput = out.Accepted
	}
	return out
}

// readEvents consumes bounded native SSE frames, including multiline data and comments.
func readEvents(body io.Reader, consume func([]byte) error) error {
	limited := &io.LimitedReader{R: body, N: maxWireBytes + 1}
	scan := bufio.NewScanner(limited)
	scan.Buffer(make([]byte, 64<<10), maxWireBytes+1)
	var event strings.Builder
	flush := func() error {
		if event.Len() == 0 {
			return nil
		}
		data := strings.TrimSuffix(event.String(), "\n")
		event.Reset()
		if data == "[DONE]" {
			return errors.New("unexpected OpenAI terminator in Gemini speech stream")
		}
		return consume([]byte(data))
	}
	for scan.Scan() {
		if limited.N <= 0 {
			return errors.New("speech stream exceeds 64 MiB")
		}
		line := strings.TrimSuffix(scan.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line[5:], " ")
			if event.Len()+len(data)+1 > maxWireBytes {
				return errors.New("speech event exceeds size limit")
			}
			event.WriteString(data)
			event.WriteByte('\n')
		}
	}
	if err := scan.Err(); err != nil {
		return errors.Wrap(err, "read Gemini speech stream")
	}
	if limited.N <= 0 {
		return errors.New("speech stream exceeds 64 MiB")
	}
	return flush()
}

// Forward converts a native response into standard binary audio or speech SSE events.
// PCM at speed 1 is delivered incrementally. Container/codec conversions are bounded
// and buffered. The returned receipt survives upstream errors and downstream disconnects.
func (p *Plan) Forward(ctx context.Context, resp *http.Response, w http.ResponseWriter, onReceipt func(Receipt)) (Receipt, error) {
	emitter := &speechEmitter{writer: w, plan: p}
	var pcm bytes.Buffer
	incremental := p.Stream && p.Format == "pcm" && p.Speed == 1
	d := &decoder{plan: p}
	d.deliver = func(data []byte) error {
		if onReceipt != nil {
			onReceipt(d.result(false))
		}
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		if incremental {
			return emitter.audio(data)
		}
		_, err := pcm.Write(data)
		return errors.WithStack(err)
	}
	var err error
	contentType, _, parseErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if parseErr != nil {
		return Receipt{}, errors.New("invalid Gemini speech response Content-Type")
	}
	if p.Stream {
		if contentType != "text/event-stream" {
			return Receipt{}, errors.New("Gemini speech streaming response is not SSE")
		}
		err = readEvents(resp.Body, d.consume)
	} else {
		if contentType != "application/json" {
			return Receipt{}, errors.New("Gemini speech response is not JSON")
		}
		var body []byte
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxWireBytes+1))
		if err == nil && len(body) > maxWireBytes {
			err = errors.New("speech response exceeds 64 MiB")
		}
		if err == nil {
			err = d.consume(body)
		}
	}
	if err == nil && (!d.terminal || !d.receipt.Accepted) {
		err = errors.New("Gemini speech ended without a complete audio receipt")
	}
	result := d.result(err == nil)
	if err == nil && !incremental {
		var audio []byte
		audio, err = p.encode(ctx, pcm.Bytes())
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				// A caller disconnect does not cancel already accepted provider work.
				err = errors.WithStack(ctxErr)
			} else {
				// No audio has been emitted on this buffered path. The gateway
				// absorbs conversion failures rather than billing an unusable result.
				result.EncodingFailed = true
			}
		} else {
			if result.Truncated {
				w.Header().Set("X-Gemini-Finish-Reason", "MAX_TOKENS")
			}
			err = emitter.audio(audio)
		}
	}
	if err == nil {
		err = emitter.done(result)
	}
	if err != nil && emitter.started && p.SSE {
		// Do not expose upstream error messages, transcript, style, or voice credentials.
		if writeErr := emitter.event("error", map[string]any{"type": "error", "error": map[string]string{"code": "audio_stream_incomplete", "message": "Speech generation did not complete."}}); writeErr != nil {
			return result, errors.Wrap(writeErr, "write speech terminal error")
		}
	}
	return result, errors.WithStack(err)
}

// speechEmitter is request-local and is only used on the caller's goroutine.
type speechEmitter struct {
	writer  http.ResponseWriter
	plan    *Plan
	started bool
}

// start commits only safe response headers when actual audio is ready to deliver.
func (e *speechEmitter) start() {
	if e.started {
		return
	}
	e.started = true
	e.writer.Header().Set("Cache-Control", "no-store")
	e.writer.Header().Set("X-Content-Type-Options", "nosniff")
	e.writer.Header().Set("Content-Type", e.plan.mimeType())
	if e.plan.SSE {
		e.writer.Header().Set("Content-Type", "text/event-stream")
		e.writer.Header().Set("X-Accel-Buffering", "no")
	}
	e.writer.WriteHeader(http.StatusOK)
}

// write sends bytes and treats a short write without an error as a failed delivery.
func (e *speechEmitter) write(data []byte) error {
	e.start()
	n, err := e.writer.Write(data)
	if err != nil {
		return errors.WithStack(err)
	}
	if n != len(data) {
		return errors.WithStack(io.ErrShortWrite)
	}
	if flush, ok := e.writer.(http.Flusher); ok && e.plan.Stream {
		flush.Flush()
	}
	return nil
}

// event emits one JSON SSE record without adding OpenAI chat-completion terminators.
func (e *speechEmitter) event(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "encode speech event")
	}
	return e.write([]byte("event: " + name + "\ndata: " + string(data) + "\n\n"))
}

// audio emits bounded chunks in the requested binary or SSE representation.
func (e *speechEmitter) audio(data []byte) error {
	for len(data) > 0 {
		chunk := data[:min(len(data), 32<<10)]
		var err error
		if e.plan.SSE {
			err = e.event("speech.audio.delta", map[string]string{"type": "speech.audio.delta", "audio": base64.StdEncoding.EncodeToString(chunk), "response_format": e.plan.Format})
		} else {
			err = e.write(chunk)
		}
		if err != nil {
			return err
		}
		data = data[len(chunk):]
	}
	return nil
}

// done emits exactly one success event after a terminal native receipt and successful delivery.
func (e *speechEmitter) done(receipt Receipt) error {
	if !e.plan.SSE {
		return nil
	}
	return e.event("speech.audio.done", map[string]any{
		"type": "speech.audio.done", "usage_complete": receipt.UsageComplete, "truncated": receipt.Truncated,
		"usage": map[string]any{"input_tokens": receipt.PromptTokens, "output_tokens": receipt.OutputTokens,
			"total_tokens":        receipt.PromptTokens + receipt.OutputTokens,
			"input_token_details": map[string]int{"text_tokens": receipt.PromptTokens, "audio_tokens": 0, "cached_tokens": receipt.CachedTokens}},
	})
}
