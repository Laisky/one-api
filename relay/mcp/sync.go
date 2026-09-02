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

// SyncServerTools fetches a complete upstream catalog and atomically stores lossless descriptors.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - server: the configured upstream MCP server to synchronize.
//
// Return values:
//   - int: the number of valid tools stored in the replacement catalog.
//   - error: a wrapped client, encoding, or database error.
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
				return 0, errors.Wrapf(err, "marshal input schema for mcp tool %q", tool.Name)
			}
			inputSchema = string(schemaBytes)
		}
		descriptorBytes, err := json.Marshal(tool)
		if err != nil {
			return 0, errors.Wrapf(err, "marshal complete descriptor for mcp tool %q", tool.Name)
		}
		displayName := tool.Title
		if displayName == "" {
			displayName = tool.Name
		}
		stored = append(stored, &model.MCPTool{
			Name:           tool.Name,
			DisplayName:    displayName,
			Description:    tool.Description,
			InputSchema:    inputSchema,
			DescriptorJSON: string(descriptorBytes),
			Status:         1,
		})
	}

	if err := model.UpsertMCPTools(server.Id, server.UUID, stored); err != nil {
		return 0, identity.Tag(
			errors.Wrapf(err, "upsert mcp tools for server %d", server.Id),
			server.Ref())
	}
	return len(stored), nil
}

// StartAutoSync starts the periodic MCP catalog synchronization loop for enabled servers.
//
// Parameters:
//   - ctx: the process context controlling worker shutdown.
//
// Return values: none; the worker logs each background result with the server identity.
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
