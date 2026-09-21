package controller

import (
	"reflect"
	"strings"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

var knownChatRequestFields = collectChatProtocolFields()

// collectChatProtocolFields derives the typed chat protocol keys once at startup.
// Parameters: none. Returns: an immutable set used to distinguish deliberately
// removed protocol fields from unknown extensions during passthrough merging.
func collectChatProtocolFields() map[string]struct{} {
	typ := reflect.TypeOf(relaymodel.GeneralOpenAIRequest{})
	fields := make(map[string]struct{}, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name := strings.SplitN(typ.Field(i).Tag.Get("json"), ",", 2)[0]
		if name != "" && name != "-" {
			fields[name] = struct{}{}
		}
	}
	return fields
}

// isKnownChatRequestField identifies a protocol field represented by the DTO.
// Parameters: name is an exact JSON key. Returns: true for known fields so a
// converter's deletion cannot be undone by the unknown-extension merge.
func isKnownChatRequestField(name string) bool {
	_, known := knownChatRequestFields[name]
	return known
}
