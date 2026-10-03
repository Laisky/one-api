package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
)

// readModernMCPEventStream incrementally reads one bounded request-scoped SSE response.
// It returns the correlated terminal envelope immediately, or an error on truncation,
// cancellation, invalid server requests, size overflow, or failed progress delivery.
func readModernMCPEventStream(ctx context.Context, body io.Reader, expectedID string, options CallToolRequestOptions) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: maxMCPResponseBodyBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), int(maxMCPResponseBodyBytes)+1)
	scanner.Split(newMCPEventLineSplitter())
	data := make([]byte, 0)
	firstLine := true
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "read MCP event stream")
		}
		if limited.N == 0 {
			return nil, errors.New("MCP SSE response exceeds body size limit")
		}
		line := scanner.Bytes()
		if firstLine {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			firstLine = false
		}
		if len(line) == 0 {
			if len(data) == 0 {
				continue
			}
			terminal, err := consumeModernMCPEvent(ctx, data[:len(data)-1], expectedID, options)
			if err != nil {
				return nil, err
			}
			if terminal != nil {
				return terminal, nil
			}
			data = data[:0]
			continue
		}
		field, value, found := bytes.Cut(line, []byte(":"))
		if string(field) != "data" {
			continue // Comments, event names, retry and event IDs are not MCP payloads.
		}
		if found {
			value = bytes.TrimPrefix(value, []byte(" "))
		}
		data = append(data, value...)
		data = append(data, '\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "scan MCP event stream")
	}
	if limited.N == 0 {
		return nil, errors.New("MCP SSE response exceeds body size limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "read MCP event stream")
	}
	return nil, errors.Errorf("mcp SSE response has no event for request id %q before EOF", expectedID)
}

// newMCPEventLineSplitter returns a per-stream SSE line splitter for LF, CRLF and bare CR.
// CR terminates a line immediately; a following LF is consumed without creating an extra empty line.
func newMCPEventLineSplitter() bufio.SplitFunc {
	skipLF := false
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		offset := 0
		if skipLF {
			if len(data) == 0 {
				return 0, nil, nil
			}
			skipLF = false
			if data[0] == '\n' {
				offset = 1
			}
		}
		for index := offset; index < len(data); index++ {
			switch data[index] {
			case '\n':
				return index + 1, data[offset:index], nil
			case '\r':
				skipLF = true
				return index + 1, data[offset:index], nil
			}
		}
		if atEOF && len(data) > offset {
			return len(data), data[offset:], nil
		}
		return offset, nil, nil
	}
}

// consumeModernMCPEvent validates one SSE data event and delivers only requested progress.
// Other response IDs and unsupported notifications are ignored; server-initiated requests are errors.
func consumeModernMCPEvent(ctx context.Context, data []byte, expectedID string, options CallToolRequestOptions) ([]byte, error) {
	var event struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  map[string]any  `json:"params"`
		Error   json.RawMessage `json:"error"`
	}
	if err := DecodeJSON(data, &event); err != nil {
		return nil, errors.Wrap(err, "decode MCP SSE event")
	}
	if event.JSONRPC != "2.0" {
		return nil, errors.New("MCP SSE event jsonrpc must be 2.0")
	}
	if event.Method != "" {
		if len(event.ID) != 0 {
			return nil, errors.New("modern MCP does not permit server-initiated JSON-RPC requests")
		}
		if options.OnNotification != nil && event.Method == "notifications/progress" && matchingMCPProgressToken(options.Meta["progressToken"], event.Params["progressToken"]) {
			// Copy before invoking user code: data is reused by the stream parser.
			if err := options.OnNotification(ctx, append(json.RawMessage(nil), data...)); err != nil {
				return nil, errors.Wrap(err, "deliver MCP progress notification")
			}
		}
		return nil, nil
	}
	if validateMCPResponseID(event.ID, expectedID) != nil && !(len(event.Error) > 0 && isUncorrelatedErrorID(event.ID)) {
		return nil, nil
	}
	if _, err := parseMCPResponseEnvelope(data, expectedID); err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

// matchingMCPProgressToken matches explicitly requested string or exact numeric tokens without float64 coercion.
// Missing tokens never opt a caller into notifications; adjacent large integer tokens remain distinct.
func matchingMCPProgressToken(requested, returned any) bool {
	if requested == nil || returned == nil {
		return false
	}
	left, leftErr := json.Marshal(requested)
	right, rightErr := json.Marshal(returned)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
