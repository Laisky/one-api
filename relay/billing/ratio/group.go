package ratio

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

var groupRatioLock sync.RWMutex
var GroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

func GroupRatio2JSONString() string {
	jsonBytes, err := json.Marshal(GroupRatio)
	if err != nil {
		logger.Logger.Error("error marshalling model ratio", zap.Error(err))
	}
	return string(jsonBytes)
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	groupRatioLock.Lock()
	defer groupRatioLock.Unlock()
	GroupRatio = make(map[string]float64)
	return json.Unmarshal([]byte(jsonStr), &GroupRatio)
}

func GetGroupRatio(name string) float64 {
	return GetGroupRatioWithContext(context.Background(), name)
}

// GetGroupRatioWithContext returns a group ratio and includes request context in diagnostics.
// Parameters: ctx carries the request logger, and name identifies the group.
// Returns: the configured group ratio, or one when the group is unknown.
func GetGroupRatioWithContext(ctx context.Context, name string) float64 {
	groupRatioLock.RLock()
	defer groupRatioLock.RUnlock()
	ratio, ok := GroupRatio[name]
	if !ok {
		logger.FromContext(ctx).Error("group ratio not found", zap.String("group", name))
		return 1
	}
	return ratio
}
