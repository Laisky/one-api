package validator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/model"
)

// GetKnownParameters extracts all valid JSON parameter names from GeneralOpenAIRequest struct
func GetKnownParameters() map[string]bool {
	knownParams := make(map[string]bool)

	// Get the struct type
	requestType := reflect.TypeOf(model.GeneralOpenAIRequest{})

	// Iterate through all fields
	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i)

		// Get the JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Parse the JSON tag (format: "name,omitempty" or just "name")
		tagParts := strings.Split(jsonTag, ",")
		if len(tagParts) > 0 && tagParts[0] != "" {
			paramName := tagParts[0]
			knownParams[paramName] = true
		}
	}

	return knownParams
}

// ValidateUnknownParameters checks for unknown parameters in the raw JSON request
func ValidateUnknownParameters(requestBody []byte) error {
	// Parse the JSON to extract field names
	var rawRequest map[string]interface{}
	if err := json.Unmarshal(requestBody, &rawRequest); err != nil {
		// If JSON is invalid, let the normal validation handle it
		return nil
	}

	// Get known parameters
	knownParams := GetKnownParameters()

	// Check for unknown parameters
	var unknownParams []string
	for paramName := range rawRequest {
		if !knownParams[paramName] {
			unknownParams = append(unknownParams, paramName)
		}
	}

	// If we found unknown parameters, return an error
	if len(unknownParams) > 0 {
		var errorMessage string
		if len(unknownParams) == 1 {
			errorMessage = fmt.Sprintf("unknown parameter '%s' is not supported", unknownParams[0])
		} else {
			errorMessage = "unknown parameters are not supported:"
			for _, param := range unknownParams {
				errorMessage += fmt.Sprintf(" %s", param)
			}
		}

		return errors.New(errorMessage)
	}

	return nil
}
