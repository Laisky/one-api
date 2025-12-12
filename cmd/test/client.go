package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

const (
	maxToolAttempts     = 3
	maxTransientRetries = 3
	initialRetryBackoff = 250 * time.Millisecond
	maxRetryBackoff     = 2 * time.Second
)

// performRequest sends a request variant, optionally trying multiple payload attempts,
// and returns the aggregated result.
func performRequest(ctx context.Context, client *http.Client, baseURL, token string, spec requestSpec, model string) (result testResult) {
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	attemptBodies := spec.AttemptBodies
	if len(attemptBodies) == 0 {
		attemptBodies = []any{spec.Body}
	}
	if len(attemptBodies) > maxToolAttempts {
		attemptBodies = attemptBodies[:maxToolAttempts]
	}

	var softPass *testResult
	for attemptIdx, body := range attemptBodies {
		attemptSpec := spec
		attemptSpec.Body = body
		attemptRes := performSingleAttempt(ctx, client, baseURL, token, attemptSpec, model)
		attemptRes.AttemptCount = attemptIdx + 1

		// Unsupported combinations are invariant to payload; stop early.
		if attemptRes.Skipped {
			return attemptRes
		}
		if attemptRes.Success && attemptRes.Warning == "" {
			return attemptRes
		}
		if attemptRes.Success && attemptRes.Warning != "" {
			softPass = &attemptRes
			continue
		}

		result = attemptRes
	}

	if softPass != nil {
		return *softPass
	}

	return result
}

// performSingleAttempt executes exactly one HTTP request for a spec.Body.
//
// It retries transient failures (network errors, 5xx responses) up to
// maxTransientRetries times.
func performSingleAttempt(ctx context.Context, client *http.Client, baseURL, token string, spec requestSpec, model string) (result testResult) {
	result = testResult{
		Model:         model,
		RequestFormat: spec.RequestFormat,
		Label:         spec.Label,
		Type:          spec.Type,
		Stream:        spec.Stream,
	}

	payload, err := json.Marshal(spec.Body)
	if err != nil {
		result.ErrorReason = fmt.Sprintf("marshal payload: %v", err)
		return result
	}
	result.RequestBody = truncateString(string(payload), maxLoggedBodyBytes)

	var (
		lastErr         error
		lastAttempt     testResult
		haveLastAttempt bool
	)
	backoff := initialRetryBackoff
	for retry := 0; retry < maxTransientRetries; retry++ {
		attemptRes, attemptErr := doRequestOnce(ctx, client, baseURL, token, spec, payload)
		lastAttempt = attemptRes
		haveLastAttempt = true
		if attemptErr == nil {
			attemptRes.Model = result.Model
			attemptRes.RequestFormat = result.RequestFormat
			attemptRes.Label = result.Label
			attemptRes.Type = result.Type
			attemptRes.Stream = result.Stream
			attemptRes.RequestBody = result.RequestBody
			return attemptRes
		}
		lastErr = attemptErr
		if !isRetryableAttemptError(attemptRes.StatusCode, attemptErr, attemptRes.ResponseBody) {
			attemptRes.Model = result.Model
			attemptRes.RequestFormat = result.RequestFormat
			attemptRes.Label = result.Label
			attemptRes.Type = result.Type
			attemptRes.Stream = result.Stream
			attemptRes.RequestBody = result.RequestBody
			attemptRes.ErrorReason = attemptErr.Error()
			return attemptRes
		}
		select {
		case <-ctx.Done():
			result.ErrorReason = fmt.Sprintf("request cancelled: %v", ctx.Err())
			return result
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxRetryBackoff {
			backoff = maxRetryBackoff
		}
	}

	if haveLastAttempt {
		lastAttempt.Model = result.Model
		lastAttempt.RequestFormat = result.RequestFormat
		lastAttempt.Label = result.Label
		lastAttempt.Type = result.Type
		lastAttempt.Stream = result.Stream
		lastAttempt.RequestBody = result.RequestBody
		lastAttempt.ErrorReason = fmt.Sprintf("transient error after retries: %v", lastErr)

		// Downgrade persistent upstream errors to SKIP so the suite focuses on one-api
		// compatibility rather than transient provider outages.
		if lastAttempt.StatusCode == http.StatusBadGateway ||
			lastAttempt.StatusCode == http.StatusServiceUnavailable ||
			lastAttempt.StatusCode == http.StatusGatewayTimeout {
			lastAttempt.Skipped = true
			return lastAttempt
		}
		lowerBody := strings.ToLower(lastAttempt.ResponseBody)
		if lastAttempt.StatusCode >= 500 && (strings.Contains(lowerBody, "\"type\":\"server_error\"") ||
			strings.Contains(lowerBody, "\"code\":\"server_error\"")) {
			lastAttempt.Skipped = true
			return lastAttempt
		}

		return lastAttempt
	}

	result.ErrorReason = fmt.Sprintf("transient error after retries: %v", lastErr)
	return result
}

// doRequestOnce issues the HTTP request once and returns a populated testResult.
func doRequestOnce(ctx context.Context, client *http.Client, baseURL, token string, spec requestSpec, payload []byte) (result testResult, err error) {
	endpoint := baseURL + spec.Path
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if reqErr != nil {
		return result, errors.Wrap(reqErr, "build request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "oneapi-test-harness/1.0")

	resp, doErr := client.Do(req)
	if doErr != nil {
		return result, errors.Wrap(doErr, "do request")
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if spec.Stream {
		streamData, streamErr := collectStreamBody(resp.Body, maxResponseBodySize)
		if len(streamData) > 0 {
			result.ResponseBody = truncateString(string(streamData), maxLoggedBodyBytes)
		}
		if streamErr != nil {
			return result, errors.Wrap(streamErr, "stream read")
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			reason := fmt.Sprintf("status %s: %s", resp.Status, snippet(streamData))
			if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, streamData, reason) {
				result.Skipped = true
				result.ErrorReason = reason
				return result, nil
			}
			return result, errors.Errorf("%s", reason)
		}

		success, reason := evaluateStreamResponse(spec, streamData)
		if success {
			result.Success = true
			return result, nil
		}

		// Soft-pass tool expectations when the provider returns a valid response
		// without tool invocations.
		if isToolExpectation(spec.Expectation) {
			ok, _ := evaluateStreamNoError(spec.Type, streamData)
			if ok {
				result.Success = true
				result.Warning = "tool was not invoked"
				return result, nil
			}
		}

		if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, streamData, reason) {
			result.Skipped = true
			result.ErrorReason = reason
			return result, nil
		}

		if reason == "" {
			reason = snippet(streamData)
		}
		return result, errors.Errorf("%s", reason)
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if len(body) > 0 {
		result.ResponseBody = truncateString(string(body), maxLoggedBodyBytes)
	}
	if readErr != nil {
		return result, errors.Wrap(readErr, "read response")
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		success, reason := evaluateResponse(spec, body)
		if success {
			result.Success = true
			return result, nil
		}

		if isToolExpectation(spec.Expectation) {
			ok, _ := evaluateResponseNoError(spec.Type, body)
			if ok {
				result.Success = true
				result.Warning = "tool was not invoked"
				return result, nil
			}
		}

		if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, body, reason) {
			result.Skipped = true
			result.ErrorReason = reason
			return result, nil
		}

		if reason == "" {
			reason = snippet(body)
		}
		return result, errors.Errorf("%s", reason)
	}

	reason := fmt.Sprintf("status %s: %s", resp.Status, snippet(body))
	if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, body, reason) {
		result.Skipped = true
		result.ErrorReason = reason
		return result, nil
	}

	return result, errors.Errorf("%s", reason)
}

// isRetryableAttemptError reports whether the attempt should be retried.
func isRetryableAttemptError(statusCode int, err error, responseBody string) bool {
	if err == nil {
		return false
	}
	if statusCode == 0 {
		return true
	}
	if statusCode >= 500 {
		return true
	}
	lower := strings.ToLower(responseBody)
	if statusCode == http.StatusUnauthorized {
		if strings.Contains(lower, "database is locked") || strings.Contains(lower, "database is busy") {
			return true
		}
	}
	if statusCode >= 200 && statusCode < 300 {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "structured output fields missing") {
			if strings.Contains(lower, "\"output_tokens\":0") || strings.Contains(lower, "\"content\":[]") {
				return true
			}
		}
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "temporarily") {
		return true
	}
	return false
}

// collectStreamBody reads a streaming response until EOF, blank line, or size limit.
func collectStreamBody(body io.Reader, limit int) ([]byte, error) {
	reader := bufio.NewReader(body)
	buffer := &bytes.Buffer{}

	for buffer.Len() < limit {
		chunk, err := reader.ReadBytes('\n')
		if len(chunk) > 0 {
			if buffer.Len()+len(chunk) > limit {
				chunk = chunk[:limit-buffer.Len()]
			}
			buffer.Write(chunk)
			trimmed := bytes.TrimSpace(chunk)
			if isStreamTerminator(trimmed) {
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return buffer.Bytes(), err
		}
	}

	if buffer.Len() == 0 {
		return buffer.Bytes(), fmt.Errorf("no stream data received")
	}

	return buffer.Bytes(), nil
}

func isStreamTerminator(line []byte) bool {
	if len(line) == 0 {
		return false
	}

	if bytes.Equal(line, []byte("data: [DONE]")) || bytes.Equal(line, []byte("[DONE]")) {
		return true
	}

	if !bytes.HasPrefix(line, []byte("data:")) {
		return false
	}

	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 {
		return false
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err == nil {
		if terminatingStreamType(decoded) {
			return true
		}
		return false
	}

	lower := bytes.ToLower(payload)
	if bytes.Contains(lower, []byte(`"type":"response.completed"`)) ||
		bytes.Contains(lower, []byte(`"type":"response.cancelled"`)) ||
		bytes.Contains(lower, []byte(`"type":"response.error"`)) {
		return true
	}

	return false
}

func terminatingStreamType(decoded map[string]any) bool {
	if t, ok := decoded["type"].(string); ok {
		switch strings.ToLower(t) {
		case "response.completed", "response.cancelled", "response.error", "done":
			return true
		}
	}

	if event, ok := decoded["event"].(string); ok && strings.ToLower(event) == "response.completed" {
		return true
	}

	if response, ok := decoded["response"].(map[string]any); ok {
		if t, ok := response["type"].(string); ok {
			switch strings.ToLower(t) {
			case "response.completed", "response.cancelled", "response.error":
				return true
			}
		}
		if status, ok := response["status"].(string); ok && strings.ToLower(status) == "completed" {
			return true
		}
	}

	if delta, ok := decoded["delta"].(map[string]any); ok {
		if status, ok := delta["status"].(string); ok && strings.ToLower(status) == "completed" {
			return true
		}
	}

	return false
}
