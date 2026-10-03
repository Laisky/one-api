package model

import "github.com/Laisky/errors/v2"

// fillLogChannelNames attaches current channel names to logs in a single batched query.
// Parameters:
//   - logs: log rows whose ChannelName fields should be populated from ChannelId.
//
// Return values:
//   - error: wrapped database lookup error, if the channel-name lookup fails.
func fillLogChannelNames(logs []*Log) error {
	if len(logs) == 0 {
		return nil
	}

	ids := make([]int, 0, len(logs))
	seen := make(map[int]struct{}, len(logs))
	for _, log := range logs {
		if log == nil || log.ChannelId <= 0 {
			continue
		}
		if _, ok := seen[log.ChannelId]; ok {
			continue
		}
		seen[log.ChannelId] = struct{}{}
		ids = append(ids, log.ChannelId)
	}
	if len(ids) == 0 {
		return nil
	}

	type channelNameRow struct {
		Id   int
		Name string
	}
	rows := make([]channelNameRow, 0, len(ids))
	// Channels live on the PRIMARY handle. migrateLOGDB creates only Log and
	// DataMigration on LOG_DB, so a deployment that points LOG_SQL_DSN at a
	// separate database has no channels table there and this lookup failed the
	// whole log list with "query channel names for logs". Logs are read from
	// LOG_DB; the names decorating them are resolved where they actually live.
	if err := DB.Raw("SELECT id, name FROM channels WHERE id IN ?", ids).Scan(&rows).Error; err != nil {
		return errors.Wrap(err, "query channel names for logs")
	}

	names := make(map[int]string, len(rows))
	for _, row := range rows {
		names[row.Id] = row.Name
	}
	for _, log := range logs {
		if log == nil {
			continue
		}
		log.ChannelName = names[log.ChannelId]
	}
	return nil
}
