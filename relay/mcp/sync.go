package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
)

const defaultSyncTimeout = 20 * time.Second

// SyncServerTools fetches tools from the MCP server and stores them locally.
func SyncServerTools(ctx context.Context, server *model.MCPServer) (int, error) {
	if server == nil {
		return 0, errors.New("mcp server is nil")
	}

	client := NewStreamableHTTPClient(server, nil, defaultSyncTimeout)
	tools, err := client.ListTools(ctx)
	if err != nil {
		return 0, err
	}

	stored := make([]*model.MCPTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		schemaBytes, _ := json.Marshal(tool.InputSchema)
		stored = append(stored, &model.MCPTool{
			Name:        tool.Name,
			DisplayName: tool.Name,
			Description: tool.Description,
			InputSchema: string(schemaBytes),
			Status:      1,
		})
	}

	if err := model.UpsertMCPTools(server.Id, stored); err != nil {
		return 0, err
	}

	return len(stored), nil
}

// StartAutoSync triggers MCP server tool syncs on a periodic schedule.
func StartAutoSync(ctx context.Context) {
	log := logger.Logger
	if log == nil {
		return
	}

	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				servers, err := model.ListEnabledMCPServers()
				if err != nil {
					log.Error("failed to list mcp servers for sync", zap.Error(err))
					continue
				}
				now := time.Now().UTC().UnixMilli()
				for _, server := range servers {
					if server == nil || !server.AutoSyncEnabled {
						continue
					}
					intervalMs := int64(server.AutoSyncIntervalMinutes) * int64(time.Minute/time.Millisecond)
					if intervalMs <= 0 {
						intervalMs = int64(60 * time.Minute / time.Millisecond)
					}
					if server.LastSyncAt != 0 && now-server.LastSyncAt < intervalMs {
						continue
					}
					syncCtx, cancel := context.WithTimeout(ctx, defaultSyncTimeout)
					count, err := SyncServerTools(syncCtx, server)
					cancel()
					if err != nil {
						server.MarkSyncResult(false, err.Error())
						if updateErr := model.UpdateMCPServer(server); updateErr != nil {
							log.Error("failed to update mcp sync status", zap.Error(updateErr))
						}
						log.Warn("mcp auto sync failed", zap.Int("server_id", server.Id), zap.Error(err))
						continue
					}
					server.MarkSyncResult(true, "")
					if updateErr := model.UpdateMCPServer(server); updateErr != nil {
						log.Error("failed to update mcp sync status", zap.Error(updateErr))
					}
					log.Info("mcp auto sync succeeded", zap.Int("server_id", server.Id), zap.Int("tool_count", count))
				}
			}
		}
	}()
}
