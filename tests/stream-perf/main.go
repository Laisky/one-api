// Command stream-perf provides a loopback mock provider and a validating HTTP load client.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type options struct {
	mode, address, target, model, output, id, fault                string
	concurrency, requests, chunks, chunkBytes, paceMS, cancelAfter int
	rate                                                         float64
	timeout                                                      time.Duration
}

type sample struct {
	Index            int     `json:"index"`
	QueueMS          float64 `json:"queue_ms"`
	TTFTMS           float64 `json:"ttft_ms"`
	DoneMS           float64 `json:"done_ms"`
	TotalMS          float64 `json:"total_ms"`
	ScheduledTotalMS float64 `json:"scheduled_total_ms"`
	Error            string  `json:"error,omitempty"`
}

type distribution struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

type result struct {
	SchemaVersion          int          `json:"schema_version"`
	GoVersion              string       `json:"go_version"`
	Concurrency            int          `json:"concurrency"`
	Offered                int          `json:"offered"`
	Completed              int          `json:"completed"`
	Failed                 int          `json:"failed"`
	Dropped                int          `json:"dropped"`
	Seconds                float64      `json:"seconds"`
	SuccessfulRPS          float64      `json:"successful_rps"`
	ContentChunksPerSecond float64      `json:"content_chunks_per_second"`
	TTFT                   distribution `json:"ttft_ms"`
	Done                   distribution `json:"done_ms"`
	Total                  distribution `json:"total_ms"`
	ScheduledTotal         distribution `json:"scheduled_total_ms"`
	Samples                []sample     `json:"samples"`
}

// main parses command-line arguments, runs the requested mode, and reports failures without credentials.
func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "load", "load or mock")
	flag.StringVar(&o.address, "listen", "127.0.0.1:18080", "mock listen address (loopback only)")
	flag.StringVar(&o.target, "url", "http://127.0.0.1:3000/v1/chat/completions", "gateway endpoint (loopback only)")
	flag.StringVar(&o.model, "model", "gpt-4o-mini", "configured mock channel model")
	flag.StringVar(&o.output, "output", "-", "JSON result path or -")
	flag.StringVar(&o.id, "id", "trial", "unique trial identifier")
	flag.StringVar(&o.fault, "fault", "", "mock fault for validator self-tests")
	flag.IntVar(&o.concurrency, "concurrency", 8, "maximum in-flight requests")
	flag.IntVar(&o.requests, "requests", 128, "offered request count")
	flag.IntVar(&o.chunks, "chunks", 256, "content chunks per response")
	flag.IntVar(&o.chunkBytes, "chunk-bytes", 96, "UTF-8 bytes per content chunk")
	flag.IntVar(&o.paceMS, "pace-ms", 0, "delay before each content chunk")
	flag.IntVar(&o.cancelAfter, "cancel-after", 0, "close client response after this many content chunks")
	flag.Float64Var(&o.rate, "rate", 0, "open-loop offered requests/s; zero uses closed-loop workers")
	flag.DurationVar(&o.timeout, "timeout", 30*time.Second, "per-request deadline")
	flag.Parse()
	if err := execute(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// execute validates bounded workload options and runs the mock server or load experiment.
func execute(o options) error {
	if o.mode == "mock" {
		return serveMock(o.address)
	}
	if o.mode != "load" {
		return fmt.Errorf("mode must be load or mock")
	}
	if o.concurrency < 1 || o.concurrency > 4096 || o.requests < 1 || o.requests > 1000000 || o.rate < 0 || o.rate > 100000 || o.timeout <= 0 || o.cancelAfter < 0 || o.cancelAfter > o.chunks {
		return fmt.Errorf("invalid concurrency, requests, rate or timeout")
	}
	if len(o.id) > 40 || strings.ContainsAny(o.id, "\r\n") {
		return fmt.Errorf("invalid trial id")
	}
	if err := validateSpec(spec{ID: o.id, Chunks: o.chunks, Bytes: o.chunkBytes, PaceMS: o.paceMS, Fault: o.fault}); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, o.target, nil)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}
	ip := net.ParseIP(req.URL.Hostname())
	if req.URL.Scheme != "http" || ip == nil || !ip.IsLoopback() || req.URL.User != nil {
		return fmt.Errorf("only literal loopback HTTP targets are permitted")
	}
	key := os.Getenv("STREAM_PERF_TOKEN")
	if key == "" {
		return fmt.Errorf("STREAM_PERF_TOKEN is required")
	}
	transport := &http.Transport{Proxy: nil, MaxIdleConns: o.concurrency * 2, MaxIdleConnsPerHost: o.concurrency, MaxConnsPerHost: o.concurrency, DisableCompression: true, IdleConnTimeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: o.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	r := runLoad(o, client, key)
	var out io.Writer = os.Stdout
	if o.output != "-" {
		f, err := os.Create(o.output)
		if err != nil {
			return fmt.Errorf("create result: %w", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "close result:", err)
			}
		}()
		out = f
	}
	if err := json.NewEncoder(out).Encode(r); err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	if r.Failed+r.Dropped > 0 {
		return fmt.Errorf("workload failed: %d requests failed and %d were dropped", r.Failed, r.Dropped)
	}
	return nil
}

type job struct {
	index     int
	scheduled time.Time
}

// runLoad drives bounded workers, counts offered-load drops, and returns every completed observation.
func runLoad(o options, client *http.Client, key string) result {
	r := result{SchemaVersion: 1, GoVersion: runtime.Version(), Concurrency: o.concurrency, Offered: o.requests}
	jobs := make(chan job) // An unbuffered queue exposes overload instead of hiding unbounded wait time.
	results := make(chan sample, o.concurrency)
	var wg sync.WaitGroup
	var drops atomic.Int64
	start := time.Now()
	for i := 0; i < o.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- requestOnce(o, client, key, j)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := 0; i < o.requests; i++ {
			scheduled := time.Now()
			if o.rate > 0 {
				scheduled = start.Add(time.Duration(float64(i) / o.rate * float64(time.Second)))
				if delay := time.Until(scheduled); delay > 0 {
					time.Sleep(delay)
				}
				select {
				case jobs <- job{i, scheduled}:
				default:
					drops.Add(1)
				}
			} else {
				jobs <- job{i, scheduled}
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	for s := range results {
		r.Samples = append(r.Samples, s)
	}
	r.Seconds = time.Since(start).Seconds()
	r.Dropped = int(drops.Load())
	var ttft, done, total, scheduled []float64
	for _, s := range r.Samples {
		if s.Error != "" {
			r.Failed++
			continue
		}
		r.Completed++
		ttft = append(ttft, s.TTFTMS)
		done = append(done, s.DoneMS)
		total = append(total, s.TotalMS)
		scheduled = append(scheduled, s.ScheduledTotalMS)
	}
	r.SuccessfulRPS = float64(r.Completed) / r.Seconds
	r.ContentChunksPerSecond = r.SuccessfulRPS * float64(o.chunks)
	if o.cancelAfter > 0 {
		r.ContentChunksPerSecond = r.SuccessfulRPS * float64(o.cancelAfter)
	}
	r.TTFT = quantiles(ttft)
	r.Done = quantiles(done)
	r.Total = quantiles(total)
	r.ScheduledTotal = quantiles(scheduled)
	sort.Slice(r.Samples, func(i, j int) bool { return r.Samples[i].Index < r.Samples[j].Index })
	return r
}

// requestOnce sends a unique fixture request and measures verified content, termination and complete HTTP-body delivery.
func requestOnce(o options, client *http.Client, key string, j job) (s sample) {
	s.Index = j.index
	start := time.Now()
	s.QueueMS = float64(start.Sub(j.scheduled)) / float64(time.Millisecond)
	defer func() {
		s.TotalMS = float64(time.Since(start)) / float64(time.Millisecond)
		s.ScheduledTotalMS = float64(time.Since(j.scheduled)) / float64(time.Millisecond)
	}()
	fixture := spec{ID: o.id + "-" + strconv.Itoa(j.index), Chunks: o.chunks, Bytes: o.chunkBytes, PaceMS: o.paceMS, Fault: o.fault}
	fixtureJSON, err := json.Marshal(fixture)
	if err != nil {
		s.Error = "encode fixture"
		return
	}
	body, err := json.Marshal(map[string]any{"model": o.model, "stream": true, "stream_options": map[string]bool{"include_usage": true}, "max_tokens": o.chunks * 32, "messages": []map[string]string{{"role": "user", "content": string(fixtureJSON)}}})
	if err != nil {
		s.Error = "encode request"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.target, bytes.NewReader(body))
	if err != nil {
		s.Error = "construct request"
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		s.Error = "HTTP request: " + err.Error()
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && s.Error == "" {
			s.Error = "close response body: " + err.Error()
		}
	}()
	if resp.StatusCode != http.StatusOK {
		s.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		return
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		s.Error = "not an SSE response"
		return
	}
	if err := validateStream(resp.Body, fixture, o.cancelAfter, start, &s); err != nil {
		s.Error = err.Error()
	}
	return
}

// quantiles returns nearest-rank millisecond quantiles; empty samples have zero values and are never presented as success.
func quantiles(values []float64) distribution {
	if len(values) == 0 {
		return distribution{}
	}
	sort.Float64s(values)
	at := func(percent int) float64 { i := (len(values)*percent+99)/100 - 1; return values[i] }
	return distribution{P50: at(50), P95: at(95), P99: at(99), Max: at(100)}
}
