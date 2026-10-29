package controller

import (
	"github.com/Laisky/one-api/common/ctxkey"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"io"
)

// writeClaudeGeminiOutput records only image blocks whose complete bytes were accepted.
func writeClaudeGeminiOutput(c *gin.Context, body []byte, receipt *relaymodel.ClaudeImageReceipt) error {
	n, err := c.Writer.Write(body)
	if n < 0 {
		n = 0
	}
	if n > len(body) {
		n = len(body)
	}
	delivered := 0
	for _, end := range receipt.ImageEndOffsets {
		if end <= n {
			delivered++
		}
	}
	c.Set(ctxkey.OutputImageCount, delivered)
	if receipt.Incomplete || n < len(body) || err != nil || delivered != len(receipt.ImageEndOffsets) {
		c.Set(ctxkey.GeminiOutputIncomplete, true)
	}
	if err != nil {
		return err
	}
	if n != len(body) {
		return io.ErrShortWrite
	}
	return nil
}
