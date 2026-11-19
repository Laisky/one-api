package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

const (
	// maxChannelPricingReadinessBodyBytes bounds a readiness request body. It
	// leaves room for a large model_configs document while keeping the
	// decoder's allocations bounded.
	maxChannelPricingReadinessBodyBytes = 4 << 20
	// maxChannelPricingReadinessModels bounds how many supported models one
	// readiness request may evaluate.
	maxChannelPricingReadinessModels = 10_000
)

// channelPricingReadinessRequest is the channel draft the editor checks
// before saving. Its fields use the channel save payload's encoding: Type is
// the channel type, Models the comma-separated supported models, and
// ModelMapping and ModelConfigs the JSON text of those channel columns (nil
// or empty when unset).
type channelPricingReadinessRequest struct {
	Type         int     `json:"type"`
	Models       string  `json:"models"`
	ModelMapping *string `json:"model_mapping"`
	ModelConfigs *string `json:"model_configs"`
}

// CheckChannelPricingReadiness reports which supported models of a channel
// draft request admission would refuse until the channel prices them, so the
// admin editor can warn before saving. It parses the draft with the same
// parsers the relay uses for saved channels and evaluates it with the
// admission validators; it reads no database rows and changes nothing.
// Parameters: c carries the admin request whose JSON body is a
// channelPricingReadinessRequest. It responds 400 for a malformed, oversized,
// or out-of-range draft, and otherwise {"success":true,"data":{"gaps":[...]}}.
func CheckChannelPricingReadiness(c *gin.Context) {
	lg := gmw.GetLogger(c)
	var request channelPricingReadinessRequest
	if err := bindChannelPricingReadinessRequest(c, &request); err != nil {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, err)
		return
	}
	provider := relaycontroller.ResolveChannelPricingAdaptor(request.Type)
	if provider == nil {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errkind.InvalidRequestErr(errors.New("Unsupported channel type")))
		return
	}
	// Bound the split before allocating it: every model needs a separator.
	if strings.Count(request.Models, ",") >= maxChannelPricingReadinessModels {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errkind.InvalidRequestErr(
			errors.Errorf("At most %d supported models can be checked at once", maxChannelPricingReadinessModels)))
		return
	}
	draft := &model.Channel{Type: request.Type, Models: request.Models}
	models := draft.GetSupportedModelNames()
	mapping, err := model.ParseModelMapping(request.ModelMapping)
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errkind.InvalidRequestErr(
			errors.Wrap(err, "Model Mapping must be a JSON object of model names")))
		return
	}
	configs, err := model.ParseModelPriceConfigs(request.ModelConfigs)
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errkind.InvalidRequestErr(
			errors.Wrap(err, "Model Configs must be a JSON object of model pricing")))
		return
	}

	gaps := relaycontroller.EvaluateChannelPricingReadiness(provider, models, mapping, configs, time.Now().UTC())
	lg.Debug("channel pricing readiness evaluated",
		zap.Int("channel_type", request.Type),
		zap.Int("models", len(models)),
		zap.Int("gaps", len(gaps)))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"gaps": gaps},
	})
}

// bindChannelPricingReadinessRequest strictly decodes one bounded readiness
// request. Parameters: c carries the request body and request receives the
// decoded draft. Returns: an InvalidRequest error for an oversized body,
// malformed JSON, unknown fields, or trailing data; nil otherwise.
func bindChannelPricingReadinessRequest(c *gin.Context, request *channelPricingReadinessRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxChannelPricingReadinessBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		if maxBytesErr := (*http.MaxBytesError)(nil); errors.As(err, &maxBytesErr) {
			return errkind.InvalidRequestErr(errors.Errorf("A pricing readiness request must not exceed %d bytes.", maxChannelPricingReadinessBodyBytes))
		}
		return errkind.InvalidRequestErr(errors.Wrap(err, "A valid channel pricing readiness request is required"))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errkind.InvalidRequestErr(errors.New("Only one channel pricing readiness request is allowed."))
	}
	return nil
}
