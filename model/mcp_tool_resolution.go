package model

import (
	"context"
	"github.com/Laisky/errors/v2"
)

// ListMCPServerNamesForToolResolution reads only configured names, including
// disabled servers, in stable order. ctx bounds the read. It returns names or a
// wrapped DB error, without loading or decrypting any credentials or policy.
func ListMCPServerNamesForToolResolution(ctx context.Context) ([]string, error) {
	var rows []struct{ Name string }
	if err := DB.WithContext(ctx).Model(&MCPServer{}).Select("name").Order("id").Scan(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "list mcp server names for tool resolution")
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names, nil
}
