package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// chatSurfaceEndpointNames lists the client-facing endpoints that make a channel
// eligible for the live chat health probe.
//
// These are the three chat-shaped surfaces the gateway exposes. A channel that
// serves any of them can be probed, because the probe's wire format is not the
// operator's choice: each adaptor translates the gateway's internal chat request
// into whatever its upstream speaks, and derives the upstream URL from the
// channel type. A Claude-only channel (channeltype.Anthropic or ClaudeCompatible)
// therefore still receives its probe at /v1/messages, and an OpenAI-compatible
// channel receives it at /v1/chat/completions -- from the same probe.
//
// Endpoints absent from this list (embeddings, rerank, moderations, audio, image,
// video, ocr, realtime, voice_clone) describe non-chat work that a chat probe
// cannot represent. A channel serving only those is skipped, never failed.
var chatSurfaceEndpointNames = []string{
	chatCompletionsTestTarget,
	responseAPITestTarget,
	claudeMessagesTestTarget,
}

// channelTestNotApplicableError reports that a channel cannot be probed at all,
// as distinct from having been probed and failed.
//
// An active health probe only carries information when a request representative
// of what the channel actually serves can be constructed. An embeddings-only,
// rerank-only or translation-only channel exposes no chat surface, so a chat probe
// says nothing about whether that channel is healthy. Reporting it as a failure is
// what caused issue #400: embeddings-only channels were auto-disabled by the
// periodic sweep even though they were serving traffic normally.
//
// Callers must route this outcome to "skipped" and must never let it reach the
// auto-disable or failure-notification paths.
type channelTestNotApplicableError struct {
	reason string
}

// Error implements the error interface.
// Returns: the human-readable reason the channel cannot be probed.
func (e *channelTestNotApplicableError) Error() string {
	return e.reason
}

// errChannelTestNotApplicable builds a not-applicable outcome carrying a formatted reason.
// Parameters: format and args describe why no probe can be constructed.
// Returns: an error that isChannelTestNotApplicable recognizes.
func errChannelTestNotApplicable(format string, args ...any) error {
	return errors.WithStack(&channelTestNotApplicableError{reason: fmt.Sprintf(format, args...)})
}

// isChannelTestNotApplicable reports whether err marks a channel as unprobeable.
// Parameters: err is any error returned by channel test-plan resolution.
// Returns: true when the channel should be skipped rather than judged.
func isChannelTestNotApplicable(err error) bool {
	var target *channelTestNotApplicableError
	return errors.As(err, &target)
}

// channelHasChatSurface reports chat-probe eligibility using background context.
// Parameters: channel is the channel to probe.
// Returns: true when the channel exposes at least one chat-shaped endpoint.
func channelHasChatSurface(channel *model.Channel) bool {
	return channelHasChatSurfaceWithContext(context.Background(), channel)
}

// channelHasChatSurfaceWithContext reports whether a channel exposes any chat-shaped
// endpoint, honouring the administrator's configuration before the channel type's
// defaults.
//
// Parameters: ctx carries request logging and channel supplies the endpoint configuration.
// Returns: true when the channel is eligible for the live chat health probe.
func channelHasChatSurfaceWithContext(ctx context.Context, channel *model.Channel) bool {
	if channel == nil {
		return false
	}

	endpoints := channel.GetSupportedEndpointsWithContext(ctx)
	if len(endpoints) == 0 {
		endpoints = channeltype.DefaultEndpointNamesForChannelType(channel.Type)
	}

	for _, name := range chatSurfaceEndpointNames {
		if channeltype.IsEndpointSupportedByName(name, endpoints) {
			return true
		}
	}
	return false
}

// chatSurfaceEndpointList renders the eligible endpoint names for diagnostics.
// Returns: the chat-shaped endpoint names joined for a human-readable message.
func chatSurfaceEndpointList() string {
	return strings.Join(chatSurfaceEndpointNames, ", ")
}
