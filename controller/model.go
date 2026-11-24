package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	relay "github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// https://platform.openai.com/docs/api-reference/models/list

type OpenAIModelPermission struct {
	Id                 string  `json:"id"`
	Object             string  `json:"object"`
	Created            int     `json:"created"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	Organization       string  `json:"organization"`
	Group              *string `json:"group"`
	IsBlocking         bool    `json:"is_blocking"`
}

type OpenAIModels struct {
	// Id model's name
	//
	// BUG: Different channels may have the same model name
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	// OwnedBy is the channel's adaptor name
	OwnedBy    string                  `json:"owned_by"`
	Permission []OpenAIModelPermission `json:"permission"`
	Root       string                  `json:"root"`
	Parent     *string                 `json:"parent"`
}

// BUG(#39): 更新 custom channel 时，应该同步更新所有自定义的 models 到 allModels
var (
	// modelCatalogCreated is the frozen `created` timestamp every catalog entry
	// carries. It must stay a constant: the supported-models snapshot is rebuilt
	// whenever the enabled-channel signature changes, and because a billing write
	// bumps channels.updated_at that happens on essentially every relayed request.
	// A time.Now() here would make `created` churn on every rebuild.
	modelCatalogCreated = 1626777600

	allModels []OpenAIModels
	modelsMap map[string]OpenAIModels
	// channelId2Models is the raw provider catalog per channel type: every
	// model the adaptor recognizes, including models that need an operator
	// tariff. GET /api/models serves it and resets validate references against it.
	channelId2Models map[int][]string
	// channelId2DefaultModels holds the zero-config recommendations per channel
	// type: the catalog minus models that are not servable without operator
	// pricing. GET /api/models/defaults serves it and resets write it.
	channelId2DefaultModels map[int][]string
	defaultModelPermissions []OpenAIModelPermission
)

// Anonymous models display cache (1-minute TTL) to avoid repeated heavy loads.
// Keyed by normalized keyword filter.
var (
	anonymousModelsDisplayCache = gutils.NewExpCache[map[string]ChannelModelsDisplayInfo](context.Background(), time.Minute)
	anonymousModelsDisplayGroup singleflight.Group
)

func init() {
	var permission []OpenAIModelPermission
	permission = append(permission, OpenAIModelPermission{
		Id:                 "modelperm-LwHkVFn8AcMItP432fKKDIKJ",
		Object:             "model_permission",
		Created:            modelCatalogCreated,
		AllowCreateEngine:  true,
		AllowSampling:      true,
		AllowLogprobs:      true,
		AllowSearchIndices: false,
		AllowView:          true,
		AllowFineTuning:    false,
		Organization:       "*",
		Group:              nil,
		IsBlocking:         false,
	})
	defaultModelPermissions = append([]OpenAIModelPermission(nil), permission...)
	// https://platform.openai.com/docs/models/model-endpoint-compatibility
	for i := range apitype.Dummy {
		if i == apitype.AIProxyLibrary {
			continue
		}
		// apitype.Azure's adaptor advertises the entire OpenAI catalog plus the
		// Foundry Claude models, which are already emitted under apitype.OpenAI and
		// apitype.Anthropic respectively. Skip it here to avoid duplicate rows (and
		// the "openai"-owner mislabel from a non-Init'd adaptor). The Azure channel
		// still lists both families via channelId2Models below and the DB-channel
		// pass in listAllSupportedModels. Mirrors the channeltype.Azure skips in the
		// CompatibleChannels loop below and in openrouter_provider.go.
		if i == apitype.Azure {
			continue
		}
		adaptor := relay.GetAdaptor(i)
		if adaptor == nil {
			continue
		}

		channelName := adaptor.GetChannelName()
		modelNames := adaptor.GetModelList()
		for _, modelName := range modelNames {
			allModels = append(allModels, OpenAIModels{
				Id:         modelName,
				Object:     "model",
				Created:    modelCatalogCreated,
				OwnedBy:    channelName,
				Permission: permission,
				Root:       modelName,
				Parent:     nil,
			})
		}
	}
	for _, channelType := range openai.CompatibleChannels {
		if channelType == channeltype.Azure {
			continue
		}
		channelName, channelModelList := openai.GetCompatibleChannelMeta(channelType)
		for _, modelName := range channelModelList {
			allModels = append(allModels, OpenAIModels{
				Id:         modelName,
				Object:     "model",
				Created:    modelCatalogCreated,
				OwnedBy:    channelName,
				Permission: permission,
				Root:       modelName,
				Parent:     nil,
			})
		}
	}
	allModels = dedupeStaticModelsByOwner(allModels)
	modelsMap = make(map[string]OpenAIModels)
	for _, model := range allModels {
		modelsMap[model.Id] = model
	}
	channelId2Models = make(map[int][]string)
	channelId2DefaultModels = make(map[int][]string)
	for i := 1; i < channeltype.Dummy; i++ {
		adaptor := relay.GetAdaptor(channeltype.ToAPIType(i))
		if adaptor == nil {
			continue
		}

		meta := &meta.Meta{
			ChannelType: i,
		}
		adaptor.Init(meta)
		catalog := adaptor.GetModelList()
		channelId2Models[i] = catalog
		// The editor fills new channels and resets write this list, so it
		// holds only models that are servable without operator pricing.
		channelId2DefaultModels[i] = zeroConfigServableModels(i, catalog)
	}
}

// dedupeStaticModelsByOwner collapses the aggregated static catalog to exactly
// one row per model id.
//
// Many providers legitimately serve the same model id -- open-weight ids such as
// Qwen/Qwen3.5-9B are hosted by several upstreams at once, Claude ids are served
// by both the Anthropic and AWS adaptors, and the Zhipu (open.bigmodel.cn) and
// Zai (api.z.ai) channels are two brands of the same GLM catalog. The aggregation
// loops above append unconditionally, so without this pass /v1/models emits a
// duplicate row per extra provider, and the modelsMap built from it resolves the
// owner by whichever adaptor happened to be swept last.
//
// This is the FALLBACK ranking, used only where no channel is available to ask:
// allModels is built at init() from the compiled-in adaptors, long before the
// database is readable. Wherever channels exist the owner comes from them instead
// -- listAllSupportedModels ranks enabled channels ahead of this catalog, and
// /v1/models resolves the owner from the ability's own channel (bestAbilityPerModel).
// Here the winner is simply the smallest OwnedBy in byte order, with the original
// position as a stable tie-break, so the catalog is at least deterministic rather
// than dependent on the apitype iota or map iteration order.
//
// Billing is unaffected either way: quota resolves per request through the
// channel's own apitype and price table, so glm-4.7 on a Zhipu channel bills at
// BigModel's CNY tiers while the same id on a Zai channel bills at Z.AI's flat USD
// rate, regardless of which one owns the row in any listing.
func dedupeStaticModelsByOwner(models []OpenAIModels) []OpenAIModels {
	winners := make(map[string]int, len(models))
	for i, m := range models {
		prev, seen := winners[m.Id]
		if !seen || m.OwnedBy < models[prev].OwnedBy {
			winners[m.Id] = i
		}
	}
	deduped := make([]OpenAIModels, 0, len(winners))
	for i, m := range models {
		if winners[m.Id] == i {
			deduped = append(deduped, m)
		}
	}
	return deduped
}

// DashboardListModels returns the complete channel-type-to-model catalog for
// administrative dashboards: every model each provider recognizes, including
// models that need an operator tariff before they are servable. Zero-config
// recommendations are served separately by DashboardListDefaultModels.
// Parameters: c is the admin request. Returns: none; it writes the envelope.
func DashboardListModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channelId2Models,
	})
}

type listAllModelsCacheEntry struct {
	Models  []OpenAIModels
	Version string
}

// cachedListAllModels is a short-term cache for ListAllModels to reduce load.
var cachedListAllModels = gutils.NewSingleItemExpCache[listAllModelsCacheEntry](time.Minute)

// ListAllModels returns every known model in the OpenAI-compatible format regardless of user permissions.
func ListAllModels(c *gin.Context) {
	models, err := getSupportedModelsSnapshotWithContext(gmw.Ctx(c))
	if err != nil {
		middleware.AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "load supported models"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

// getSupportedModelsSnapshotWithContext returns the cached supported-model
// snapshot, rebuilding it with request-correlated diagnostics when stale.
// Parameters: ctx carries cancellation and logging values for snapshot loading.
// Returns: the supported models or a wrapped loading error.
func getSupportedModelsSnapshotWithContext(ctx context.Context) ([]OpenAIModels, error) {
	version, err := model.GetEnabledChannelsVersionSignature()
	if err != nil {
		return nil, errors.Wrap(err, "channels version signature")
	}

	if entry, ok := cachedListAllModels.Get(); ok && entry.Version == version {
		return entry.Models, nil
	}

	models, err := listAllSupportedModels(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list models")
	}

	cachedListAllModels.Set(listAllModelsCacheEntry{
		Models:  models,
		Version: version,
	})

	return models, nil
}

// getRequestUserGroup returns the request context and resolved user group for authenticated model endpoints.
func getRequestUserGroup(c *gin.Context) (context.Context, string, error) {
	ctx := gmw.Ctx(c)
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if user, ok := userObj.(*model.User); ok {
			group := strings.TrimSpace(user.Group)
			if group != "" {
				return ctx, group, nil
			}
		}
	}
	group, err := model.CacheGetUserGroup(ctx, c.GetInt(ctxkey.Id))
	if err != nil {
		return ctx, "", errors.Wrap(err, "cache get user group")
	}
	return ctx, group, nil
}

// respondModelNotFound returns the OpenAI-compatible model-not-found error payload.
//
// HTTP 404, matching OpenAI: SDKs key their NotFoundError on the status, so a 200
// carrying an error body is parsed as a successful model descriptor and surfaces
// as a confusing decode failure rather than a clean "model not found".
//
// The same payload is returned whether the model does not exist, is hidden on its
// channel, or is outside the calling key's allow-list. That is deliberate -- three
// distinguishable responses would let any key enumerate the deployment's catalog --
// which is also why the message follows OpenAI in saying "or you do not have access
// to it" rather than asserting the model does not exist.
func respondModelNotFound(c *gin.Context, modelID string) {
	msg := fmt.Sprintf("The model '%s' does not exist or you do not have access to it", modelID)
	respErr := relaymodel.Error{Message: msg, Type: relaymodel.ErrorTypeInvalidRequest, Param: "model", Code: "model_not_found", RawError: errors.New(msg)}
	c.JSON(http.StatusNotFound, gin.H{
		"error": respErr,
	})
}
