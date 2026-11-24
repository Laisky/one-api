package controller

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// loadChannelCached loads a channel once and reuses it from the provided cache.
//
// Failures are memoized as a nil entry. Channel.Delete is not transactional, so
// orphaned ability rows are a real state; without negative memoization one
// deleted channel costs a failed query per ability it used to back, on every
// request that lists models.
func loadChannelCached(channelID int, cache map[int]*model.Channel) (*model.Channel, error) {
	if channelID == 0 {
		return nil, errors.New("channel id is required")
	}
	if channel, ok := cache[channelID]; ok {
		if channel == nil {
			return nil, errors.Errorf("channel %d is not resolvable", channelID)
		}
		return channel, nil
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		cache[channelID] = nil
		return nil, errors.Wrapf(err, "load channel %d", channelID)
	}
	cache[channelID] = channel
	return channel, nil
}

// isVisibleAbilityModel reports whether the ability's model remains publicly visible after hidden-model filtering.
func isVisibleAbilityModel(ability dto.EnabledAbility, cache map[int]*model.Channel) bool {
	modelName := strings.TrimSpace(ability.Model)
	if modelName == "" {
		return false
	}
	channel, err := loadChannelCached(ability.ChannelId, cache)
	if err != nil {
		return false
	}
	return !channel.IsModelHidden(modelName)
}

// filterVisibleAbilities removes stale or hidden ability rows from public model responses.
func filterVisibleAbilities(abilities []dto.EnabledAbility, cache map[int]*model.Channel) []dto.EnabledAbility {
	visible := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		if !isVisibleAbilityModel(ability, cache) {
			continue
		}
		visible = append(visible, ability)
	}
	return visible
}

// filterAbilitiesByTokenAllowList narrows abilities to what the calling API token
// is permitted to invoke.
//
// Group abilities answer "what can this deployment serve for this user", which is
// a coarser question than "what may this key call". TokenAuth enforces the token's
// own allow-list on every request and 403s anything outside it, so a listing that
// skipped this filter advertised models the caller would then be refused.
//
// The membership test is middleware.IsModelInList -- the very predicate that
// produces the 403 -- so discovery and invocability cannot disagree. The TOKEN
// entry is matched raw (that helper does not trim), so an allow-list stored as
// "a, b" genuinely does not permit "b" and the listing reflects that rather than
// hiding it. The ABILITY name is trimmed because the trimmed form is the id
// actually advertised, and therefore the string a client sends back; see
// bestAbilityPerModel, which trims identically.
//
// An unrestricted token has no ctxkey.AvailableModels entry at all (TokenAuth sets
// it only for a non-empty Token.Models), so the absent case must return everything
// -- returning an empty list there would blank the catalog for most callers.
func filterAbilitiesByTokenAllowList(c *gin.Context, abilities []dto.EnabledAbility) []dto.EnabledAbility {
	raw, restricted := c.Get(ctxkey.AvailableModels)
	if !restricted {
		return abilities
	}
	allowList, ok := raw.(string)
	if !ok {
		// Only TokenAuth writes this key, and always as a string. Reaching here
		// means something else clobbered it; the relay would still enforce the
		// allow-list, so failing open would advertise models that then 403.
		gmw.GetLogger(c).Warn("ctxkey.AvailableModels is not a string; token allow-list filter skipped")
		return abilities
	}
	if allowList == "" {
		return abilities
	}

	permitted := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		if middleware.IsModelInList(strings.TrimSpace(ability.Model), allowList) {
			permitted = append(permitted, ability)
		}
	}
	return permitted
}

// ListModels lists all models available to the user.
func ListModels(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)

	var userGroup string
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			userGroup = u.Group
		}
	}
	if userGroup == "" {
		var err error
		userGroup, err = model.CacheGetUserGroup(ctx, userId)
		if err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, err)
			return
		}
	}

	availableAbilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}
	channelCache := make(map[int]*model.Channel)
	availableAbilities = filterVisibleAbilities(availableAbilities, channelCache)
	availableAbilities = filterAbilitiesByTokenAllowList(c, availableAbilities)

	respondModelList(c, resolveUserAvailableModels(availableAbilities, channelCache, lg))
}

// abilityChannelRef resolves the log identity of the channel backing an ability.
//
// It reads only the caller-supplied channel cache, never the database, because it
// runs inside the /v1/models listing loop. A cache miss degrades to an id-only
// reference so an operator's `channel_id=` grep still matches.
//
// Parameters:
//   - channelCache: channel snapshot keyed by channel id; may be nil.
//   - channelID: the ability's channel primary key.
//
// Return values:
//   - identity.ChannelRef: fullest reference resolvable without I/O.
func abilityChannelRef(channelCache map[int]*model.Channel, channelID int) identity.ChannelRef {
	if ch := channelCache[channelID]; ch != nil {
		return ch.Ref()
	}
	return identity.NewChannelRef(channelID, "", "")
}

// bestAbilityPerModel collapses the one-row-per-(model, channel) ability list to a
// single representative channel per exact model name, ranked the way routing
// ranks channels: highest priority first, lowest channel id as the tie-break.
//
// This is what makes a listing's owned_by honest. The same model id is routinely
// served by several channels -- open-weight ids hosted by many upstreams, and
// two-brand providers such as Zhipu (open.bigmodel.cn) and Z.ai (api.z.ai) whose
// catalogs are near-identical -- so without an explicit rank the attribution
// would fall out of row order or map iteration and move between restarts.
//
// Keyed by the exact model name because routing is case-sensitive: two abilities
// differing only in case are distinct routing keys (see issue #352).
func bestAbilityPerModel(abilities []dto.EnabledAbility) map[string]dto.EnabledAbility {
	best := make(map[string]dto.EnabledAbility, len(abilities))
	for _, ability := range abilities {
		modelName := strings.TrimSpace(ability.Model)
		if modelName == "" {
			continue
		}
		if current, ok := best[modelName]; ok && !ability.Beats(current) {
			continue
		}
		best[modelName] = ability
	}
	return best
}

// resolveUserAvailableModels converts a user group's enabled abilities into
// OpenAI-shaped model entries.
//
// Every field is derived from the abilities and their channels; the compiled-in
// adaptor catalog is deliberately not consulted. That catalog describes what this
// binary knows how to talk to, not what this deployment can serve, so borrowing
// from it produced two defects: a gateway running only a Zhipu channel reported
// glm-4.7 as owned by Z.ai (both adaptors ship the id), and an id whose catalog
// entry used a different casing could be advertised in a form channel routing
// cannot match (issue #352). Building from the ability makes the first impossible
// to get wrong and the second structurally impossible: Id/Root are the ability's
// own case-sensitive routing key by construction.
//
// The three remaining fields are constants or channel-derived: owned_by comes
// from the channel chosen by bestAbilityPerModel, created is the frozen
// modelCatalogCreated, and permission is defaultModelPermissions.
//
// Entries are keyed by the exact ability model name: two abilities that differ
// only in case are distinct routing keys and must both remain listable, while
// identical names collapse to a single entry.
func resolveUserAvailableModels(abilities []dto.EnabledAbility, channelCache map[int]*model.Channel, lg glog.Logger) []OpenAIModels {
	allowed := make(map[string]OpenAIModels, len(abilities))
	for modelName, ability := range bestAbilityPerModel(abilities) {
		if entry, ok := buildModelEntryFromAbility(modelName, ability.ChannelId, ability.ChannelType, channelCache); ok {
			allowed[modelName] = entry
			continue
		}
		if lg != nil {
			// The ability's channel is not the request's own channel, so name it
			// explicitly; the cache is already in hand, so this costs no query.
			lg.Debug("unable to build model entry for ability",
				abilityChannelRef(channelCache, ability.ChannelId).AppendZap([]zap.Field{
					zap.String("model", modelName),
					zap.Int("channel_type", ability.ChannelType),
				})...)
		}
	}

	userAvailableModels := make([]OpenAIModels, 0, len(allowed))
	for _, entry := range allowed {
		userAvailableModels = append(userAvailableModels, entry)
	}

	sort.Slice(userAvailableModels, func(i, j int) bool {
		return userAvailableModels[i].Id < userAvailableModels[j].Id
	})

	return userAvailableModels
}

// abilityOwnerFromCache resolves the owned_by label for the channel backing an
// ability WITHOUT any database access, so it is safe inside the /v1/models
// listing loop (same constraint as abilityChannelRef).
//
// The cached channel's own type wins when available; otherwise the ability's
// channel_type is authoritative on its own, because GetGroupModelsV2 reads it by
// joining channels. An unnamed or unknown type degrades to "channel-<uuid>" when
// the cached channel carries its external UUID, and to "unknown" otherwise; the
// internal integer id never reaches the public catalog and this path stays
// I/O-free.
func abilityOwnerFromCache(channelID int, channelType int, cache map[int]*model.Channel) string {
	owner := channeltype.IdToName(channelType)
	if channelID > 0 {
		if channel, ok := cache[channelID]; ok && channel != nil {
			owner = channeltype.IdToName(channel.Type)
			if owner == "" || owner == "unknown" {
				owner = channelUUIDOwner(channel.UUID)
			}
		}
	}
	if owner == "" || owner == "unknown" {
		return "unknown"
	}
	return owner
}

// channelUUIDOwner builds the owned_by fallback label for a channel whose type
// has no display name.
// Parameters:
//   - channelUUID: the channel's external UUID; may be empty when not backfilled.
//
// Return values:
//   - string: "channel-<uuid>" or "unknown" when the UUID is empty.
func channelUUIDOwner(channelUUID string) string {
	channelUUID = strings.TrimSpace(channelUUID)
	if channelUUID == "" {
		return "unknown"
	}
	return "channel-" + channelUUID
}

// buildModelEntryFromAbility renders one ability as an OpenAI model entry.
//
// It performs no I/O: callers run it once per advertised model, and every caller
// has already populated the channel cache via filterVisibleAbilities.
func buildModelEntryFromAbility(modelName string, channelID int, channelType int, cache map[int]*model.Channel) (OpenAIModels, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return OpenAIModels{}, false
	}

	owner := abilityOwnerFromCache(channelID, channelType, cache)

	return OpenAIModels{
		Id:         modelName,
		Object:     "model",
		Created:    modelCatalogCreated,
		OwnedBy:    owner,
		Permission: defaultModelPermissions,
		Root:       modelName,
		Parent:     nil,
	}, true
}

// matchVisibleAbilityByModelID selects an ability for a requested model ID. The
// exact trimmed routing ID wins; otherwise, the lexicographically smallest
// case-folded match is returned. Within one model name the channel is chosen the
// way routing chooses it -- highest priority, then lowest channel id -- so the
// metadata reported for a model served by several channels matches what
// /v1/models reports for the same model.
func matchVisibleAbilityByModelID(abilities []dto.EnabledAbility, modelID string) (dto.EnabledAbility, bool) {
	modelID = strings.TrimSpace(modelID)
	candidates := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		ability.Model = strings.TrimSpace(ability.Model)
		if ability.Model == "" || !strings.EqualFold(ability.Model, modelID) {
			continue
		}
		candidates = append(candidates, ability)
	}
	if len(candidates) == 0 {
		return dto.EnabledAbility{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Model != candidates[j].Model {
			return candidates[i].Model < candidates[j].Model
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if candidates[i].ChannelId != candidates[j].ChannelId {
			return candidates[i].ChannelId < candidates[j].ChannelId
		}
		return candidates[i].ChannelType < candidates[j].ChannelType
	})
	for _, candidate := range candidates {
		if candidate.Model == modelID {
			return candidate, true
		}
	}
	return candidates[0], true
}

// RetrieveModel returns details about a specific model or an error when it does not exist.
func RetrieveModel(c *gin.Context) {
	// Scoped to the caller's group and key, so it must not be shared by a cache.
	setPrivateCatalogHeaders(c)
	modelId := strings.TrimSpace(c.Param("model"))
	ctx, userGroup, err := getRequestUserGroup(c)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}
	abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}
	channelCache := make(map[int]*model.Channel)
	visibleAbilities := filterVisibleAbilities(abilities, channelCache)
	// A model outside the token's allow-list is reported as not found rather than
	// described: the caller cannot invoke it, so advertising its metadata would
	// both mislead and disclose part of the group catalog the key has no access to.
	visibleAbilities = filterAbilitiesByTokenAllowList(c, visibleAbilities)
	matched, ok := matchVisibleAbilityByModelID(visibleAbilities, modelId)
	if !ok {
		respondModelNotFound(c, modelId)
		return
	}
	modelId = matched.Model

	// modelId was rebound above to the ability's actual casing, the case-sensitive
	// routing key /v1/chat/completions matches (issue #352). The entry is built
	// from that ability rather than looked up in the compiled-in catalog, so this
	// endpoint reports exactly what GET /v1/models reports for the same model --
	// same id, same owner, same created -- and cannot drift from it.
	if entry, ok := buildModelEntryFromAbility(modelId, matched.ChannelId, matched.ChannelType, channelCache); ok {
		c.JSON(http.StatusOK, entry)
		return
	}
	respondModelNotFound(c, modelId)
}

// GetUserAvailableModels lists the model identifiers the authenticated user can access.
func GetUserAvailableModels(c *gin.Context) {
	ctx, userGroup, err := getRequestUserGroup(c)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	models, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	channelCache := make(map[int]*model.Channel)
	models = filterVisibleAbilities(models, channelCache)

	modelNames := make([]string, 0)
	modelsMap := map[string]bool{}
	for _, model := range models {
		modelsMap[model.Model] = true
	}
	for modelName := range modelsMap {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    modelNames,
	})
}

// intersectTokenModelIDs intersects a token's configured model IDs with visible
// abilities using exact trimmed routing IDs. It returns each canonical ability ID
// once, preserving the token configuration order.
func intersectTokenModelIDs(modelsString string, abilities []dto.EnabledAbility) []string {
	routingIDs := make(map[string]string, len(abilities))
	for _, ability := range abilities {
		modelName := strings.TrimSpace(ability.Model)
		if modelName == "" {
			continue
		}
		routingIDs[modelName] = modelName
	}

	// Membership is delegated to middleware.IsModelInList -- the same call the relay
	// makes before it 403s -- so this endpoint cannot advertise a model the caller
	// would be refused. It used to TrimSpace each entry, which made an allow-list
	// stored as "a, b" advertise "b" while the relay refused it.
	//
	// Iteration stays over the CSV so the caller's own ordering is preserved.
	tokenModels := strings.Split(modelsString, ",")
	modelNames := make([]string, 0, len(tokenModels))
	seen := make(map[string]struct{}, len(tokenModels))
	for _, rawModel := range tokenModels {
		canonicalModel, ok := routingIDs[rawModel]
		if !ok || !middleware.IsModelInList(canonicalModel, modelsString) {
			continue
		}
		if _, ok := seen[canonicalModel]; ok {
			continue
		}
		seen[canonicalModel] = struct{}{}
		modelNames = append(modelNames, canonicalModel)
	}
	return modelNames
}

// GetAvailableModelsByToken reports the models allowed for the current API token when explicitly restricted.
func GetAvailableModelsByToken(c *gin.Context) {
	lg := gmw.GetLogger(c)
	// Get token information to determine status
	tokenID := c.GetInt(ctxkey.TokenId)
	userID := c.GetInt(ctxkey.Id)
	token, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
			"data": gin.H{
				"available": nil,
				"enabled":   false,
			},
		})
		return
	}

	// Determine if token is enabled
	statusToken := token.Status == model.TokenStatusEnabled

	// Check if the token has specific model restrictions
	if availableModels, exists := c.Get(ctxkey.AvailableModels); exists {
		// Token has model restrictions, use those models. The assertion is checked:
		// an unchecked one would panic the handler on the same corrupted-context
		// state that filterAbilitiesByTokenAllowList merely logs.
		modelsString, _ := availableModels.(string)
		if modelsString != "" {
			ctx, userGroup, err := getRequestUserGroup(c)
			if err != nil {
				lg.Warn("failed to load user group for available models", zap.Int("user_id", userID), zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": err.Error(),
					"data": gin.H{
						"available": nil,
						"enabled":   false,
					},
				})
				return
			}
			abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": err.Error(),
					"data": gin.H{
						"available": nil,
						"enabled":   false,
					},
				})
				return
			}
			channelCache := make(map[int]*model.Channel)
			visibleAbilities := filterVisibleAbilities(abilities, channelCache)
			modelNames := intersectTokenModelIDs(modelsString, visibleAbilities)
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"available": modelNames,
					"enabled":   statusToken,
				},
			})
			return
		}
	}

	// Token has no model restrictions, return error instead of fallback
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "the token has no available models",
		"data": gin.H{
			"available": nil,
			"enabled":   statusToken,
		},
	})
}
