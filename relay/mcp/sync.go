package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

const defaultSyncTimeout = 20 * time.Second

// SyncServerTools fetches tools from the MCP server and stores them locally.
func SyncServerTools(ctx context.Context, server *model.MCPServer) (int, error) {
	if server == nil {
		return 0, errors.New("mcp server is nil")
	}

	client := NewStreamableHTTPClient(server, nil, defaultSyncTimeout)
	tools, err := client.ListToolsLatest(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "list mcp tools from server")
	}

	stored := make([]*model.MCPTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		inputSchema := ""
		if tool.InputSchema != nil {
			schemaBytes, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return 0, errors.Wrap(err, "marshal mcp tool schema")
			}
			if string(schemaBytes) != "null" {
				inputSchema = string(schemaBytes)
			}
		}
		displayName := tool.Title
		if displayName == "" {
			displayName = tool.Name
		}
		stored = append(stored, &model.MCPTool{
			Name:        tool.Name,
			DisplayName: displayName,
			Description: tool.Description,
			InputSchema: inputSchema,
			Status:      1,
		})
	}

	if err := model.UpsertMCPTools(server.Id, server.UUID, stored); err != nil {
		return 0, identity.Tag(
			errors.Wrapf(err, "upsert mcp tools for server %d", server.Id),
			server.Ref())
	}

	return len(stored), nil
}

// StartAutoSync triggers MCP server tool syncs on a periodic schedule.
func StartAutoSync(ctx context.Context) {
	log := logger.FromContext(ctx)
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
					// This is a background job: the logger is not request-bound,
					// so every line must carry the MCP server identity explicitly.
					serverRef := server.Ref()
					if err != nil {
						server.MarkSyncResult(false, err.Error())
						if updateErr := model.UpdateMCPServer(server); updateErr != nil {
							log.Error("failed to update mcp sync status",
								append(serverRef.Zap(), zap.Error(updateErr))...)
						}
						log.Warn("mcp auto sync failed", append(serverRef.Zap(), zap.Error(err))...)
						continue
					}
					server.MarkSyncResult(true, "")
					if updateErr := model.UpdateMCPServer(server); updateErr != nil {
						log.Error("failed to update mcp sync status",
							append(serverRef.Zap(), zap.Error(updateErr))...)
					}
					log.Info("mcp auto sync succeeded",
						append(serverRef.Zap(), zap.Int("tool_count", count))...)
				}
			}
		}
	}()
}
