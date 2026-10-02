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
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

const (
	muAPIPricingTimeout  = 5 * time.Second
	maxMuAPIPricingBytes = 64 << 10
	muAPIPricingCurrency = "USD"
)

// EstimateVideoCostUSD asks MuAPI for the exact cost of the normalized request
// before one-api reserves quota. Parameters: c carries the body and trusted
// selected channel, meta must agree with that channel, and request supplies duration
// and resolution billing hints. Return values preserve the provider's exact
// total USD decimal or an error when MuAPI cannot quote the request.
func (a *Adaptor) EstimateVideoCostUSD(c *gin.Context, metaInfo *meta.Meta, request *model.VideoRequest) (string, error) {
	if c == nil || metaInfo == nil || request == nil {
		return "", errors.New("MuAPI pricing estimate requires request context and metadata")
	}
	duration := request.RequestedDurationSeconds()
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return "", errors.New("MuAPI pricing estimate requires a positive duration")
	}
	modelName := strings.TrimSpace(metaInfo.ActualModelName)
	if !validMuAPIModelName(modelName) {
		return "", errors.Errorf("invalid MuAPI model slug %q", modelName)
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return "", errors.Wrap(err, "read normalized MuAPI video request for pricing")
	}

	ctx, cancel := context.WithTimeout(gmw.Ctx(c), muAPIPricingTimeout)
	defer cancel()
	endpoint, apiKey, err := muAPIQuoteDestination(c, metaInfo, modelName)
	if err != nil {
		return "", errors.Wrap(err, "validate MuAPI pricing endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.Wrap(err, "create MuAPI pricing request")
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-api-key", apiKey)
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
	boundedClient.CheckRedirect = a.CheckRedirect
	resp, err := boundedClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "request MuAPI video pricing")
	}
	if resp == nil {
		return "", errors.New("MuAPI pricing response is nil")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxMuAPIPricingBytes+1))
	if err != nil {
		return "", errors.Wrap(err, "read MuAPI pricing response")
	}
	if len(responseBody) > maxMuAPIPricingBytes {
		return "", errors.New("MuAPI pricing response is too large")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.Errorf("MuAPI pricing endpoint returned HTTP %d", resp.StatusCode)
	}

	var estimate struct {
		Cost     json.Number `json:"cost"`
		Currency string      `json:"currency"`
	}
	if err := json.Unmarshal(responseBody, &estimate); err != nil {
		return "", errors.Wrap(err, "decode MuAPI pricing response")
	}
	if estimate.Currency != "" && !strings.EqualFold(estimate.Currency, muAPIPricingCurrency) {
		return "", errors.Errorf("MuAPI pricing response uses unsupported currency %q", estimate.Currency)
	}
	quotedCost := strings.TrimSpace(estimate.Cost.String())
	quota, err := dbmodel.AsyncUpstreamCostQuota("1", quotedCost)
	if err != nil || quota <= 0 {
		return "", errors.New("MuAPI pricing response did not contain a bounded positive USD cost")
	}
	return quotedCost, nil
}
