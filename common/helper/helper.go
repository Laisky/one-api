package helper

import (
	"fmt"
	"html/template"

	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/random"
)

// OpenBrowser attempts to launch the system browser pointing at the provided URL.
// Any failure is logged but not returned to the caller.
func OpenBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		logger.Logger.Error("failed to open browser", zap.Error(err))
	}
}

// RespondError sends a JSON response with a success status and an error message.
func RespondError(c *gin.Context, err error) {
	logger := gmw.GetLogger(c)
	logger.Error("http server error", zap.Error(err))
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": err.Error(),
	})
}

// GetIp returns the first detected non-loopback IPv4 address for the host, preferring private ranges.
func GetIp() (ip string) {
	ips, err := net.InterfaceAddrs()
	if err != nil {
		logger.Logger.Error("failed to get IP addresses", zap.Error(err))
		return ip
	}

	for _, a := range ips {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip = ipNet.IP.String()
				if strings.HasPrefix(ip, "10") {
					return
				}
				if strings.HasPrefix(ip, "172") {
					return
				}
				if strings.HasPrefix(ip, "192.168") {
					return
				}
				ip = ""
			}
		}
	}
	return
}

var sizeKB = 1024
var sizeMB = sizeKB * 1024
var sizeGB = sizeMB * 1024

// Bytes2Size converts a byte count into a human-readable string using the largest appropriate unit.
func Bytes2Size(num int64) string {
	numStr := ""
	unit := "B"
	if num/int64(sizeGB) > 1 {
		numStr = fmt.Sprintf("%.2f", float64(num)/float64(sizeGB))
		unit = "GB"
	} else if num/int64(sizeMB) > 1 {
		numStr = fmt.Sprintf("%d", int(float64(num)/float64(sizeMB)))
		unit = "MB"
	} else if num/int64(sizeKB) > 1 {
		numStr = fmt.Sprintf("%d", int(float64(num)/float64(sizeKB)))
		unit = "KB"
	} else {
		numStr = fmt.Sprintf("%d", num)
	}
	return numStr + " " + unit
}

// Interface2String converts primitive types into their string representation, returning a default marker otherwise.
func Interface2String(inter any) string {
	switch inter := inter.(type) {
	case string:
		return inter
	case int:
		return fmt.Sprintf("%d", inter)
	case float64:
		return fmt.Sprintf("%f", inter)
	}
	return "Not Implemented"
}

// UnescapeHTML marks the string as trusted HTML so it bypasses escaping in templates.
func UnescapeHTML(x string) any {
	return template.HTML(x)
}

// IntMax returns the greater of the two integer arguments.
func IntMax(a int, b int) int {
	if a >= b {
		return a
	} else {
		return b
	}
}

// GenRequestID generates a request identifier combining time-based and random components.
func GenRequestID() string {
	return GetTimeString() + random.GetRandomNumberString(8)
}

// Removed std context ID extractors; callers must pass IDs explicitly via gin.Context

// GetResponseID formats the gin context request ID into an OpenAI-style response identifier.
func GetResponseID(c *gin.Context) string {
	logID := c.GetString(RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

// Max returns the greater of the two integer arguments.
func Max(a int, b int) int {
	if a >= b {
		return a
	} else {
		return b
	}
}

// AssignOrDefault returns value when non-empty, otherwise defaultValue.
func AssignOrDefault(value string, defaultValue string) string {
	if len(value) != 0 {
		return value
	}
	return defaultValue
}

// MessageWithRequestId appends the request identifier to the supplied message for log clarity.
func MessageWithRequestId(message string, id string) string {
	return fmt.Sprintf("%s (request id: %s)", message, id)
}

// String2Int converts the string into an integer, returning zero on parse failure.
func String2Int(str string) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return num
}

// Float64PtrMax caps the referenced float64 at maxValue while preserving nil pointers.
func Float64PtrMax(p *float64, maxValue float64) *float64 {
	if p == nil {
		return nil
	}
	if *p > maxValue {
		return &maxValue
	}
	return p
}

// Float64PtrMin raises the referenced float64 to at least minValue while preserving nil pointers.
func Float64PtrMin(p *float64, minValue float64) *float64 {
	if p == nil {
		return nil
	}
	if *p < minValue {
		return &minValue
	}
	return p
}
