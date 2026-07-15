package handlers

import (
	"encoding/json"

	"example.com/dto"
	"example.com/gin"
	"example.com/model"
)

// --- positive cases: raw entities reaching a JSON boundary ---

func direct(c *gin.Context) {
	var u model.User
	c.JSON(200, u) // want `raw model\.User reaches a JSON boundary`
}

func pointerInGinH(c *gin.Context) {
	u := &model.User{}
	c.JSON(200, gin.H{"data": u}) // want `raw model\.User reaches a JSON boundary`
}

func sliceInGinH(c *gin.Context) {
	var ts []*model.Token
	c.JSON(200, gin.H{"data": ts}) // want `raw model\.Token reaches a JSON boundary`
}

func nestedGinH(c *gin.Context) {
	r := model.Redemption{}
	c.JSON(200, gin.H{"payload": gin.H{"item": r}}) // want `raw model\.Redemption reaches a JSON boundary`
}

func abortJSON(c *gin.Context) {
	c.AbortWithStatusJSON(400, gin.H{"data": &model.Log{}}) // want `raw model\.Log reaches a JSON boundary`
}

func marshalValue() {
	var ch model.Channel
	_, _ = json.Marshal(ch) // want `raw model\.Channel reaches a JSON boundary`
}

func marshalSlice() {
	var rs []model.Redemption
	_, _ = json.Marshal(rs) // want `raw model\.Redemption reaches a JSON boundary`
}

func indentedJSONSlice(c *gin.Context) {
	var us []model.User
	c.IndentedJSON(200, us) // want `raw model\.User reaches a JSON boundary`
}

// --- negative cases: must NOT be flagged ---

func okResponseMapper(c *gin.Context) {
	var u model.User
	c.JSON(200, gin.H{"data": u.ToResponse()})
}

func okListMapper(c *gin.Context) {
	var us []*model.User
	c.JSON(200, gin.H{"data": model.UsersToResponses(us)})
}

func okScalars(c *gin.Context) {
	c.JSON(200, gin.H{"message": "ok", "count": 3, "success": true})
}

func okSafeType(c *gin.Context) {
	c.JSON(200, model.Safe{})
}

func okDtoDirect(c *gin.Context) {
	c.JSON(200, dto.UserResponse{})
}

func okMarshalDto() {
	_, _ = json.Marshal(dto.TokenResponse{})
}
