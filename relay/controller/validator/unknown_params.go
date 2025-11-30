package validator

import (
	"encoding/json"
	"reflect"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/model"
)

// GetKnownParameters extracts all valid JSON parameter names from GeneralOpenAIRequest struct
func GetKnownParameters() map[string]bool {
	knownParams := make(map[string]bool)
	// Collect known parameters from multiple request structs that we accept
	// This includes legacy/general OpenAI-style DTOs as well as rerank-specific DTO.
	requestTypes := []reflect.Type{
		reflect.TypeOf(model.GeneralOpenAIRequest{}),
		reflect.TypeOf(model.RerankRequest{}),
	}

	for _, requestType := range requestTypes {
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
	}

	return knownParams
}

// ValidateUnknownParameters checks for unknown parameters in the raw JSON request.
// Instead of rejecting requests with unknown parameters, it logs a warning and allows
// the request to proceed. This ensures forward compatibility when upstream services
// add new parameters that one-api hasn't explicitly added yet.
//
// Note: Unknown parameters will be dropped during deserialization since they're not
// defined in the struct. If you need a new parameter to be passed through, add it
// to the appropriate request struct (e.g., GeneralOpenAIRequest).
func ValidateUnknownParameters(requestBody []byte) error {
	unknownParams := findUnknownParameters(requestBody)
	if len(unknownParams) > 0 {
		// Log warning but don't reject the request
		logger.Logger.Warn("request contains unknown parameters that will be ignored",
			zap.Strings("unknown_params", unknownParams),
		)
	}

	return nil
}

// ValidateUnknownParametersWithContext checks for unknown parameters and logs warnings
// with request context. Use this variant when you have access to a gin.Context for
// richer logging context.
func ValidateUnknownParametersWithContext(c *gin.Context, requestBody []byte) error {
	unknownParams := findUnknownParameters(requestBody)
	if len(unknownParams) > 0 {
		lg := gmw.GetLogger(c)
		lg.Warn("request contains unknown parameters that will be ignored",
			zap.Strings("unknown_params", unknownParams),
		)
	}

	return nil
}

// findUnknownParameters identifies parameters in the request body that are not
// defined in the known request structs.
func findUnknownParameters(requestBody []byte) []string {
	// Parse the JSON to extract field names
	var rawRequest map[string]any
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

	return unknownParams
}
