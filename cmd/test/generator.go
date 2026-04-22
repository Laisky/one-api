package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// generate executes the configured request variants and records request/response payloads to disk.
// It uses the same configuration loader as the regression harness so the command honours the
// ONEAPI_TEST_* environment overrides. Files are written beneath cmd/test/generated.
func generate(ctx context.Context, logger glog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return errors.Wrap(err, "load config")
	}

	outputDir := filepath.Join("cmd", "test", "generated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return errors.Wrap(err, "create output directory")
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}
	timestamp := time.Now().UTC().Format(generationTimestampLayout)

	for _, model := range cfg.Models {
		specs := buildRequestSpecs(model, cfg.Variants)
		for _, spec := range specs {
			select {
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "generate aborted")
			default:
			}

			prettyPayload, err := json.MarshalIndent(spec.Body, "", "  ")
			if err != nil {
				return errors.Wrap(err, "marshal request payload")
			}

			if skip, reason := shouldSkipVariant(model, spec); skip {
				record := generationRecord{
					Timestamp:      timestamp,
					Model:          model,
					RequestFormat:  spec.RequestFormat,
					VariantLabel:   spec.Label,
					Path:           spec.Path,
					Stream:         spec.Stream,
					Expectation:    expectationName(spec.Expectation),
					RequestPayload: string(prettyPayload),
					Success:        false,
					Skipped:        true,
					Error:          reason,
				}

				filePath, err := writeGenerationRecord(outputDir, timestamp, model, spec.RequestFormat, record)
				if err != nil {
					return errors.Wrap(err, "write generation artifact")
				}

				logger.Info("variant skipped",
					zap.String("model", model),
					zap.String("variant", spec.Label),
					zap.String("file", filePath),
					zap.Bool("skipped", true),
					zap.String("error", reason),
				)
				continue
			}

			existingPath, existingRecord, err := loadExistingGenerationRecord(outputDir, model, spec.RequestFormat)
			if err != nil {
				logger.Warn("failed to inspect existing generation artifact",
					zap.String("model", model),
					zap.String("variant", spec.Label),
					zap.Error(err),
				)
			} else if existingRecord != nil {
				if err := validateGenerationRecordStructure(existingRecord); err != nil {
					logger.Warn("existing generation artifact invalid",
						zap.String("model", model),
						zap.String("variant", spec.Label),
						zap.String("file", existingPath),
						zap.Error(err),
					)
				} else if err := ensureGenerationRecordSuccessful(existingRecord); err != nil {
					logger.Info("existing generation artifact not reusable",
						zap.String("model", model),
						zap.String("variant", spec.Label),
						zap.String("file", existingPath),
						zap.Error(err),
					)
				} else {
					logger.Info("existing generation artifact is valid; skipping request",
						zap.String("model", model),
						zap.String("variant", spec.Label),
						zap.String("file", existingPath),
					)
					continue
				}
			}

			result := performRequest(ctx, httpClient, cfg.APIBase, cfg.Token, spec, model)

			record := generationRecord{
				Timestamp:      timestamp,
				Model:          model,
				RequestFormat:  spec.RequestFormat,
				VariantLabel:   spec.Label,
				Path:           spec.Path,
				Stream:         spec.Stream,
				Expectation:    expectationName(spec.Expectation),
				RequestPayload: string(prettyPayload),
				SerializedBody: result.RequestBody,
				ResponseBody:   result.ResponseBody,
				StatusCode:     result.StatusCode,
				Success:        result.Success,
				Skipped:        result.Skipped,
				Error:          result.ErrorReason,
				LatencyMillis:  result.Duration.Milliseconds(),
			}

			filePath, err := writeGenerationRecord(outputDir, timestamp, model, spec.RequestFormat, record)
			if err != nil {
				return errors.Wrap(err, "write generation artifact")
			}

			lgFields := []zap.Field{
				zap.String("model", model),
				zap.String("variant", spec.Label),
				zap.String("file", filePath),
				zap.Bool("success", result.Success),
				zap.Bool("skipped", result.Skipped),
				zap.String("error", result.ErrorReason),
			}
			if result.Success {
				logger.Info("recorded payload", lgFields...)
			} else {
				logger.Error("recorded payload with issues", lgFields...)
			}
		}
	}

	return nil
}

// generationRecord encapsulates the captured request/response data for artefact generation.
type generationRecord struct {
	Timestamp      string `json:"timestamp"`
	Model          string `json:"model"`
	RequestFormat  string `json:"request_format"`
	VariantLabel   string `json:"variant_label"`
	Path           string `json:"path"`
	Stream         bool   `json:"stream"`
	Expectation    string `json:"expectation"`
	RequestPayload string `json:"request_payload"`
	SerializedBody string `json:"request_body_logged"`
	ResponseBody   string `json:"response_body"`
	StatusCode     int    `json:"status_code"`
	Success        bool   `json:"success"`
	Skipped        bool   `json:"skipped"`
	Error          string `json:"error"`
	LatencyMillis  int64  `json:"latency_ms"`
}

var fileNameSafePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// sanitizeFileComponent converts arbitrary model or variant identifiers into filename-safe tokens.
func sanitizeFileComponent(raw string) string {
	sanitized := fileNameSafePattern.ReplaceAllString(strings.ToLower(raw), "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		return "payload"
	}
	return sanitized
}

func generationFileName(timestamp, model, requestFormat string) string {
	return strings.Join([]string{timestamp, sanitizeFileComponent(model), sanitizeFileComponent(requestFormat)}, "_") + ".json"
}

const generationTimestampLayout = "20060102T150405Z"

// loadExistingGenerationRecord returns the most recent generation artifact for the model/request format pair.
func loadExistingGenerationRecord(outputDir, model, requestFormat string) (string, *generationRecord, error) {
	pattern := filepath.Join(outputDir, "*_"+sanitizeFileComponent(model)+"_"+sanitizeFileComponent(requestFormat)+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", nil, errors.Wrap(err, "glob existing generation records")
	}
	if len(matches) == 0 {
		return "", nil, nil
	}

	sort.Strings(matches)
	existingPath := matches[len(matches)-1]

	data, err := os.ReadFile(existingPath)
	if err != nil {
		return "", nil, errors.Wrapf(err, "read generation record %s", existingPath)
	}

	var record generationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return existingPath, nil, errors.Wrap(err, "unmarshal generation record")
	}

	return existingPath, &record, nil
}

// validateGenerationRecordStructure ensures the stored record matches the expected schema.
func validateGenerationRecordStructure(record *generationRecord) error {
	if record == nil {
		return errors.New("generation record is nil")
	}
	if record.Timestamp == "" {
		return errors.New("generation record missing timestamp")
	}
	if _, err := time.Parse(generationTimestampLayout, record.Timestamp); err != nil {
		return errors.Wrap(err, "invalid generation record timestamp")
	}
	if record.Model == "" {
		return errors.New("generation record missing model")
	}
	if record.RequestFormat == "" {
		return errors.New("generation record missing request_format")
	}
	if record.VariantLabel == "" {
		return errors.New("generation record missing variant_label")
	}
	if record.Path == "" {
		return errors.New("generation record missing path")
	}
	if record.Expectation == "" {
		return errors.New("generation record missing expectation")
	}
	if record.RequestPayload == "" {
		return errors.New("generation record missing request_payload")
	}
	return nil
}

// ensureGenerationRecordSuccessful validates that the stored record represents a successful execution.
func ensureGenerationRecordSuccessful(record *generationRecord) error {
	if record == nil {
		return errors.New("generation record is nil")
	}
	if record.Skipped {
		return errors.New("generation record flagged as skipped")
	}
	if record.StatusCode < http.StatusOK || record.StatusCode >= http.StatusMultipleChoices {
		return errors.Errorf("status_code %d indicates failure", record.StatusCode)
	}
	if !record.Success {
		return errors.New("generation record marked unsuccessful")
	}
	if strings.TrimSpace(record.Error) != "" {
		return errors.New("generation record contains error message")
	}
	return nil
}

func writeGenerationRecord(outputDir, timestamp, model, requestFormat string, record generationRecord) (string, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "marshal generation record")
	}

	fileName := generationFileName(timestamp, model, requestFormat)
	filePath := filepath.Join(outputDir, fileName)

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", errors.Wrap(err, "write generation record file")
	}

	return filePath, nil
}

// expectationName returns a stable string label for the given expectation category.
func expectationName(exp expectation) string {
	switch exp {
	case expectationDefault:
		return "default"
	case expectationToolInvocation:
		return "tool_invocation"
	case expectationVision:
		return "vision"
	case expectationStructuredOutput:
		return "structured_output"
	case expectationToolHistory:
		return "tool_history"
	default:
		return "unknown"
	}
}
