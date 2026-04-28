package common

import (
	"os"
	"regexp"
	"strings"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

var windowsEnvPattern = regexp.MustCompile(`%([A-Za-z0-9_]+)%`)

// expandLogDirPath resolves environment variable placeholders in log directory paths.
func expandLogDirPath(path string) string {
	logger.Logger.Debug("expand log dir path", zap.String("path", path))
	if path == "" {
		return ""
	}

	expanded := os.ExpandEnv(path)

	expanded = windowsEnvPattern.ReplaceAllStringFunc(expanded, func(match string) string {
		key := strings.Trim(match, "%")
		if val, ok := os.LookupEnv(key); ok && val != "" {
			return val
		}
		switch key {
		case "DATA_DIR":
			return "/data"
		default:
			return match
		}
	})

	return expanded
}
