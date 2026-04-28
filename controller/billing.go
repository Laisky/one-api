package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// GetSubscription returns the user's subscription-style quota summary in the OpenAI billing format.
func GetSubscription(c *gin.Context) {
	var remainQuota int64
	var usedQuota int64
	var err error
	var token *model.Token
	var expiredTime int64
	if config.DisplayTokenStatEnabled {
		tokenId := c.GetInt(ctxkey.TokenId)
		token, err = model.GetTokenById(tokenId)
		if err == nil {
			expiredTime = token.ExpiredTime
			remainQuota = token.RemainQuota
			usedQuota = token.UsedQuota
		}
	} else {
		userId := c.GetInt(ctxkey.Id)
		remainQuota, err = model.GetUserQuota(userId)
		if err != nil {
			usedQuota, err = model.GetUserUsedQuota(userId)
		}
	}
	if expiredTime <= 0 {
		expiredTime = 0
	}
	if err != nil {
		Error := relaymodel.Error{Message: err.Error(), Type: relaymodel.ErrorTypeUpstream, RawError: err}
		c.JSON(http.StatusOK, gin.H{
			"error": Error,
		})
		return
	}
	quota := remainQuota + usedQuota
	amount := float64(quota)
	if config.DisplayInCurrencyEnabled {
		amount /= config.QuotaPerUnit
	}
	if token != nil && token.UnlimitedQuota {
		amount = 100000000
	}
	subscription := OpenAISubscriptionResponse{
		Object:             "billing_subscription",
		HasPaymentMethod:   true,
		SoftLimitUSD:       amount,
		HardLimitUSD:       amount,
		SystemHardLimitUSD: amount,
		AccessUntil:        expiredTime,
	}
	c.JSON(http.StatusOK, subscription)
}

// GetUsage returns the user's quota consumption wrapped in the OpenAI usage response shape.
func GetUsage(c *gin.Context) {
	var quota int64
	var err error
	var token *model.Token
	if config.DisplayTokenStatEnabled {
		tokenId := c.GetInt(ctxkey.TokenId)
		token, err = model.GetTokenById(tokenId)
		quota = token.UsedQuota
	} else {
		userId := c.GetInt(ctxkey.Id)
		quota, err = model.GetUserUsedQuota(userId)
	}
	if err != nil {
		Error := relaymodel.Error{Message: err.Error(), Type: relaymodel.ErrorTypeOneAPI, RawError: err}
		c.JSON(http.StatusOK, gin.H{
			"error": Error,
		})
		return
	}
	amount := float64(quota)
	if config.DisplayInCurrencyEnabled {
		amount /= config.QuotaPerUnit
	}
	usage := OpenAIUsageResponse{
		Object:     "list",
		TotalUsage: amount * 100,
	}
	c.JSON(http.StatusOK, usage)
}
