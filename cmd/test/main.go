package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"
	_ "github.com/joho/godotenv/autoload"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/sync/errgroup"
)

type requestType string

const (
	requestTypeChatCompletion requestType = "chat_completion"
	requestTypeResponseAPI    requestType = "response_api"
	requestTypeClaudeMessages requestType = "claude_messages"

	defaultAPIBase      = "https://oneapi.laisky.com"
	defaultTestModels   = "gpt-4o-mini,gpt-5-mini,claude-3.5-haiku,gemini-2.5-flash,openai/gpt-oss-20b,deepseek-chat"
	defaultMaxTokens    = 2048
	defaultTemperature  = 0.7
	defaultTopP         = 0.9
	defaultTopK         = 40
	maxResponseBodySize = 1 << 20 // 1 MiB
	maxLoggedBodyBytes  = 2048
)

type requestVariant struct {
	Key    string
	Header string
	Type   requestType
	Path   string
	Stream bool
}

var requestVariants = []requestVariant{
	{Key: "chat_stream_false", Header: "Chat (stream=false)", Type: requestTypeChatCompletion, Path: "/v1/chat/completions", Stream: false},
	{Key: "chat_stream_true", Header: "Chat (stream=true)", Type: requestTypeChatCompletion, Path: "/v1/chat/completions", Stream: true},
	{Key: "response_stream_false", Header: "Response (stream=false)", Type: requestTypeResponseAPI, Path: "/v1/responses", Stream: false},
	{Key: "response_stream_true", Header: "Response (stream=true)", Type: requestTypeResponseAPI, Path: "/v1/responses", Stream: true},
	{Key: "claude_stream_false", Header: "Claude (stream=false)", Type: requestTypeClaudeMessages, Path: "/v1/messages", Stream: false},
	{Key: "claude_stream_true", Header: "Claude (stream=true)", Type: requestTypeClaudeMessages, Path: "/v1/messages", Stream: true},
}

type testResult struct {
	Model        string
	Variant      string
	Label        string
	Type         requestType
	Stream       bool
	Success      bool
	Skipped      bool
	StatusCode   int
	Duration     time.Duration
	ErrorReason  string
	RequestBody  string
	ResponseBody string
}

type requestSpec struct {
	Variant string
	Label   string
	Type    requestType
	Path    string
	Body    any
	Stream  bool
}

func main() {
	logger, err := glog.NewConsoleWithName("oneapi-test", glog.LevelInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %+v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("test run failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("all tests passed")
}

func run(ctx context.Context, logger glog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return errors.Wrap(err, "load config")
	}

	logger.Info("starting API regression sweep",
		zap.String("base_url", cfg.APIBase),
		zap.Int("model_count", len(cfg.Models)),
	)

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resultsCh := make(chan testResult, len(cfg.Models)*len(requestVariants))

	var (
		results   []testResult
		collectWg sync.WaitGroup
	)

	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		for res := range resultsCh {
			results = append(results, res)
			switch {
			case res.Success:
				logger.Info("request succeeded",
					zap.String("model", res.Model),
					zap.String("variant", res.Label),
					zap.String("type", string(res.Type)),
					zap.Bool("stream", res.Stream),
					zap.Duration("duration", res.Duration),
					zap.Int("status", res.StatusCode),
				)
			case res.Skipped:
				logger.Info("request skipped",
					zap.String("model", res.Model),
					zap.String("variant", res.Label),
					zap.String("type", string(res.Type)),
					zap.Bool("stream", res.Stream),
					zap.Int("status", res.StatusCode),
					zap.String("reason", res.ErrorReason),
				)
			default:
				logger.Warn("request failed",
					zap.String("model", res.Model),
					zap.String("variant", res.Label),
					zap.String("type", string(res.Type)),
					zap.Bool("stream", res.Stream),
					zap.Duration("duration", res.Duration),
					zap.Int("status", res.StatusCode),
					zap.String("error", res.ErrorReason),
					zap.String("request_body", res.RequestBody),
					zap.String("response_body", res.ResponseBody),
				)
			}
		}
	}()

	grp, grpCtx := errgroup.WithContext(ctx)
	for _, model := range cfg.Models {
		model := model
		grp.Go(func() error {
			executeModelSweep(grpCtx, httpClient, cfg, model, resultsCh)
			return nil
		})
	}

	_ = grp.Wait()
	close(resultsCh)
	collectWg.Wait()

	report := buildReport(cfg.Models, requestVariants, results)
	renderReport(report)

	if report.failedCount > 0 {
		return errors.Errorf("%d of %d requests failed", report.failedCount, report.totalRequests)
	}

	return nil
}

type config struct {
	APIBase string
	Token   string
	Models  []string
}

func loadConfig() (config, error) {
	base := strings.TrimSpace(os.Getenv("API_BASE"))
	if base == "" {
		base = defaultAPIBase
	}

	token := strings.TrimSpace(os.Getenv("API_TOKEN"))
	if token == "" {
		return config{}, errors.Errorf("API_TOKEN must be set")
	}

	modelsRaw := os.Getenv("ONEAPI_TEST_MODELS")
	models, err := parseModels(modelsRaw)
	if err != nil {
		return config{}, errors.Wrap(err, "parse models")
	}
	if len(models) == 0 {
		models, err = parseModels(defaultTestModels)
		if err != nil {
			return config{}, errors.Wrap(err, "parse default models")
		}
	}

	return config{
		APIBase: strings.TrimSuffix(base, "/"),
		Token:   token,
		Models:  models,
	}, nil
}

func parseModels(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	separators := []string{",", ";", "\n", "\r"}
	normalized := raw
	for _, sep := range separators {
		normalized = strings.ReplaceAll(normalized, sep, ",")
	}

	parts := strings.Split(normalized, ",")
	if len(parts) == 1 && !strings.Contains(raw, ",") && !strings.Contains(raw, ";") && !strings.Contains(raw, "\n") {
		parts = strings.Fields(raw)
	}

	var models []string
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		models = append(models, candidate)
	}

	return models, nil
}

func executeModelSweep(ctx context.Context, client *http.Client, cfg config, model string, results chan<- testResult) {
	specs := buildRequestSpecs(model)

	innerGrp, innerCtx := errgroup.WithContext(ctx)
	for _, spec := range specs {
		spec := spec
		innerGrp.Go(func() error {
			res := performRequest(innerCtx, client, cfg.APIBase, cfg.Token, spec, model)
			select {
			case results <- res:
			case <-innerCtx.Done():
			}
			return nil
		})
	}

	_ = innerGrp.Wait()
}

func buildRequestSpecs(model string) []requestSpec {
	specs := make([]requestSpec, 0, len(requestVariants))
	for _, variant := range requestVariants {
		var body any
		switch variant.Type {
		case requestTypeChatCompletion:
			body = chatCompletionPayload(model, variant.Stream)
		case requestTypeResponseAPI:
			body = responseAPIPayload(model, variant.Stream)
		case requestTypeClaudeMessages:
			body = claudeMessagesPayload(model, variant.Stream)
		default:
			body = nil
		}

		specs = append(specs, requestSpec{
			Variant: variant.Key,
			Label:   variant.Header,
			Type:    variant.Type,
			Path:    variant.Path,
			Body:    body,
			Stream:  variant.Stream,
		})
	}

	return specs
}

func performRequest(ctx context.Context, client *http.Client, baseURL, token string, spec requestSpec, model string) (result testResult) {
	start := time.Now()
	result = testResult{
		Model:   model,
		Variant: spec.Variant,
		Label:   spec.Label,
		Type:    spec.Type,
		Stream:  spec.Stream,
	}
	defer func() {
		result.Duration = time.Since(start)
	}()

	payload, err := json.Marshal(spec.Body)
	if err != nil {
		result.ErrorReason = fmt.Sprintf("marshal payload: %v", err)
		return
	}
	result.RequestBody = truncateString(string(payload), maxLoggedBodyBytes)

	endpoint := baseURL + spec.Path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		result.ErrorReason = fmt.Sprintf("build request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "oneapi-test-harness/1.0")

	resp, err := client.Do(req)
	if err != nil {
		result.ErrorReason = fmt.Sprintf("do request: %v", err)
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if spec.Stream {
		streamData, streamErr := collectStreamBody(resp.Body, maxResponseBodySize)
		if len(streamData) > 0 {
			result.ResponseBody = truncateString(string(streamData), maxLoggedBodyBytes)
		}
		if streamErr != nil {
			result.ErrorReason = fmt.Sprintf("stream read: %v", streamErr)
			return
		}

		success, reason := evaluateStreamResponse(spec.Type, streamData)
		if success {
			result.Success = true
			return
		}

		if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, streamData, reason) {
			result.Skipped = false
			result.ErrorReason = reason
			return
		}

		if reason == "" {
			reason = snippet(streamData)
		}
		result.ErrorReason = reason
		return
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if len(body) > 0 {
		result.ResponseBody = truncateString(string(body), maxLoggedBodyBytes)
	}
	if readErr != nil {
		result.ErrorReason = fmt.Sprintf("read response: %v", readErr)
		return
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		success, reason := evaluateResponse(spec.Type, body)
		if success {
			result.Success = true
			return
		}

		if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, body, reason) {
			result.Skipped = false
			result.ErrorReason = reason
			return
		}

		if reason == "" {
			reason = snippet(body)
		}
		result.ErrorReason = reason
		return
	}

	reason := fmt.Sprintf("status %s: %s", resp.Status, snippet(body))
	if isUnsupportedCombination(spec.Type, spec.Stream, resp.StatusCode, body, reason) {
		result.Skipped = false
		result.ErrorReason = reason
		return
	}

	result.ErrorReason = reason
	return
}

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
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return buffer.Bytes(), err
		}
		if len(strings.TrimSpace(string(chunk))) == 0 && buffer.Len() > 0 {
			break
		}
	}

	if buffer.Len() == 0 {
		return buffer.Bytes(), fmt.Errorf("no stream data received")
	}

	return buffer.Bytes(), nil
}

func evaluateResponse(reqType requestType, body []byte) (bool, string) {
	if len(body) == 0 {
		return true, ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Sprintf("malformed JSON response: %v", err)
	}

	if errVal, ok := payload["error"]; ok && isMeaningfulErrorValue(errVal) {
		return false, snippet(body)
	}

	switch reqType {
	case requestTypeChatCompletion:
		if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
			return true, ""
		}
		return false, "response missing choices"
	case requestTypeResponseAPI:
		status := stringValue(payload, "status")
		if status == "failed" {
			return false, snippet(body)
		}
		if output, ok := payload["output"].([]any); ok && len(output) > 0 {
			return true, ""
		}
		if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
			return true, ""
		}
		if status == "completed" || status == "in_progress" || status == "requires_action" {
			return true, ""
		}
		if len(payload) == 0 {
			return false, "empty response"
		}
		return false, "response missing output"
	case requestTypeClaudeMessages:
		if content, ok := payload["content"].([]any); ok && len(content) > 0 {
			return true, ""
		}
		if msgType := stringValue(payload, "type"); msgType != "" {
			return true, ""
		}
		if len(payload) == 0 {
			return false, "empty response"
		}
		return true, ""
	default:
		return true, ""
	}
}

func evaluateStreamResponse(reqType requestType, data []byte) (bool, string) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false, "empty stream"
	}

	lines := bytes.Split(trimmed, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			payload := bytes.TrimSpace(line[len("data:"):])
			if len(payload) == 0 {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal(payload, &obj); err == nil {
				if errVal, ok := obj["error"]; ok && isMeaningfulErrorValue(errVal) {
					return false, snippet(payload)
				}
			}
		}
	}

	lower := bytes.ToLower(trimmed)
	if bytes.Contains(lower, []byte("\"error\"")) && !bytes.Contains(lower, []byte("\"error\":null")) {
		return false, snippet(trimmed)
	}

	return true, ""
}

func stringValue(data map[string]any, key string) string {
	if raw, ok := data[key]; ok {
		if s, ok := raw.(string); ok {
			return s
		}
	}
	return ""
}

func isMeaningfulErrorValue(val any) bool {
	switch v := val.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case map[string]any:
		if len(v) == 0 {
			return false
		}
		for _, nested := range v {
			if isMeaningfulErrorValue(nested) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range v {
			if isMeaningfulErrorValue(item) {
				return true
			}
		}
		return false
	case bool:
		return v
	case float64:
		return v != 0
	default:
		return true
	}
}

func isUnsupportedCombination(reqType requestType, stream bool, statusCode int, body []byte, reason string) bool {
	text := reason
	if text == "" {
		text = snippet(body)
	}
	lower := strings.ToLower(text)

	switch reqType {
	case requestTypeResponseAPI:
		if strings.Contains(lower, "unknown field `messages`") ||
			strings.Contains(lower, "does not support responses") ||
			strings.Contains(lower, "response api is not available") {
			return true
		}
	case requestTypeChatCompletion:
		if strings.Contains(lower, "only supports response") ||
			strings.Contains(lower, "chat completions unsupported") {
			return true
		}
	case requestTypeClaudeMessages:
		if strings.Contains(lower, "does not support claude") ||
			strings.Contains(lower, "claude messages unsupported") {
			return true
		}
	}

	if stream && (strings.Contains(lower, "streaming is not supported") ||
		strings.Contains(lower, "stream parameter is not supported") ||
		strings.Contains(lower, "stream currently disabled")) {
		return true
	}

	if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed {
		return true
	}

	return false
}

func chatCompletionPayload(model string, stream bool) any {
	return map[string]any{
		"model":       model,
		"max_tokens":  defaultMaxTokens,
		"temperature": defaultTemperature,
		"top_p":       defaultTopP,
		"stream":      stream,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Say hello in one sentence.",
			},
		},
	}
}

func responseAPIPayload(model string, stream bool) any {
	return map[string]any{
		"model":             model,
		"max_output_tokens": defaultMaxTokens,
		"temperature":       defaultTemperature,
		"top_p":             defaultTopP,
		"stream":            stream,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": "Say hello in one sentence.",
					},
				},
			},
		},
	}
}

func claudeMessagesPayload(model string, stream bool) any {
	return map[string]any{
		"model":       model,
		"max_tokens":  defaultMaxTokens,
		"temperature": defaultTemperature,
		"top_p":       defaultTopP,
		"top_k":       defaultTopK,
		"stream":      stream,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "text",
						"text": "Say hello in one sentence.",
					},
				},
			},
		},
	}
}

type report struct {
	models         []string
	variants       []requestVariant
	resultsByModel map[string]map[string]testResult
	totalRequests  int
	failedCount    int
	skippedCount   int
}

func buildReport(models []string, variants []requestVariant, results []testResult) report {
	byModel := make(map[string]map[string]testResult, len(models))
	for _, model := range models {
		byModel[model] = make(map[string]testResult)
	}

	failed := 0
	skipped := 0
	for _, res := range results {
		if res.Model == "" {
			continue
		}
		modelMap, ok := byModel[res.Model]
		if !ok {
			modelMap = make(map[string]testResult)
			byModel[res.Model] = modelMap
		}
		modelMap[res.Variant] = res
		if res.Skipped {
			skipped++
			continue
		}
		if !res.Success {
			failed++
		}
	}

	return report{
		models:         models,
		variants:       variants,
		resultsByModel: byModel,
		totalRequests:  len(results),
		failedCount:    failed,
		skippedCount:   skipped,
	}
}

func renderReport(rep report) {
	if len(rep.models) == 0 {
		fmt.Println("no models to report")
		return
	}

	header := []string{"Model"}
	for _, variant := range rep.variants {
		header = append(header, variant.Header)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.SetAutoWrapText(false)
	table.SetReflowDuringAutoWrap(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_CENTER)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(true)
	table.SetCenterSeparator("│")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetBorder(true)

	for _, model := range rep.models {
		entry := rep.resultsByModel[model]
		row := make([]string, 0, len(rep.variants)+1)
		row = append(row, model)
		for _, variant := range rep.variants {
			row = append(row, formatCell(entry[variant.Key]))
		}
		table.Append(row)
	}

	fmt.Println()
	fmt.Println("=== One-API Regression Report ===")
	table.Render()

	passed := rep.totalRequests - rep.failedCount - rep.skippedCount
	fmt.Printf("\nTotals  | Requests: %d | Passed: %d | Failed: %d | Skipped: %d\n",
		rep.totalRequests,
		passed,
		rep.failedCount,
		rep.skippedCount,
	)

	failures, skips := gatherOutcomes(rep)
	if len(failures) > 0 {
		fmt.Println("\nFailures:")
		for _, res := range failures {
			fmt.Printf("- %s · %s → %s\n", res.Model, res.Label, shorten(res.ErrorReason, 200))
		}
	}
	if len(skips) > 0 {
		fmt.Println("\nSkipped (unsupported combinations):")
		for _, res := range skips {
			fmt.Printf("- %s · %s → %s\n", res.Model, res.Label, shorten(res.ErrorReason, 200))
		}
	}

	fmt.Println()
}

func gatherOutcomes(rep report) (failures, skips []testResult) {
	for _, model := range rep.models {
		entry := rep.resultsByModel[model]
		for _, variant := range rep.variants {
			res, ok := entry[variant.Key]
			if !ok || res.Model == "" {
				continue
			}
			if res.Skipped {
				skips = append(skips, res)
				continue
			}
			if !res.Success {
				failures = append(failures, res)
			}
		}
	}

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Model == failures[j].Model {
			return failures[i].Label < failures[j].Label
		}
		return failures[i].Model < failures[j].Model
	})
	sort.Slice(skips, func(i, j int) bool {
		if skips[i].Model == skips[j].Model {
			return skips[i].Label < skips[j].Label
		}
		return skips[i].Model < skips[j].Model
	})

	return failures, skips
}

func formatCell(res testResult) string {
	if res.Model == "" {
		return "—"
	}

	switch {
	case res.Success:
		return fmt.Sprintf("PASS\n%s", res.Duration.Truncate(time.Millisecond))
	case res.Skipped:
		detail := res.ErrorReason
		if detail == "" {
			detail = "unsupported"
		}
		return fmt.Sprintf("SKIP\n%s", shorten(detail, 60))
	default:
		detail := res.ErrorReason
		if detail == "" {
			detail = res.Duration.String()
		}
		return fmt.Sprintf("FAIL\n%s", shorten(detail, 60))
	}
}

func shorten(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func truncateString(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func snippet(body []byte) string {
	const maxLen = 256
	cleaned := strings.TrimSpace(string(body))
	if len(cleaned) <= maxLen {
		return cleaned
	}
	return cleaned[:maxLen] + "…"
}
