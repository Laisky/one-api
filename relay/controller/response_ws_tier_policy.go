package controller

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/state"
)

// responseWSTierFunding prices each accepted create using the complete inherited
// context. The client-reader owns quoted/reserved; the upstream-reader only
// publishes confirmed parent receipts under mu. No gin.Context crosses workers.
type responseWSTierFunding struct {
	mu      sync.RWMutex
	parents map[string]int // -1 means the confirmed receipt lacks priceable usage

	meta             metalib.Meta
	modelRatios      map[string]float64
	completionRatios map[string]float64
	configs          map[string]model.ModelConfigLocal
	modelRatio       float64
	groupRatio       float64
	provider         adaptor.Adaptor
	enabled          bool

	quoted   int64 // total per-create tier allowance admitted on this socket
	reserved int64 // handshake tier allowance plus incremental tier holds
}

// newResponseWSTierFunding captures immutable pricing and identity before the
// WebSocket proxy starts its concurrent copy loops.
func newResponseWSTierFunding(c *gin.Context, meta *metalib.Meta, handshakeHold int64) *responseWSTierFunding {
	ratios, completions := getChannelRatios(c)
	configs := getChannelModelConfigs(c)
	provider := resolvePricingAdaptor(meta)
	group := c.GetFloat64(ctxkey.ChannelRatio)
	cfg, known := pricing.ResolveModelConfigRatioOnly(meta.ActualModelName, configs, provider, meta.StartTime)
	return &responseWSTierFunding{
		meta:             *meta,
		modelRatios:      ratios,
		completionRatios: completions,
		configs:          configs,
		modelRatio:       pricing.ResolveModelRatioAt(meta.ActualModelName, configs, ratios, provider, meta.StartTime),
		groupRatio:       group,
		provider:         provider,
		enabled:          known && len(cfg.Tiers) > 0 && group != 0,
		reserved:         handshakeHold,
	}
}

// observe accepts provider-confirmed, terminal response usage for continuations
// within this socket. The gateway persists stored receipts only when the socket
// finishes, so state.Store cannot resolve an earlier store=false/local turn.
func (s *responseWSTierFunding) observe(frame []byte) {
	if s == nil || !s.enabled {
		return
	}
	var event struct {
		Type     string `json:"type"`
		Response struct {
			ID     string          `json:"id"`
			Model  string          `json:"model"`
			Status string          `json:"status"`
			Usage  json.RawMessage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame, &event); err != nil || event.Response.ID == "" ||
		(event.Type != "response.completed" && event.Type != "response.incomplete") {
		return
	}
	// The native WS collector accepts terminal receipts without a status for
	// compatibility. Reject conflicting status/event pairs, but allow capped
	// incomplete turns as valid continuations just like the HTTP path.
	status := strings.ToLower(strings.TrimSpace(event.Response.Status))
	if (event.Type == "response.completed" && status != "" && status != "completed") ||
		(event.Type == "response.incomplete" && status != "" && status != "incomplete") {
		return
	}
	// The already-authenticated WebSocket pins provider/model selection. The
	// echoed model can be a canonical dated snapshot for the requested alias,
	// so it is not an independent authorization or pricing source.
	// Raw provider receipts preserve explicit zero output, unlike typed stored
	// historical records whose omitted counters cannot establish that zero.
	tokens, err := responseTierObservedInheritedTokens(event.Response.Usage)
	if err != nil {
		tokens = -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parents == nil {
		s.parents = make(map[string]int)
	}
	if _, exists := s.parents[event.Response.ID]; exists {
		return
	}
	if len(s.parents) < 1024 {
		s.parents[event.Response.ID] = tokens
	}
}

// inherited returns the full preceding input+output token count for a parent.
// Owner/channel/model affinity is rechecked for persisted parents, after the
// proxy has authorized and rewritten the upstream response ID.
func (s *responseWSTierFunding) inherited(ctx context.Context, parentID string) (int, error) {
	s.mu.RLock()
	value, exists := s.parents[parentID]
	s.mu.RUnlock()
	if exists {
		if value <= 0 {
			return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket parent receipt has no reliable usage")
		}
		return value, nil
	}
	if !state.Enabled() || state.Store() == nil {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket parent storage is unavailable")
	}
	owner := stateOwnerFromMeta(&s.meta)
	if !owner.Valid() {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket continuation owner is unavailable")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	parent, err := state.Store().GetResponse(lookupCtx, owner, parentID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return 0, errors.Wrap(errResponseContinuationUnpriceable, "owned WebSocket parent was not found")
		}
		return 0, errors.Wrap(errors.Join(errResponseContinuationStoreUnavailable, err), "load WebSocket parent usage")
	}
	if parent == nil || parent.Owner != owner || parent.Binding == nil {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket parent binding is unavailable")
	}
	switch parent.Status {
	case state.StatusCompleted, state.StatusIncomplete, state.StatusFailed, state.StatusCancelled:
	default:
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket parent is not terminal")
	}
	binding := parent.Binding
	if binding.ChannelID != s.meta.ChannelId || binding.APIType != s.meta.APIType ||
		binding.ActualModel != s.meta.ActualModelName || binding.UpstreamResponseID == "" {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "WebSocket parent pricing identity does not match")
	}
	return responseTierInheritedTokens(parent.Usage)
}

// additionalQuote returns the extra atomic hold and a per-turn quote without
// mutating reservations. The caller commits both only after ALL policies accept
// the frame and the durable debit succeeds.
func (s *responseWSTierFunding) additionalQuote(ctx context.Context, frame []byte) (int64, int64, error) {
	if s == nil || !s.enabled {
		return 0, 0, nil
	}
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(frame, &event); err != nil {
		return 0, 0, errors.Wrap(err, "decode WebSocket event type for tier admission")
	}
	if event.Type != "response.create" {
		// Other WebSocket control/event frames are not inference requests and
		// must not be charged or blocked by the per-create funding policy.
		return 0, 0, nil
	}
	var request openai.ResponseAPIRequest
	// Reject conflicting decoded/priced fields while passing unknown provider
	// extensions through untouched. The same typed-root guard protects HTTP.
	if err := common.ValidateUnambiguousJSONRootKeys(frame, request); err != nil {
		return 0, 0, errors.Wrap(err, "validate WebSocket tiered response.create pricing keys")
	}
	if err := json.Unmarshal(frame, &request); err != nil {
		return 0, 0, errors.Wrap(err, "decode WebSocket tiered response.create frame")
	}
	// The proxy already pinned and validated the mapped model before admission.
	// Price the actual provider model without editing the original wire bytes.
	request.Model = s.meta.ActualModelName
	prompt := getResponseAPIPromptTokens(ctx, &request)
	if prompt < 0 {
		return 0, 0, errors.New("invalid WebSocket prompt estimate")
	}
	if request.PreviousResponseId != nil && strings.TrimSpace(*request.PreviousResponseId) != "" {
		inherited, err := s.inherited(ctx, strings.TrimSpace(*request.PreviousResponseId))
		if err != nil {
			return 0, 0, err
		}
		if inherited > math.MaxInt-prompt {
			return 0, 0, errors.New("WebSocket inherited token budget overflows")
		}
		prompt += inherited
	}
	var maxOutput int
	if request.MaxOutputTokens != nil {
		maxOutput = *request.MaxOutputTokens
	}
	write5m, write1h, err := tierAdmissionCacheWrites(json.RawMessage(frame))
	if err != nil {
		return 0, 0, errors.Wrap(err, "inspect WebSocket tiered cache controls")
	}
	quote, _, err := quotautil.EstimateTierAdmission(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{PromptTokens: prompt}, ModelName: request.Model,
		ModelRatio: s.modelRatio, ChannelModelRatio: s.modelRatios, GroupRatio: s.groupRatio,
		ChannelModelConfigs: s.configs, ChannelCompletionRatio: s.completionRatios,
		PricingAdaptor: s.provider, RequestTime: s.meta.StartTime,
	}, maxOutput, 0, quotautil.AdmissionOptions{CacheWrite5m: write5m, CacheWrite1h: write1h})
	if err != nil {
		return 0, 0, errors.Wrap(err, "quote WebSocket inherited tier allowance")
	}
	if quote < 0 || quote > math.MaxInt64-s.quoted {
		return 0, 0, errors.New("WebSocket tiered allowance overflows")
	}
	want := s.quoted + quote
	if want > s.reserved {
		return want - s.reserved, quote, nil
	}
	return 0, quote, nil
}

func (s *responseWSTierFunding) accepted(quote, added int64) {
	if s == nil || !s.enabled {
		return
	}
	s.quoted += quote
	s.reserved += added
}

// responseTierInheritedTokens validates persisted parent usage. Historical
// typed state serialization omits zero counters, so an absent field or stored
// zero cannot authorize previously unbudgeted provider context.
func responseTierInheritedTokens(raw json.RawMessage) (int, error) {
	return responseTierInheritedTokensValidated(raw, false)
}

// responseTierObservedInheritedTokens accepts explicit zero output only on an
// original provider terminal receipt, where JSON field presence is preserved.
// This permits WebSocket warmup (generate=false) to be used as a local parent.
func responseTierObservedInheritedTokens(raw json.RawMessage) (int, error) {
	return responseTierInheritedTokensValidated(raw, true)
}

func responseTierInheritedTokensValidated(raw json.RawMessage, allowExplicitZeroOutput bool) (int, error) {
	var usage struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
	}
	if len(raw) == 0 {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "parent usage is unavailable")
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return 0, errors.Wrap(errors.Join(errResponseContinuationUnpriceable, err), "decode parent usage")
	}
	if usage.InputTokens == nil || usage.OutputTokens == nil ||
		*usage.InputTokens <= 0 || *usage.OutputTokens < 0 ||
		(!allowExplicitZeroOutput && *usage.OutputTokens == 0) {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "parent counters are absent, invalid, or ambiguous zeros")
	}
	if *usage.InputTokens > math.MaxInt-*usage.OutputTokens {
		return 0, errors.New("parent token budget overflows")
	}
	return *usage.InputTokens + *usage.OutputTokens, nil
}
