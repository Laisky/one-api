import PropTypes from 'prop-types';

import { TableRow, TableCell } from '@mui/material';

import { timestamp2string, renderQuota } from 'utils/common';
import Label from 'ui-component/Label';
import LogType from '../type/LogType';

function renderType(type) {
  const typeOption = LogType[type];
  if (typeOption) {
    return (
      <Label variant="filled" color={typeOption.color}>
        {' '}
        {typeOption.text}{' '}
      </Label>
    );
  } else {
    return (
      <Label variant="filled" color="error">
        {' '}
        未知{' '}
      </Label>
    );
  }
}

export default function LogTableRow({ item, userIsAdmin, onRowClick }) {
  const fullTimestamp = timestamp2string(item.created_at);
  // Extract MM-DD HH:MM:SS from YYYY-MM-DD HH:MM:SS for compact display
  const compactTimestamp = fullTimestamp.slice(5); // Remove YYYY- part

  return (
    <>
      <TableRow
        tabIndex={item.id}
        sx={{ cursor: 'pointer', '&:hover': { backgroundColor: 'action.hover' } }}
        onClick={() => onRowClick && onRowClick(item.uuid || item.id)}
      >
        <TableCell data-label="时间" title={fullTimestamp}>{compactTimestamp}</TableCell>

        {userIsAdmin && <TableCell data-label="渠道">{item.channel || ''}</TableCell>}
        {userIsAdmin && (
          <TableCell data-label="用户">
            <Label color="default" variant="outlined">
              {item.username}
            </Label>
          </TableCell>
        )}
        <TableCell data-label="令牌">
          {item.token_name && (
            <Label color="default" variant="soft">
              {item.token_name}
            </Label>
          )}
        </TableCell>
        <TableCell data-label="类型">{renderType(item.type)}</TableCell>
        <TableCell data-label="模型">
          {item.model_name && (
            <Label color="primary" variant="outlined">
              {item.model_name}
            </Label>
          )}
        </TableCell>
        <TableCell data-label="提示">{item.prompt_tokens || ''}</TableCell>
        <TableCell data-label="完成">{item.completion_tokens || ''}</TableCell>
        <TableCell data-label="配额">{item.quota ? renderQuota(item.quota, 6) : ''}</TableCell>
        <TableCell data-label="用时">{item.elapsed_time ? `${item.elapsed_time} ms` : ''}</TableCell>
        <TableCell data-label="详情">{item.content}</TableCell>
      </TableRow>
    </>
  );
}

LogTableRow.propTypes = {
  item: PropTypes.object,
  userIsAdmin: PropTypes.bool
};
