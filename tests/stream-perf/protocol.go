package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type spec struct {
	ID     string `json:"id"`
	Chunks int    `json:"chunks"`
	Bytes  int    `json:"bytes"`
	PaceMS int    `json:"pace_ms"`
	Fault  string `json:"fault,omitempty"`
}

type counters struct{ active, started, completed, cancelled atomic.Int64 }

// validateSpec rejects unbounded fixture dimensions and unknown fault injection modes.
func validateSpec(s spec) error {
	if len(s.ID) > 64 || s.Chunks < 1 || s.Chunks > 16384 || s.Bytes < 96 || s.Bytes > 16384 || s.Chunks*s.Bytes > 64<<20 || s.PaceMS < 0 || s.PaceMS > 1000 {
		return fmt.Errorf("invalid fixture bounds")
	}
	switch s.Fault {
	case "", "missing-done", "duplicate-done", "wrong-content", "no-usage", "malformed", "error", "crlf", "fragmented":
	default:
		return fmt.Errorf("unknown fixture fault")
	}
	return nil
}

// contentChunk returns deterministic, request-specific UTF-8 content with an exact byte count.
func contentChunk(s spec, index int) string {
	prefix := fmt.Sprintf("%s/%06d:hello 世界🙂|", s.ID, index)
	return prefix + strings.Repeat("x", s.Bytes-len(prefix))
}

// serveMock runs a loopback-only, cancellable SSE provider with independent lifecycle counters.
func serveMock(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("mock must bind a literal loopback address")
	}
	key := os.Getenv("STREAM_PERF_UPSTREAM_TOKEN")
	if key == "" {
		return fmt.Errorf("STREAM_PERF_UPSTREAM_TOKEN is required")
	}
	stats := &counters{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]int64{"active": stats.active.Load(), "started": stats.started.Load(), "completed": stats.completed.Load(), "cancelled": stats.cancelled.Load()}); err != nil {
			fmt.Fprintln(os.Stderr, "write mock health:", err)
		}
	})
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+key)) != 1 {
			http.Error(w, "unauthorized fixture request", http.StatusUnauthorized)
			return
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool   `json:"stream"`
			Model  string `json:"model"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&request); err != nil || len(request.Messages) != 1 || !request.Stream {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		var fixture spec
		if err := json.Unmarshal([]byte(request.Messages[0].Content), &fixture); err != nil {
			http.Error(w, "invalid fixture spec", 400)
			return
		}
		if err := validateSpec(fixture); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		stats.started.Add(1)
		stats.active.Add(1)
		defer stats.active.Add(-1)
		if err := writeFixture(w, r, fixture, request.Model); err != nil {
			stats.cancelled.Add(1)
			return
		}
		stats.completed.Add(1)
	}
	mux.HandleFunc("/v1/chat/completions", handler)
	mux.HandleFunc("/chat/completions", handler)
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("serve mock: %w", err)
	}
	return nil
}

// writeFixture writes and flushes role, content, stop, usage and terminal frames; cancellation stops the producer.
func writeFixture(w http.ResponseWriter, r *http.Request, s spec, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("missing HTTP flusher")
	}
	ending := "\n\n"
	if s.Fault == "crlf" {
		ending = "\r\n\r\n"
	}
	write := func(payload string) error {
		frame := "data: " + payload + ending
		if s.Fault == "fragmented" {
			for i := 0; i < len(frame); i++ {
				if _, err := io.WriteString(w, frame[i:i+1]); err != nil {
					return fmt.Errorf("write fragmented frame: %w", err)
				}
				flusher.Flush()
			}
		} else {
			if _, err := io.WriteString(w, frame); err != nil {
				return fmt.Errorf("write frame: %w", err)
			}
			flusher.Flush()
		}
		return nil
	}
	emit := func(delta map[string]string, finish any) error {
		payload, err := json.Marshal(map[string]any{"id": "chatcmpl-" + s.ID, "object": "chat.completion.chunk", "created": 1700000000, "model": model, "choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}}})
		if err != nil {
			return fmt.Errorf("encode frame: %w", err)
		}
		return write(string(payload))
	}
	if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
		return fmt.Errorf("write heartbeat: %w", err)
	}
	flusher.Flush()
	if err := emit(map[string]string{"role": "assistant"}, nil); err != nil {
		return err
	}
	for i := 0; i < s.Chunks; i++ {
		if s.PaceMS > 0 {
			timer := time.NewTimer(time.Duration(s.PaceMS) * time.Millisecond)
			select {
			case <-r.Context().Done():
				timer.Stop()
				return fmt.Errorf("mock request cancelled: %w", r.Context().Err())
			case <-timer.C:
			}
		}
		if err := r.Context().Err(); err != nil {
			return fmt.Errorf("mock request cancelled: %w", err)
		}
		content := contentChunk(s, i)
		if s.Fault == "wrong-content" && i == 0 {
			content = "incorrect"
		}
		if err := emit(map[string]string{"content": content}, nil); err != nil {
			return err
		}
	}
	if s.Fault == "malformed" {
		if err := write("{invalid"); err != nil {
			return err
		}
	}
	if s.Fault == "error" {
		if err := write(`{"error":{"message":"fixture failure"}}`); err != nil {
			return err
		}
	}
	if err := emit(map[string]string{}, "stop"); err != nil {
		return err
	}
	if s.Fault != "no-usage" {
		payload, err := json.Marshal(map[string]any{"id": "chatcmpl-" + s.ID, "object": "chat.completion.chunk", "created": 1700000000, "model": model, "choices": []any{}, "usage": map[string]int{"prompt_tokens": 16, "completion_tokens": s.Chunks * 16, "total_tokens": 16 + s.Chunks*16}})
		if err != nil {
			return fmt.Errorf("encode usage: %w", err)
		}
		if err := write(string(payload)); err != nil {
			return err
		}
	}
	if s.Fault != "missing-done" {
		if err := write("[DONE]"); err != nil {
			return err
		}
	}
	if s.Fault == "duplicate-done" {
		return write("[DONE]")
	}
	return nil
}

// validateStream checks exact request-specific content and usage through EOF, excluding role and heartbeat frames from TTFT.
func validateStream(body io.Reader, spec spec, cancelAfter int, start time.Time, s *sample) error {
	return validateStreamWithClock(body, spec, cancelAfter, start, s, time.Now)
}

// validateStreamWithClock validates frames with an injectable monotonic clock for deterministic latency tests.
func validateStreamWithClock(body io.Reader, spec spec, cancelAfter int, start time.Time, s *sample, now func() time.Time) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var data []string
	eventBytes := 0
	chunks, done, stops, usages := 0, 0, 0, 0
	var lastContent time.Time
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		eventBytes = 0
		if done > 0 {
			return fmt.Errorf("frame after DONE")
		}
		if payload == "[DONE]" {
			done++
			s.DoneMS = float64(now().Sub(start)) / float64(time.Millisecond)
			return nil
		}
		var event struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Finish *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Prompt     int `json:"prompt_tokens"`
				Completion int `json:"completion_tokens"`
				Total      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("invalid SSE JSON: %w", err)
		}
		if len(event.Error) > 0 && string(event.Error) != "null" {
			return fmt.Errorf("in-band stream error")
		}
		for _, choice := range event.Choices {
			if choice.Index != 0 {
				return fmt.Errorf("unexpected choice index")
			}
			if choice.Delta.Content != "" {
				if chunks >= spec.Chunks || choice.Delta.Content != contentChunk(spec, chunks) {
					return fmt.Errorf("content mismatch at chunk %d", chunks)
				}
				observed := now()
				if chunks == 0 {
					s.TTFTMS = float64(observed.Sub(start)) / float64(time.Millisecond)
				} else {
					gap := float64(observed.Sub(lastContent)) / float64(time.Millisecond)
					s.MaxInterContentGapMS = max(s.MaxInterContentGapMS, gap)
				}
				lastContent = observed
				chunks++
			}
			if choice.Finish != nil {
				if *choice.Finish != "stop" {
					return fmt.Errorf("unexpected finish reason")
				}
				stops++
			}
		}
		if event.Usage != nil {
			usages++
			if event.Usage.Prompt != 16 || event.Usage.Completion != spec.Chunks*16 || event.Usage.Total != 16+spec.Chunks*16 {
				return fmt.Errorf("usage mismatch")
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := process(); err != nil {
				return err
			}
			if cancelAfter > 0 && chunks >= cancelAfter {
				return nil
			}
		} else if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			eventBytes += len(value)
			if len(data) >= 1024 || eventBytes > 1<<20 {
				return fmt.Errorf("too many SSE data lines")
			}
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE: %w", err)
	}
	if len(data) > 0 {
		return fmt.Errorf("unterminated SSE event")
	}
	if chunks != spec.Chunks || done != 1 || stops != 1 || usages != 1 {
		return fmt.Errorf("incomplete stream: chunks=%d/%d done=%d stop=%d usage=%d", chunks, spec.Chunks, done, stops, usages)
	}
	return nil
}
