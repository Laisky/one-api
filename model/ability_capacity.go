package model

import (
	"cmp"
	"context"
	"math/rand"
	"slices"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/identity"
)

// CountRetryCandidates counts the channels a retry walk may still reach for
// (group, model): every untried channel of the routing pool, that is, every
// enabled ability not in excluded, INCLUDING abilities that are cooling down
// under either deadline. The count is a ceiling for the retry budget, not an
// eligibility verdict: each retry selection still enforces both cooldown
// deadlines and ends the walk when nothing is eligible, so a cooling channel is
// never dispatched. Counting it keeps its slot for when its cooldown ends during
// the walk, which a long request can outlive. Counting the pool directly, rather
// than assuming the failed channel is still listed, also keeps a concurrently
// cooled failed channel from eating a slot. It reads the same source as routing:
// the memory cache snapshot when enabled, otherwise the abilities table.
//
// Parameters:
//   - ctx: lifecycle scope for the database count.
//   - group, model: the exact routing pool; model is the caller-facing name.
//   - excluded: channel IDs already tried by this request.
//
// Return values:
//   - int: the number of untried channels in the pool.
//   - error: a wrapped database failure on the database path.
func CountRetryCandidates(ctx context.Context, group, model string, excluded map[int]bool) (int, error) {
	if config.MemoryCacheEnabled {
		return countCachedRetryCandidates(group, model, excluded), nil
	}
	if DB == nil {
		return 0, errors.New("database not initialized")
	}
	var count int64
	if err := routingPoolQuery(DB.WithContext(ctx), group, model, excludedChannelIDs(excluded)).
		Distinct("channel_id").Count(&count).Error; err != nil {
		return 0, errors.Wrapf(err, "count retry candidates for group %q and model %q", group, model)
	}
	return int(count), nil
}

// countCachedRetryCandidates counts the cached routing pool of (group, model)
// without excluded, cooling abilities included (group2model2channels keeps
// them; selectors filter cooldowns at selection time).
//
// Parameters:
//   - group, model: the exact routing pool.
//   - excluded: channel IDs already tried by this request.
//
// Return values:
//   - int: the number of untried cached channels.
func countCachedRetryCandidates(group, model string, excluded map[int]bool) int {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	count := 0
	for _, channel := range group2model2channels[group][model] {
		if !excluded[channel.Id] {
			count++
		}
	}
	return count
}

// NoCapacityFloor selects channels without any capacity requirement.
const NoCapacityFloor int32 = 0

// ChannelModelCapacity returns the max_tokens the operator declared for model on
// channel, or 0 when none is declared (no model config, or max_tokens unset or
// non-positive). Zero means unknown/unbounded, never "zero capacity".
//
// Parameters:
//   - channel: the channel; nil has no declared capacity.
//   - model: the caller-facing model name.
//
// Return values:
//   - int32: the declared capacity, or 0 when undeclared.
func ChannelModelCapacity(channel *Channel, model string) int32 {
	if channel == nil {
		return 0
	}
	modelConfig := channel.GetModelConfig(model)
	if modelConfig == nil || modelConfig.MaxTokens <= 0 {
		return 0
	}
	return modelConfig.MaxTokens
}

// RaiseCapacityFloor returns the capacity floor after channel rejected the
// current request as too large (an upstream HTTP 413). A request too large for a
// declared capacity is too large for every capacity up to it, so the floor is
// the largest declared capacity that rejected the request. A rejection by a
// channel without a declared capacity proves nothing about other channels and
// keeps the floor.
//
// Parameters:
//   - floor: the current floor; NoCapacityFloor before any rejection.
//   - channel: the channel that answered 413.
//   - model: the caller-facing model name.
//
// Return values:
//   - int32: the new floor.
func RaiseCapacityFloor(floor int32, channel *Channel, model string) int32 {
	return max(floor, ChannelModelCapacity(channel, model))
}

// capacityExceedsFloor reports whether channel may serve a request that channels
// declaring up to floor tokens already rejected as too large: its declared
// capacity is larger than floor, or it declares none (an unknown limit is never
// proven too small). With NoCapacityFloor every channel qualifies.
//
// Parameters:
//   - channel: the candidate.
//   - model: the caller-facing model name.
//   - floor: the largest declared capacity that rejected the request.
//
// Return values:
//   - bool: true when the candidate stays eligible.
func capacityExceedsFloor(channel *Channel, model string, floor int32) bool {
	if floor <= NoCapacityFloor {
		return true
	}
	capacity := ChannelModelCapacity(channel, model)
	return capacity == 0 || capacity > floor
}

// filterByCapacityFloor keeps, in place and in order, the channels that may serve
// a request rejected at floor (see capacityExceedsFloor). The caller must own
// channels; it must not be a routing-cache snapshot.
//
// Parameters:
//   - channels: caller-owned candidates.
//   - model: the caller-facing model name.
//   - floor: the largest declared capacity that rejected the request.
//
// Return values:
//   - []*Channel: the eligible prefix of channels' backing array.
func filterByCapacityFloor(channels []*Channel, model string, floor int32) []*Channel {
	if floor <= NoCapacityFloor {
		return channels
	}
	kept := channels[:0]
	for _, channel := range channels {
		if capacityExceedsFloor(channel, model, floor) {
			kept = append(kept, channel)
		}
	}
	return kept
}

// excludedCapacityFloor returns the largest capacity declared for model by the
// channels in excluded, treating each of them as a 413 rejection. It backs the
// legacy tryLargerMaxTokens selector flag; the retry loop passes its own floor
// built from upstream 413s only. It reads the routing snapshot (cooling
// abilities included) when the memory cache is enabled, otherwise the channels
// table.
//
// Parameters:
//   - ctx: lifecycle scope for the database read.
//   - group, model: the exact routing pool; model is the caller-facing name.
//   - excluded: the channel IDs treated as rejections.
//
// Return values:
//   - int32: the derived floor, NoCapacityFloor when none declares a capacity.
//   - error: a wrapped database failure on the database path.
func excludedCapacityFloor(ctx context.Context, group, model string, excluded map[int]bool) (int32, error) {
	var rejected []*Channel
	if config.MemoryCacheEnabled {
		channelSyncLock.RLock()
		for _, channel := range group2model2channels[group][model] {
			if excluded[channel.Id] {
				rejected = append(rejected, channel)
			}
		}
		channelSyncLock.RUnlock()
	} else if len(excluded) > 0 {
		if DB == nil {
			return NoCapacityFloor, errors.New("database not initialized")
		}
		if err := DB.WithContext(ctx).Where("id IN ?", excludedChannelIDs(excluded)).Find(&rejected).Error; err != nil {
			return NoCapacityFloor, errors.Wrap(err, "load excluded channels for the capacity floor")
		}
	}
	floor := NoCapacityFloor
	for _, channel := range rejected {
		floor = RaiseCapacityFloor(floor, channel, model)
	}
	return floor, nil
}

// keepSmallestDeclaredCapacity implements DEFAULT_USE_MIN_MAX_TOKENS_MODEL: it
// orders channels in place by ascending declared capacity for model, with
// undeclared capacities (no model config, or max_tokens unset) last because they
// mean "no limit", and returns the end of the group that shares the smallest
// declared capacity. When no channel declares a capacity, or every channel
// declares the same one, it returns fallbackEnd.
//
// Parameters:
//   - channels: non-empty caller-owned candidates of one priority tier.
//   - model: the caller-facing model name.
//   - fallbackEnd: the end index to keep when no smaller group stands out.
//
// Return values:
//   - int: the exclusive end index of the channels to select from.
func keepSmallestDeclaredCapacity(channels []*Channel, model string, fallbackEnd int) int {
	type rankedChannel struct {
		channel  *Channel
		capacity int32
	}
	ranked := make([]rankedChannel, len(channels))
	for i, channel := range channels {
		ranked[i] = rankedChannel{channel: channel, capacity: ChannelModelCapacity(channel, model)}
	}
	slices.SortStableFunc(ranked, func(a, b rankedChannel) int {
		switch {
		case a.capacity == b.capacity:
			return 0
		case a.capacity == 0:
			return 1
		case b.capacity == 0:
			return -1
		}
		return cmp.Compare(a.capacity, b.capacity)
	})
	for i := range ranked {
		channels[i] = ranked[i].channel
	}
	smallest := ranked[0].capacity
	if smallest == 0 {
		return fallbackEnd
	}
	for i := range ranked {
		if ranked[i].capacity != smallest {
			return i
		}
	}
	return fallbackEnd
}

// getChannelAboveCapacityFromDB is the database-path form of the capacity-aware
// selection that CacheGetRandomSatisfiedChannelAboveCapacityWithContext applies
// on the memory path: among the routable channels not yet tried (both cooldown
// deadlines enforced by availableAbilitiesQuery), it keeps those that may serve
// a request already rejected at capacityFloor, then selects from the highest
// remaining priority tier (or, with ignoreFirstPriority, strictly lower tiers)
// by weight.
//
// Parameters:
//   - ctx: lifecycle scope for the database reads.
//   - group, model: the exact routing pool; model is the caller-facing name.
//   - ignoreFirstPriority: select strictly below the highest remaining tier.
//   - excluded: channel IDs already tried or skipped.
//   - capacityFloor: the largest declared capacity that rejected the request;
//     it must be positive.
//
// Return values:
//   - *Channel: the selected channel.
//   - error: a configuration error whose text keeps the selector's exhaustion
//     wording when no channel qualifies, or a wrapped database failure.
func getChannelAboveCapacityFromDB(ctx context.Context, group, model string, ignoreFirstPriority bool, excluded map[int]bool, capacityFloor int32) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	excludeIDs := excludedChannelIDs(excluded)
	var eligibleIDs []int
	if err := availableAbilitiesQuery(DB.WithContext(ctx), group, model, time.Now().UTC(), excludeIDs).
		Pluck("channel_id", &eligibleIDs).Error; err != nil {
		return nil, errors.Wrap(err, "load capacity retry candidate channel ids")
	}
	if len(eligibleIDs) == 0 {
		return nil, errkind.ConfigErr(noSatisfiedChannelError(group, model, excludeIDs))
	}
	var candidates []*Channel
	if err := DB.WithContext(ctx).Where("id IN ?", eligibleIDs).Find(&candidates).Error; err != nil {
		return nil, errors.Wrap(err, "load capacity retry candidate channels")
	}
	larger := filterByCapacityFloor(candidates, model, capacityFloor)
	if len(larger) == 0 {
		return nil, errkind.ConfigErr(errors.Errorf(
			"no available channels support model %s after exclusions (capacity above %d tokens)", model, capacityFloor))
	}

	tier, err := selectPriorityTier(larger, ignoreFirstPriority)
	if err != nil {
		return nil, err
	}
	channel := pickWeightedChannel(tier)
	if channel == nil {
		// Every candidate weight is zero: fall back to uniform random.
		channel = tier[rand.Intn(len(tier))]
	}
	if !channel.SupportsModel(model) {
		return nil, identity.Tag(
			errors.Errorf("channel #%d does not list support for model %s", channel.Id, model),
			channel.Ref())
	}
	return channel, nil
}

// selectPriorityTier returns the channels of the highest priority tier, or with
// ignoreFirstPriority the channels strictly below it, matching the excluding
// selectors' tier contract. It sorts channels in place by descending priority.
//
// Parameters:
//   - channels: a non-empty caller-owned candidate slice.
//   - ignoreFirstPriority: skip the highest tier.
//
// Return values:
//   - []*Channel: the selected tier, never empty on success.
//   - error: a configuration error when ignoreFirstPriority finds no lower tier.
func selectPriorityTier(channels []*Channel, ignoreFirstPriority bool) ([]*Channel, error) {
	slices.SortStableFunc(channels, func(a, b *Channel) int {
		return cmp.Compare(b.GetPriority(), a.GetPriority())
	})
	boundary := len(channels)
	for i, channel := range channels {
		if channel.GetPriority() != channels[0].GetPriority() {
			boundary = i
			break
		}
	}
	if !ignoreFirstPriority {
		return channels[:boundary], nil
	}
	if boundary == len(channels) {
		return nil, errkind.ConfigErr(errors.New("no lower priority channels available after excluding failed channels"))
	}
	return channels[boundary:], nil
}
