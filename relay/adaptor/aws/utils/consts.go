package utils

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/songquanpeng/one-api/common/logger"
)

// CrossRegionConfig holds configuration for cross-region inference
type CrossRegionConfig struct {
	EnableCrossRegion      bool
	PreferredRegions       []string
	MaxRetryAttempts       int
	RetryBackoffMultiplier float64
	CacheEnabled           bool
	CacheTTL               time.Duration
	FallbackTimeout        time.Duration
}

// RegionCapacityTracker tracks regional capacity and performance
type RegionCapacityTracker struct {
	mu              sync.RWMutex
	regionHealth    map[string]RegionHealth
	lastHealthCheck map[string]time.Time
	healthCheckTTL  time.Duration
}

// RegionHealth tracks the health status of a region
type RegionHealth struct {
	IsHealthy   bool
	LastError   error
	ErrorCount  int
	LastSuccess time.Time
	AvgLatency  time.Duration
	SuccessRate float64
}

// InferenceProfileCache caches inference profile information
type InferenceProfileCache struct {
	mu               sync.RWMutex
	profilesByRegion map[string][]string // region -> available profile IDs
	modelToProfile   map[string]string   // model -> profile mapping
	lastUpdated      map[string]time.Time
	ttl              time.Duration
}

var (
	// Global instances
	capacityTracker = &RegionCapacityTracker{
		regionHealth:    make(map[string]RegionHealth),
		lastHealthCheck: make(map[string]time.Time),
		healthCheckTTL:  5 * time.Minute,
	}

	profileCache = &InferenceProfileCache{
		profilesByRegion: make(map[string][]string),
		modelToProfile:   make(map[string]string),
		lastUpdated:      make(map[string]time.Time),
		ttl:              30 * time.Minute,
	}

	// GlobalProfileSourceRegions enumerates models with global inference profiles and the
	// source Regions that may access them. Refer to the AWS documentation at
	// https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles-support.html.
	GlobalProfileSourceRegions = map[string][]string{
		"anthropic.claude-sonnet-4-20250514-v1:0": {
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"eu-west-1",
			"ap-northeast-1",
		},
		// claude-opus-4-5 requires the global inference profile for on-demand invocation.
		// AWS does not support direct model ID invocation for this model.
		// Source: https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles-support.html
		"anthropic.claude-opus-4-5-20251101-v1:0": {
			"us-west-1",
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"sa-east-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"eu-south-1",
			"eu-south-2",
			"eu-north-1",
			"eu-central-1",
			"eu-central-2",
			"ca-central-1",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
		},
		"anthropic.claude-opus-4-6-v1": {
			"us-west-1",
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"sa-east-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"eu-south-1",
			"eu-south-2",
			"eu-north-1",
			"eu-central-1",
			"eu-central-2",
			"ca-central-1",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
		},
		"anthropic.claude-sonnet-4-5-20250929-v1:0": {
			"us-west-1",
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"sa-east-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"eu-south-1",
			"eu-south-2",
			"eu-north-1",
			"eu-central-1",
			"eu-central-2",
			"ca-central-1",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
		},
		"anthropic.claude-sonnet-4-6": {
			"us-west-1",
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"sa-east-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"eu-south-1",
			"eu-south-2",
			"eu-north-1",
			"eu-central-1",
			"eu-central-2",
			"ca-central-1",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
		},
		"anthropic.claude-haiku-4-5-20251001-v1:0": {
			"us-west-1",
			"us-west-2",
			"us-east-1",
			"us-east-2",
			"sa-east-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"eu-south-1",
			"eu-south-2",
			"eu-north-1",
			"eu-central-1",
			"eu-central-2",
			"ca-central-1",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
		},
	}

	// Default configuration following AWS best practices
	DefaultCrossRegionConfig = CrossRegionConfig{
		EnableCrossRegion:      true,
		MaxRetryAttempts:       3,
		RetryBackoffMultiplier: 2.0,
		CacheEnabled:           true,
		CacheTTL:               30 * time.Minute,
		FallbackTimeout:        5 * time.Second,
	}
)

// CrossRegionInferences is a list of model IDs that support cross-region inference.
// This serves as the primary reference for supported models.
//
// https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles-support.html
//
// Array.from(new Set(Array.from(document.querySelectorAll('pre.programlisting code')).map(e => e.textContent.trim()).filter(Boolean)));
//
// Note: should also update GlobalProfileSourceRegions accordingly!
var CrossRegionInferences = []string{
	"global.anthropic.claude-opus-4-5-20251101-v1:0",
	"global.anthropic.claude-opus-4-6-v1",
	"global.anthropic.claude-haiku-4-5-20251001-v1:0",
	"global.anthropic.claude-sonnet-4-20250514-v1:0",
	"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"global.anthropic.claude-sonnet-4-6",
	"global.cohere.embed-v4:0",
	"us.anthropic.claude-3-haiku-20240307-v1:0",
	"us.anthropic.claude-3-opus-20240229-v1:0",
	"us.anthropic.claude-3-sonnet-20240229-v1:0",
	"us.anthropic.claude-3-5-haiku-20241022-v1:0",
	"us.anthropic.claude-3-5-sonnet-20240620-v1:0",
	"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
	"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
	"us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"us.anthropic.claude-sonnet-4-6",
	"us.anthropic.claude-opus-4-20250514-v1:0",
	"us.anthropic.claude-opus-4-1-20250805-v1:0",
	"us.anthropic.claude-opus-4-6-v1",
	"us.anthropic.claude-sonnet-4-20250514-v1:0",
	"us.cohere.embed-v4:0",
	"us.deepseek.r1-v1:0",
	"us.meta.llama4-maverick-17b-instruct-v1:0",
	"us.meta.llama4-scout-17b-instruct-v1:0",
	"us.meta.llama3-1-70b-instruct-v1:0",
	"us.meta.llama3-1-8b-instruct-v1:0",
	"us.meta.llama3-1-405b-instruct-v1:0",
	"us.meta.llama3-2-11b-instruct-v1:0",
	"us.meta.llama3-2-1b-instruct-v1:0",
	"us.meta.llama3-2-3b-instruct-v1:0",
	"us.meta.llama3-2-90b-instruct-v1:0",
	"us.meta.llama3-3-70b-instruct-v1:0",
	"us.mistral.pixtral-large-2502-v1:0",
	"us.amazon.nova-lite-v1:0",
	"us.amazon.nova-micro-v1:0",
	"us.amazon.nova-premier-v1:0",
	"us.amazon.nova-pro-v1:0",
	"us.twelvelabs.pegasus-1-2-v1:0",
	"us.stability.stable-conservative-upscale-v1:0",
	"us.stability.stable-image-control-sketch-v1:0",
	"us.stability.stable-image-control-structure-v1:0",
	"us.stability.stable-creative-upscale-v1:0",
	"us.stability.stable-image-erase-object-v1:0",
	"us.stability.stable-fast-upscale-v1:0",
	"us.stability.stable-image-inpaint-v1:0",
	"us.stability.stable-outpaint-v1:0",
	"us.stability.stable-image-remove-background-v1:0",
	"us.stability.stable-image-search-recolor-v1:0",
	"us.stability.stable-image-search-replace-v1:0",
	"us.stability.stable-image-style-guide-v1:0",
	"us.stability.stable-style-transfer-v1:0",
	"us.twelvelabs.marengo-embed-3-0-v1:0",
	"us.twelvelabs.marengo-embed-2-7-v1:0",
	"us.writer.palmyra-x4-v1:0",
	"us.writer.palmyra-x5-v1:0",
	"us-gov.anthropic.claude-3-haiku-20240307-v1:0",
	"us-gov.anthropic.claude-3-5-sonnet-20240620-v1:0",
	"us-gov.anthropic.claude-3-7-sonnet-20250219-v1:0",
	"us-gov.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"us-gov.anthropic.claude-sonnet-4-6",
	"apac.anthropic.claude-3-haiku-20240307-v1:0",
	"apac.anthropic.claude-3-sonnet-20240229-v1:0",
	"apac.anthropic.claude-3-5-sonnet-20240620-v1:0",
	"apac.anthropic.claude-3-5-sonnet-20241022-v2:0",
	"apac.anthropic.claude-3-7-sonnet-20250219-v1:0",
	"apac.anthropic.claude-sonnet-4-20250514-v1:0",
	"apac.anthropic.claude-sonnet-4-6",
	"apac.amazon.nova-lite-v1:0",
	"apac.amazon.nova-micro-v1:0",
	"apac.amazon.nova-pro-v1:0",
	"apac.twelvelabs.pegasus-1-2-v1:0",
	"apac.twelvelabs.marengo-embed-2-7-v1:0",
	"au.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"au.anthropic.claude-opus-4-6-v1",
	"au.anthropic.claude-sonnet-4-6",
	"au.anthropic.claude-haiku-4-5-20251001-v1:0",
	"ca.amazon.nova-lite-v1:0",
	"eu.anthropic.claude-3-haiku-20240307-v1:0",
	"eu.anthropic.claude-3-sonnet-20240229-v1:0",
	"eu.anthropic.claude-3-5-sonnet-20240620-v1:0",
	"eu.anthropic.claude-3-7-sonnet-20250219-v1:0",
	"eu.anthropic.claude-haiku-4-5-20251001-v1:0",
	"eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"eu.anthropic.claude-sonnet-4-6",
	"eu.anthropic.claude-sonnet-4-20250514-v1:0",
	"eu.anthropic.claude-opus-4-6-v1",
	"eu.cohere.embed-v4:0",
	"eu.meta.llama3-2-1b-instruct-v1:0",
	"eu.meta.llama3-2-3b-instruct-v1:0",
	"eu.mistral.pixtral-large-2502-v1:0",
	"eu.amazon.nova-lite-v1:0",
	"eu.amazon.nova-micro-v1:0",
	"eu.amazon.nova-pro-v1:0",
	"eu.twelvelabs.marengo-embed-3-0-v1:0",
	"eu.twelvelabs.marengo-embed-2-7-v1:0",
	"eu.twelvelabs.pegasus-1-2-v1:0",
	"jp.anthropic.claude-haiku-4-5-20251001-v1:0",
	"jp.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"jp.anthropic.claude-sonnet-4-6",
}

// RegionMapping defines the mapping between AWS regions and their cross-region inference prefixes
// Following AWS best practices for region grouping
var RegionMapping = map[string][]string{
	// US regions
	"us-east-1":    {"us"},
	"us-east-2":    {"us"},
	"us-west-1":    {"us"},
	"us-west-2":    {"us"},
	"ca-central-1": {"us"}, // Canada uses US inference profiles
	"sa-east-1":    {"us"}, // South America uses US inference profiles

	// US Government regions
	"us-gov-east-1": {"us-gov"},
	"us-gov-west-1": {"us-gov"},

	// EU regions
	"eu-west-1":    {"eu"},
	"eu-west-2":    {"eu"},
	"eu-west-3":    {"eu"},
	"eu-north-1":   {"eu"},
	"eu-south-1":   {"eu"},
	"eu-central-1": {"eu"},
	"eu-central-2": {"eu"},

	// Asia Pacific regions
	"ap-south-1":     {"apac"},
	"ap-south-2":     {"apac"},
	"ap-southeast-1": {"apac"},
	"ap-southeast-2": {"apac", "au"},
	"ap-southeast-3": {"apac"},
	"ap-southeast-4": {"apac", "au"},
	"ap-northeast-1": {"jp", "apac"},
	"ap-northeast-2": {"apac"},
	"ap-northeast-3": {"jp", "apac"},
}

// getRegionPrefix returns the primary cross-region inference prefix for a given AWS region.
func getRegionPrefix(region string) string {
	prefixes := getRegionPrefixes(region)
	if len(prefixes) == 0 {
		return ""
	}

	return prefixes[0]
}

// getRegionPrefixes returns the ordered set of cross-region inference prefixes for the region.
// The first prefix matches the historical single-prefix behavior to keep backwards compatibility.
func getRegionPrefixes(region string) []string {
	if prefixes, exists := RegionMapping[region]; exists {
		if len(prefixes) > 0 {
			return prefixes
		}
	}

	// Fallback to parsing logic for regions not in the map
	if strings.HasPrefix(region, "us-gov-") {
		return []string{"us-gov"}
	}

	parts := strings.Split(region, "-")
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "us":
		return []string{"us"}
	case "eu":
		return []string{"eu"}
	case "ap":
		return []string{"apac"}
	case "ca", "sa":
		// Canadian and South American regions typically use US inference profiles
		return []string{"us"}
	case "au":
		return []string{"au"}
	default:
		logger.Logger.Debug("unknown region prefix", zap.String("region", region))
		return nil
	}
}

// TestInferenceProfileAvailability tests if a cross-region inference profile is available
// by attempting a lightweight operation with the runtime client
func TestInferenceProfileAvailability(ctx context.Context, client *bedrockruntime.Client, profileID string) bool {
	profileCache.mu.RLock()
	cacheKey := client.Options().Region + ":" + profileID
	if availability, exists := profileCache.modelToProfile[cacheKey]; exists {
		lastUpdated := profileCache.lastUpdated[client.Options().Region]
		if time.Since(lastUpdated) < profileCache.ttl {
			profileCache.mu.RUnlock()
			return availability == "available"
		}
	}
	profileCache.mu.RUnlock()

	// Test with a minimal request to check availability
	// We'll try to invoke the model with minimal input to see if it's available
	available := testModelAvailability(ctx, client, profileID)

	// Update cache
	profileCache.mu.Lock()
	if available {
		profileCache.modelToProfile[cacheKey] = "available"
	} else {
		profileCache.modelToProfile[cacheKey] = "unavailable"
	}
	profileCache.lastUpdated[client.Options().Region] = time.Now()
	profileCache.mu.Unlock()

	return available
}

// testModelAvailability performs a lightweight test to check if the model is available
func testModelAvailability(ctx context.Context, client *bedrockruntime.Client, modelID string) bool {
	// Create a timeout context for the test
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Use a minimal test request that should fail quickly if the model doesn't exist
	// but succeed (or fail with a different error) if it does exist
	testInput := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        []byte(`{"max_tokens": 1}`), // Minimal test payload
	}

	_, err := client.InvokeModel(testCtx, testInput)

	// If the error indicates the model doesn't exist, return false
	// If there's no error or a different error (like validation), the model likely exists
	if err != nil {
		errStr := err.Error()
		// AWS returns specific error codes for non-existent models
		if strings.Contains(errStr, "ValidationException") &&
			(strings.Contains(errStr, "model") || strings.Contains(errStr, "not found")) {
			return false
		}
	}

	return true
}

// ConvertModelID2CrossRegionProfile converts the model ID to a cross-region profile ID.
// Enhanced version that uses aws-sdk-go-v2 patterns and includes availability testing.
func ConvertModelID2CrossRegionProfile(ctx context.Context, model, region string) string {
	lg := gmw.GetLogger(ctx)

	// First check if this model requires a global inference profile
	if allowedRegions, exists := GlobalProfileSourceRegions[model]; exists {
		if slices.Contains(allowedRegions, region) {
			globalModel := "global." + model
			if slices.Contains(CrossRegionInferences, globalModel) {
				lg.Debug("convert model to global cross-region profile",
					zap.String("model", model),
					zap.String("region", region),
					zap.String("cross_region_profile", globalModel))
				return globalModel
			}
			lg.Debug("global profile not found in CrossRegionInferences",
				zap.String("model", model),
				zap.String("region", region),
				zap.String("expected_global_profile", globalModel))
		} else {
			lg.Debug("region not in GlobalProfileSourceRegions allowed list",
				zap.String("model", model),
				zap.String("region", region),
				zap.Strings("allowed_regions", allowedRegions))
		}
	}

	regionPrefixes := getRegionPrefixes(region)
	if len(regionPrefixes) == 0 {
		lg.Debug("unsupported region for cross-region inference, using raw model ID",
			zap.String("model", model),
			zap.String("region", region))
		return model
	}

	for _, regionPrefix := range regionPrefixes {
		newModelID := regionPrefix + "." + model
		if slices.Contains(CrossRegionInferences, newModelID) {
			lg.Debug("convert model to cross-region profile",
				zap.String("model", model),
				zap.String("region", region),
				zap.String("region_prefix", regionPrefix),
				zap.String("cross_region_profile", newModelID))
			return newModelID
		}
	}

	lg.Debug("no cross-region profile found, using raw model ID",
		zap.String("model", model),
		zap.String("region", region),
		zap.Strings("tried_prefixes", regionPrefixes))
	return model
}

// ConvertModelID2CrossRegionProfileWithFallback provides enhanced conversion with runtime availability testing
func ConvertModelID2CrossRegionProfileWithFallback(ctx context.Context, model, region string, client *bedrockruntime.Client) string {
	lg := gmw.GetLogger(ctx)

	// Try cross-region profile first
	crossRegionModel := ConvertModelID2CrossRegionProfile(ctx, model, region)

	// If we got a cross-region profile and have a client, test availability
	if crossRegionModel != model && client != nil {
		if TestInferenceProfileAvailability(ctx, client, crossRegionModel) {
			lg.Debug("cross-region profile available", zap.String("cross_region_profile", crossRegionModel))
			return crossRegionModel
		}
		lg.Debug("cross-region profile not available, falling back", zap.String("cross_region_profile", crossRegionModel))
		return model
	}

	// If no client provided, return the cross-region profile anyway (best effort)
	// This allows the system to still attempt cross-region inference
	if crossRegionModel != model {
		lg.Debug("no client for availability testing, using cross-region profile", zap.String("cross_region_profile", crossRegionModel))
		return crossRegionModel
	}

	return model
}

// UpdateRegionHealthMetrics updates health metrics for a region based on operation results
func UpdateRegionHealthMetrics(region string, success bool, latency time.Duration, err error) {
	capacityTracker.mu.Lock()
	defer capacityTracker.mu.Unlock()

	health, exists := capacityTracker.regionHealth[region]
	if !exists {
		health = RegionHealth{
			IsHealthy:   true,
			SuccessRate: 1.0,
		}
	}

	now := time.Now()

	if success {
		health.LastSuccess = now
		health.ErrorCount = 0
		health.LastError = nil
		health.IsHealthy = true

		// Update average latency (simple moving average)
		if health.AvgLatency == 0 {
			health.AvgLatency = latency
		} else {
			health.AvgLatency = (health.AvgLatency + latency) / 2
		}

		// Update success rate
		health.SuccessRate = (health.SuccessRate + 1.0) / 2.0
	} else {
		health.ErrorCount++
		health.LastError = err
		health.SuccessRate = health.SuccessRate * 0.9 // Decay success rate

		// Mark as unhealthy if too many consecutive errors
		if health.ErrorCount >= 3 {
			health.IsHealthy = false
		}
	}

	capacityTracker.regionHealth[region] = health
	capacityTracker.lastHealthCheck[region] = now
}

// GetRegionHealth returns the current health status of a region
func GetRegionHealth(region string) RegionHealth {
	capacityTracker.mu.RLock()
	defer capacityTracker.mu.RUnlock()

	if health, exists := capacityTracker.regionHealth[region]; exists {
		return health
	}

	// Return default healthy status for unknown regions
	return RegionHealth{
		IsHealthy:   true,
		SuccessRate: 1.0,
	}
}
