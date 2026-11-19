package controller

import (
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// AdminGetAllTokens lists tokens across users. Admin-only, read-only.
// Query params: p, size, user_id (optional filter, 0/absent = all users), sort, order.
func AdminGetAllTokens(c *gin.Context) {
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
	userId, err := resolveOptionalUserRef(c.Query("user_id"))
	if err != nil {
		respondResourceRefError(c, err)
		return
	}
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	tokens, total, err := model.GetAllTokensForAdmin(userId, p*size, size, sortBy, sortOrder)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.TokensToAdminResponses(tokens),
		"total":   total,
	})
}

// AdminSearchTokens searches tokens across users by name keyword. Admin-only, read-only.
func AdminSearchTokens(c *gin.Context) {
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

	tokens, total, err := model.SearchAllTokensForAdmin(keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.TokensToAdminResponses(tokens),
		"total":   total,
	})
}

// AdminGetToken returns a token by id regardless of owner. Admin-only, read-only.
// Parameters: c carries the token UUID in the :id path parameter.
// Return values: none; writes the admin token view, or an error envelope whose
// message names the token id only when the id itself is malformed.
func AdminGetToken(c *gin.Context) {
	id, err := resolveTokenRef(c.Param("id"))
	if err != nil {
		// Only a malformed id is an invalid token id. A well-formed UUID that
		// matches no token, or a failed lookup, keeps the shared message.
		if isInvalidResourceRef(err) {
			err = errors.Wrap(err, "invalid token id")
		}
		respondResourceRefError(c, err)
		return
	}
	token, err := model.GetTokenById(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    token.ToAdminResponse(),
	})
}
