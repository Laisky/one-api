package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// GetAllRedemptions lists redemption codes with pagination.
func GetAllRedemptions(c *gin.Context) {
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

	redemptions, err := model.GetAllRedemptions(p*size, size)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetRedemptionCount()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemptions,
		"total":   totalCount,
	})
}

// SearchRedemptions performs a keyword search for redemption codes and returns paginated results.
func SearchRedemptions(c *gin.Context) {
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
	redemptions, total, err := model.SearchRedemptions(keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemptions,
		"total":   total,
	})
}

// GetRedemption fetches a single redemption code by its identifier.
func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
}

// AddRedemption creates one or more redemption codes based on the supplied payload.
func AddRedemption(c *gin.Context) {
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if strings.TrimSpace(redemption.Name) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Redemption name is required"})
		return
	}
	if len(redemption.Name) == 0 || len(redemption.Name) > 20 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The length of the redemption code name must be between 1-20",
		})
		return
	}
	if redemption.Count <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The number of redemption codes must be greater than 0",
		})
		return
	}
	if redemption.Count > 100 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The number of redemption codes generated in a batch cannot be greater than 100",
		})
		return
	}
	var keys []string
	for i := 0; i < redemption.Count; i++ {
		key := random.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:      c.GetInt(ctxkey.Id),
			Name:        redemption.Name,
			Key:         key,
			CreatedTime: helper.GetTimestamp(),
			Quota:       redemption.Quota,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
}

// DeleteRedemption removes a redemption code by ID.
func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// UpdateRedemption modifies redemption metadata or status depending on the request.
func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	} else {
		// If you add more fields, please also update redemption.Update()
		if strings.TrimSpace(redemption.Name) == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Redemption name cannot be empty"})
			return
		}
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
	}
	err = cleanRedemption.Update()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
}
