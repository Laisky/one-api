package controller

import (
	"encoding/json"
	"math"
	"strings"
	"sync"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/tooling"
)

// deepSeekWSConservativeMeter tracks successfully dispatched response.create
// tools separately from provider output. Socket legs update under the lock;
// request-scoped gin values are touched only by settle after both legs drain.
type deepSeekWSConservativeMeter struct {
	mu           sync.Mutex
	target       string
	channel      *model.Channel
	forwarded    map[string]int64
	observedIDs  map[string]map[string]struct{}
	anonymous    map[string]int64
	snapshotAnon map[string]int64
}

func newDeepSeekWSConservativeMeter(c *gin.Context) *deepSeekWSConservativeMeter {
	meter := &deepSeekWSConservativeMeter{
		forwarded:    make(map[string]int64),
		observedIDs:  make(map[string]map[string]struct{}),
		anonymous:    make(map[string]int64),
		snapshotAnon: make(map[string]int64),
	}
	if raw, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := raw.(*model.Channel); ok && channel != nil {
			meter.channel = &model.Channel{Id: channel.Id, Type: channel.Type, Config: channel.Config}
		}
	}
	return meter
}

type conservativeWSToolItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

func conservativeWSKind(raw string) string {
	_, canonical := tooling.ClassifyClaudeTool(raw)
	if canonical == tooling.BuiltinWebSearch || canonical == tooling.BuiltinWebFetch {
		return canonical
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "web_search_call":
		return tooling.BuiltinWebSearch
	case "web_fetch_call":
		return tooling.BuiltinWebFetch
	}
	return ""
}

func (m *deepSeekWSConservativeMeter) onForwardedCreate(frame []byte, target string) {
	if !deepseekcompat.HostUsesDeepSeekAPIContract(target) {
		return
	}
	var create struct {
		Type  string                   `json:"type"`
		Tools []openai.ResponseAPITool `json:"tools"`
	}
	if json.Unmarshal(frame, &create) != nil || create.Type != "response.create" {
		return
	}
	declared := make(map[string]struct{})
	for _, tool := range create.Tools {
		if name := conservativeWSKind(tool.Type); name != "" {
			declared[name] = struct{}{}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.target = target
	for name := range declared {
		m.forwarded[name]++
	}
}

func (m *deepSeekWSConservativeMeter) onUpstreamEvent(frame []byte) {
	var event struct {
		Type     string                 `json:"type"`
		ItemID   string                 `json:"item_id"`
		Item     conservativeWSToolItem `json:"item"`
		Response struct {
			Output []conservativeWSToolItem `json:"output"`
		} `json:"response"`
	}
	if json.Unmarshal(frame, &event) != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	observe := func(item conservativeWSToolItem, anonymous bool) {
		name := conservativeWSKind(item.Type)
		if name == "" {
			return
		}
		if item.ID == "" {
			if anonymous {
				m.anonymous[name]++
			}
			return
		}
		if m.observedIDs[name] == nil {
			m.observedIDs[name] = make(map[string]struct{})
		}
		m.observedIDs[name][item.ID] = struct{}{}
	}
	if event.Item.Type != "" && (event.Type == "response.output_item.added" ||
		event.Type == "response.output_item.done") {
		observe(event.Item, event.Type == "response.output_item.added")
	}
	if event.ItemID != "" {
		switch {
		case strings.HasPrefix(event.Type, "response.web_search_call."):
			observe(conservativeWSToolItem{ID: event.ItemID, Type: "web_search_call"}, false)
		case strings.HasPrefix(event.Type, "response.web_fetch_call."):
			observe(conservativeWSToolItem{ID: event.ItemID, Type: "web_fetch_call"}, false)
		}
	}
	anonymous := make(map[string]int64)
	for _, item := range event.Response.Output {
		if item.ID == "" {
			if name := conservativeWSKind(item.Type); name != "" {
				anonymous[name]++
			}
		}
		observe(item, false)
	}
	for name, n := range anonymous {
		m.snapshotAnon[name] = max(m.snapshotAnon[name], n)
	}
}

// settle uses already-validated channel tariffs to produce a single
// ToolUsageSummary/ToolsCost through the ordinary post-billing path.
func (m *deepSeekWSConservativeMeter) settle(c *gin.Context, meta *metalib.Meta, usage **relaymodel.Usage) {
	m.mu.Lock()
	target := m.target
	declared := make(map[string]int64, len(m.forwarded))
	observed := make(map[string]int, len(m.forwarded))
	for name, n := range m.forwarded {
		if n > math.MaxInt64/tooling.ConservativeDeepSeekDefaultCalls {
			declared[name] = math.MaxInt64
		} else {
			declared[name] = n * tooling.ConservativeDeepSeekDefaultCalls
		}
	}
	for name, ids := range m.observedIDs {
		num := int64(len(ids)) + max(m.anonymous[name], m.snapshotAnon[name])
		if num > math.MaxInt {
			num = math.MaxInt
		}
		observed[name] = int(num)
	}
	for name, num := range m.anonymous {
		if _, ok := observed[name]; !ok {
			observed[name] = int(max(num, m.snapshotAnon[name]))
		}
	}
	m.mu.Unlock()
	if target == "" || len(declared) == 0 {
		return
	}
	billMeta := *meta
	billMeta.BaseURL = target
	billMeta.Config.EndpointURLs = nil
	provider := resolvePricingAdaptor(&billMeta)
	requested := make(map[string]struct{}, len(declared))
	for name := range declared {
		requested[name] = struct{}{}
	}
	quotes, err := tooling.BuiltinToolQuotes(gmw.Ctx(c), billMeta.ActualModelName, &billMeta, m.channel, provider, requested)
	if err != nil {
		gmw.GetLogger(c).Error("DeepSeek WebSocket tool tariff missing after accepted work", zap.Error(err))
		return
	}
	tooling.RecordConservativeDeepSeekTools(c, &billMeta, quotes, declared)
	c.Set(ctxkey.ToolInvocationCounts, observed)
	tooling.ApplyBuiltinToolCharges(c, usage, &billMeta, m.channel, provider)
}
