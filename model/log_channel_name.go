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
	if err := LOG_DB.Raw("SELECT id, name FROM channels WHERE id IN ?", ids).Scan(&rows).Error; err != nil {
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
