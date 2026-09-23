package muapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

const (
	muAPIPricingTimeout  = 5 * time.Second
	maxMuAPIPricingBytes = 64 << 10
	muAPIPricingCurrency = "USD"
)

// EstimateVideoPricing asks MuAPI for the exact cost of the normalized request
// before one-api reserves quota. Parameters: c carries the request body, meta
// identifies the configured MuAPI host and key, and request supplies duration
// and resolution billing hints. Return values are an equivalent per-second
// tariff or an error when MuAPI cannot quote the request.
func (a *Adaptor) EstimateVideoPricing(c *gin.Context, metaInfo *meta.Meta, request *model.VideoRequest) (*adaptor.VideoPricingConfig, error) {
	if c == nil || metaInfo == nil || request == nil {
		return nil, errors.New("MuAPI pricing estimate requires request context and metadata")
	}
	duration := request.RequestedDurationSeconds()
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return nil, errors.New("MuAPI pricing estimate requires a positive duration")
	}
	modelName := strings.TrimSpace(metaInfo.ActualModelName)
	if !validMuAPIModelName(modelName) {
		return nil, errors.Errorf("invalid MuAPI model slug %q", modelName)
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "read normalized MuAPI video request for pricing")
	}

	ctx, cancel := context.WithTimeout(gmw.Ctx(c), muAPIPricingTimeout)
	defer cancel()
	endpoint := muAPICoreBaseURL(metaInfo.BaseURL) + "/models/" + modelName + "/estimate-cost"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create MuAPI pricing request")
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(metaInfo.APIKey) != "" {
		req.Header.Set("x-api-key", metaInfo.APIKey)
	}

	httpClient := client.HTTPClient
	if httpClient == nil {
		client.Init()
		httpClient = client.HTTPClient
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	boundedClient := *httpClient
	if boundedClient.Timeout == 0 || boundedClient.Timeout > muAPIPricingTimeout {
		boundedClient.Timeout = muAPIPricingTimeout
	}
	resp, err := boundedClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request MuAPI video pricing")
	}
	if resp == nil {
		return nil, errors.New("MuAPI pricing response is nil")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxMuAPIPricingBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "read MuAPI pricing response")
	}
	if len(responseBody) > maxMuAPIPricingBytes {
		return nil, errors.New("MuAPI pricing response is too large")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.Errorf("MuAPI pricing endpoint returned HTTP %d", resp.StatusCode)
	}

	var estimate struct {
		Cost     float64 `json:"cost"`
		Currency string  `json:"currency"`
	}
	if err := json.Unmarshal(responseBody, &estimate); err != nil {
		return nil, errors.Wrap(err, "decode MuAPI pricing response")
	}
	if estimate.Currency != "" && !strings.EqualFold(estimate.Currency, muAPIPricingCurrency) {
		return nil, errors.Errorf("MuAPI pricing response uses unsupported currency %q", estimate.Currency)
	}
	if estimate.Cost <= 0 || math.IsNaN(estimate.Cost) || math.IsInf(estimate.Cost, 0) {
		return nil, errors.New("MuAPI pricing response did not contain a positive USD cost")
	}

	return &adaptor.VideoPricingConfig{
		PerSecondUsd:   estimate.Cost / duration,
		BaseResolution: request.RequestedResolution(),
	}, nil
}
