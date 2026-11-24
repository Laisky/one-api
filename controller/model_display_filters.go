package controller

import (
	"math"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
)

// modelDisplayFilters describes the optional filter parameters accepted by /api/models/display.
// All fields are derived from query string and applied AFTER pricing info is collected per model.
type modelDisplayFilters struct {
	inputModalities   []string // any-match against ModelConfig.InputModalities (empty = no filter)
	outputModalities  []string // any-match against ModelConfig.OutputModalities (empty = no filter)
	features          []string // all-match against ModelConfig.SupportedFeatures (empty = no filter)
	reasoningEfforts  []string // any-match against ModelConfig.SupportedReasoningEfforts (empty = no filter)
	channelTypes      []int    // restrict to specific channel type ids (empty = no filter)
	minContextLength  int32    // require ContextLength >= this (0 = no filter)
	maxInputPriceUsd  float64  // require InputPrice <= this (per 1M tokens, 0 = no filter)
	requireImage      bool     // require image pricing or image output
	requireVideo      bool     // require video pricing or video output
	requireAudio      bool     // require audio pricing or audio output
	requireEmbedding  bool     // require embedding pricing
	requireReasoning  bool     // require reasoning feature (any of supported_features contains "reasoning")
	requireTools      bool     // require tools feature
	requireWebSearch  bool     // require web_search feature
	requireStructured bool     // require structured_outputs feature
}

// hasAny returns true when haystack contains any of the needles (case-insensitive).
func hasAny(haystack []string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; ok {
			return true
		}
	}
	return false
}

// hasAll returns true when haystack contains every needle (case-insensitive).
func hasAll(haystack []string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; !ok {
			return false
		}
	}
	return true
}

// parseCSVQuery returns trimmed, lower-cased, non-empty tokens from a query parameter.
// Supports comma-separated single values and repeated query parameters.
func parseCSVQuery(c *gin.Context, key string) []string {
	values := c.QueryArray(key)
	if v := c.Query(key); v != "" && len(values) == 0 {
		values = []string{v}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			t := strings.ToLower(strings.TrimSpace(part))
			if t == "" {
				continue
			}
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// parseIntCSV returns the integer tokens of a comma-separated or repeated
// query parameter, skipping tokens that are not integers. Parameters: c is the
// request and key the parameter name. Returns: the parsed integers.
func parseIntCSV(c *gin.Context, key string) []int {
	raw := parseCSVQuery(c, key)
	out := make([]int, 0, len(raw))
	for _, v := range raw {
		if i, err := strconv.Atoi(v); err == nil {
			out = append(out, i)
		}
	}
	return out
}

// parseBoolQuery reports whether a query parameter is set to a truthy value
// (1, true, yes, on; case-insensitive). Parameters: c is the request and key
// the parameter name. Returns: true for a truthy value.
func parseBoolQuery(c *gin.Context, key string) bool {
	v := strings.ToLower(strings.TrimSpace(c.Query(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// parseInt32Query parses a non-negative int32 query parameter. A missing,
// malformed, or negative value is 0 (no filter); a value beyond int32 is
// clamped to math.MaxInt32 instead of wrapping around to a small number.
// Parameters: c is the request and key the parameter name. Returns: the value.
func parseInt32Query(c *gin.Context, key string) int32 {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if errors.Is(err, strconv.ErrRange) && n > 0 {
		return math.MaxInt32
	}
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

// parseFloatQuery parses a non-negative float query parameter; a missing,
// malformed, or negative value is 0 (no filter). Parameters: c is the request
// and key the parameter name. Returns: the value.
func parseFloatQuery(c *gin.Context, key string) float64 {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// parseModelDisplayFilters extracts every supported filter query parameter.
func parseModelDisplayFilters(c *gin.Context) modelDisplayFilters {
	return modelDisplayFilters{
		inputModalities:   parseCSVQuery(c, "input_modality"),
		outputModalities:  parseCSVQuery(c, "output_modality"),
		features:          parseCSVQuery(c, "feature"),
		reasoningEfforts:  parseCSVQuery(c, "reasoning_effort"),
		channelTypes:      parseIntCSV(c, "channel_type"),
		minContextLength:  parseInt32Query(c, "min_context_length"),
		maxInputPriceUsd:  parseFloatQuery(c, "max_input_price"),
		requireImage:      parseBoolQuery(c, "has_image"),
		requireVideo:      parseBoolQuery(c, "has_video"),
		requireAudio:      parseBoolQuery(c, "has_audio"),
		requireEmbedding:  parseBoolQuery(c, "has_embedding"),
		requireReasoning:  parseBoolQuery(c, "has_reasoning"),
		requireTools:      parseBoolQuery(c, "has_tools"),
		requireWebSearch:  parseBoolQuery(c, "has_web_search"),
		requireStructured: parseBoolQuery(c, "has_structured_outputs"),
	}
}

// hasContent reports whether any filter parameter is active.
func (f modelDisplayFilters) hasContent() bool {
	return len(f.inputModalities) > 0 || len(f.outputModalities) > 0 || len(f.features) > 0 ||
		len(f.reasoningEfforts) > 0 || len(f.channelTypes) > 0 || f.minContextLength > 0 ||
		f.maxInputPriceUsd > 0 || f.requireImage || f.requireVideo || f.requireAudio ||
		f.requireEmbedding || f.requireReasoning || f.requireTools || f.requireWebSearch || f.requireStructured
}

// matchesChannel reports whether the channel type is allowed by the filter (or no filter set).
func (f modelDisplayFilters) matchesChannel(channelType int) bool {
	if len(f.channelTypes) == 0 {
		return true
	}
	for _, t := range f.channelTypes {
		if t == channelType {
			return true
		}
	}
	return false
}

// matchesModel evaluates the per-model portion of the filter against the assembled ModelDisplayInfo.
// Empty filter fields are treated as "no constraint".
func (f modelDisplayFilters) matchesModel(info ModelDisplayInfo) bool {
	if len(f.inputModalities) > 0 && !hasAny(info.InputModalities, f.inputModalities) {
		return false
	}
	if len(f.outputModalities) > 0 && !hasAny(info.OutputModalities, f.outputModalities) {
		return false
	}
	if len(f.features) > 0 && !hasAll(info.SupportedFeatures, f.features) {
		return false
	}
	if len(f.reasoningEfforts) > 0 && !hasAny(info.SupportedReasoningEfforts, f.reasoningEfforts) {
		return false
	}
	if f.minContextLength > 0 && info.ContextLength < f.minContextLength {
		return false
	}
	if f.maxInputPriceUsd > 0 && info.InputPrice > f.maxInputPriceUsd {
		return false
	}
	if f.requireImage && info.ImagePricing == nil && !hasAny(info.OutputModalities, []string{"image"}) {
		return false
	}
	if f.requireVideo && info.VideoPricing == nil && !hasAny(info.OutputModalities, []string{"video"}) {
		return false
	}
	if f.requireAudio && info.AudioPricing == nil && !hasAny(info.OutputModalities, []string{"audio"}) && !hasAny(info.InputModalities, []string{"audio"}) {
		return false
	}
	if f.requireEmbedding && info.EmbeddingPricing == nil {
		return false
	}
	if f.requireReasoning && !hasAny(info.SupportedFeatures, []string{"reasoning"}) {
		return false
	}
	if f.requireTools && !hasAny(info.SupportedFeatures, []string{"tools"}) {
		return false
	}
	if f.requireWebSearch && !hasAny(info.SupportedFeatures, []string{"web_search"}) {
		return false
	}
	if f.requireStructured && !hasAny(info.SupportedFeatures, []string{"structured_outputs"}) {
		return false
	}
	return true
}
