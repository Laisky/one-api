package model

// Per-timestamp trace columns (proposal 20260905_observability-data-tiering.md,
// Phase 1 / W1.5).
//
// The `traces.timestamps` TEXT column exists because the pre-proposal write path
// mutated one timestamp at a time and therefore had to rewrite a whole JSON
// document per mutation. With W1.1-W1.3 a trace is accumulated in memory and
// written exactly once, so the document can be projected onto real columns as
// well. The columns are written IN ADDITION to the document, never instead of
// it: a pre-migration binary reading a row this binary wrote still finds a
// complete `timestamps` value, which is the compatibility rule the project
// applies to every additive storage change.

// overlayTimestampColumns copies populated per-timestamp columns over the parsed
// JSON document so callers see one merged view.
//
// Parameters:
//   - dst: the timestamp document parsed from the JSON column; must not be nil.
//
// Return values: none; dst is mutated in place.
func (t *Trace) overlayTimestampColumns(dst *TraceTimestamps) {
	if dst == nil {
		return
	}
	if t.TsRequestReceived != nil {
		dst.RequestReceived = t.TsRequestReceived
	}
	if t.TsRequestForwarded != nil {
		dst.RequestForwarded = t.TsRequestForwarded
	}
	if t.TsFirstUpstreamResponse != nil {
		dst.FirstUpstreamResponse = t.TsFirstUpstreamResponse
	}
	if t.TsFirstClientResponse != nil {
		dst.FirstClientResponse = t.TsFirstClientResponse
	}
	if t.TsUpstreamCompleted != nil {
		dst.UpstreamCompleted = t.TsUpstreamCompleted
	}
	if t.TsRequestCompleted != nil {
		dst.RequestCompleted = t.TsRequestCompleted
	}
}

// applyTimestampColumns projects a timestamp document onto the row's
// per-timestamp columns.
//
// Parameters:
//   - src: the timestamp document to project; a nil document clears the columns.
//
// Return values: none; the receiver is mutated in place.
func (t *Trace) applyTimestampColumns(src *TraceTimestamps) {
	if src == nil {
		t.TsRequestReceived = nil
		t.TsRequestForwarded = nil
		t.TsFirstUpstreamResponse = nil
		t.TsFirstClientResponse = nil
		t.TsUpstreamCompleted = nil
		t.TsRequestCompleted = nil
		return
	}
	t.TsRequestReceived = src.RequestReceived
	t.TsRequestForwarded = src.RequestForwarded
	t.TsFirstUpstreamResponse = src.FirstUpstreamResponse
	t.TsFirstClientResponse = src.FirstClientResponse
	t.TsUpstreamCompleted = src.UpstreamCompleted
	t.TsRequestCompleted = src.RequestCompleted
}

// timestampColumnName maps a lifecycle timestamp key to its dedicated column.
//
// Parameters:
//   - key: one of the Timestamp* constants.
//
// Return values:
//   - string: the column name, or "" when the key has no dedicated column.
func timestampColumnName(key string) string {
	switch key {
	case TimestampRequestReceived:
		return "ts_request_received"
	case TimestampRequestForwarded:
		return "ts_request_forwarded"
	case TimestampFirstUpstreamResponse:
		return "ts_first_upstream_response"
	case TimestampFirstClientResponse:
		return "ts_first_client_response"
	case TimestampUpstreamCompleted:
		return "ts_upstream_completed"
	case TimestampRequestCompleted:
		return "ts_request_completed"
	default:
		return ""
	}
}
