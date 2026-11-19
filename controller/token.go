package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/network"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// GetRequestCost answers the public per-request cost lookup for the
// request_id path parameter. It writes exactly one response: the cost record,
// or a single error envelope (an unknown id is a client-side NotFound).
func GetRequestCost(c *gin.Context) {
	reqId := c.Param("request_id")
	if reqId == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("request_id should not be empty")))
		return
	}

	docu, err := model.GetCostByRequestId(reqId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = errkind.NotFoundErr(err)
		}
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, docu)
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	// Get page size from query parameter, default to config value
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	order := c.Query("order")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	tokens, err := model.GetAllUserTokens(userId, p*size, size, order, sortBy, sortOrder)

	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetUserTokenCount(userId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.TokensToResponses(tokens),
		"total":   totalCount,
	})
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	keyword := c.Query("keyword")
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	tokens, total, err := model.SearchUserTokens(userId, keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.TokensToResponses(tokens),
		"total":   total,
	})
}

func GetToken(c *gin.Context) {
	id, err := resolveTokenRef(c.Param("id"))
	userId := c.GetInt(ctxkey.Id)
	if err != nil {
		respondResourceRefError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    token.ToResponse(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt(ctxkey.TokenId)
	userId := c.GetInt(ctxkey.Id)
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func validateToken(_ *gin.Context, token *model.Token) error {
	if len(token.Name) > 30 {
		return errors.Errorf("Token name is too long")
	}

	if token.Subnet != nil && *token.Subnet != "" {
		err := network.IsValidSubnets(*token.Subnet)
		if err != nil {
			return errors.Wrap(err, "invalid network segment")
		}
	}

	return nil
}

func AddToken(c *gin.Context) {
	token := new(model.Token)
	err := c.ShouldBindJSON(token)
	if err != nil {
		// Malformed request body: the caller sent JSON this endpoint cannot bind.
		helper.RespondError(c, errkind.InvalidRequestErr(err))
		return
	}
	token.UUID = ""
	token.UserUUID = nil

	// Disallow empty name on create
	if strings.TrimSpace(token.Name) == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Token name is required")))
		return
	}

	err = validateToken(c, token)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Errorf("invalid token: %s", err.Error())))
		return
	}

	cleanToken := model.Token{
		UserId:         c.GetInt(ctxkey.Id),
		Name:           token.Name,
		Key:            random.GenerateKey(),
		CreatedTime:    helper.GetTimestamp(),
		AccessedTime:   helper.GetTimestamp(),
		ExpiredTime:    token.ExpiredTime,
		RemainQuota:    token.RemainQuota,
		UnlimitedQuota: token.UnlimitedQuota,
		Models:         token.Models,
		Subnet:         token.Subnet,
	}
	err = cleanToken.Insert(gmw.Ctx(c))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanToken.ToResponse(),
	})
}

func DeleteToken(c *gin.Context) {
	id, err := resolveTokenRef(c.Param("id"))
	if err != nil {
		respondResourceRefError(c, err)
		return
	}
	userId := c.GetInt(ctxkey.Id)
	err = model.DeleteTokenById(gmw.Ctx(c), id, userId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	statusOnly := c.Query("status_only")
	tokenPatch := new(model.Token)
	err := c.ShouldBindJSON(tokenPatch)
	if err != nil {
		// Malformed request body: the caller sent JSON this endpoint cannot bind.
		helper.RespondError(c, errkind.InvalidRequestErr(err))
		return
	}
	ref, err := preferUUIDRef(tokenPatch.UUID, tokenPatch.Id)
	if err != nil {
		respondResourceRefError(c, err)
		return
	}
	resolvedTokenID, err := resolveTokenRef(ref)
	if err != nil {
		respondResourceRefError(c, err)
		return
	}
	tokenPatch.Id = resolvedTokenID
	tokenPatch.UUID = ""
	tokenPatch.UserUUID = nil

	// Disallow empty name when not status_only
	if statusOnly == "" && strings.TrimSpace(tokenPatch.Name) == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Token name cannot be empty")))
		return
	}

	token := new(model.Token)
	if err = copier.Copy(token, tokenPatch); err != nil {
		helper.RespondError(c, err)
		return
	}

	err = validateToken(c, token)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Errorf("invalid token: %s", err.Error())))
		return
	}

	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	switch token.Status {
	case model.TokenStatusEnabled:
		if cleanToken.Status == model.TokenStatusExpired &&
			cleanToken.ExpiredTime <= helper.GetTimestamp() && cleanToken.ExpiredTime != -1 &&
			token.ExpiredTime != -1 && token.ExpiredTime < helper.GetTimestamp() {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("The token has expired and cannot be enabled. Please modify the expiration time of the token, or set it to never expire.")))
			return
		}
		if cleanToken.Status == model.TokenStatusExhausted &&
			cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota &&
			token.RemainQuota <= 0 && !token.UnlimitedQuota {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("The available quota of the token has been used up and cannot be enabled. Please modify the remaining quota of the token, or set it to unlimited quota")))
			return
		}
	case model.TokenStatusExhausted:
		if token.RemainQuota > 0 || token.UnlimitedQuota {
			token.Status = model.TokenStatusEnabled
		}
	case model.TokenStatusExpired:
		if token.ExpiredTime == -1 || token.ExpiredTime > helper.GetTimestamp() {
			token.Status = model.TokenStatusEnabled
		}
	}

	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		cleanToken.Models = token.Models
		cleanToken.Subnet = token.Subnet
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.Status = token.Status
	}

	err = cleanToken.Update(gmw.Ctx(c))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanToken.ToResponse(),
	})
}
