import PropTypes from 'prop-types';
import { TableCell, TableHead, TableRow } from '@mui/material';

const LogTableHead = ({ userIsAdmin, sortBy, sortOrder, sortLoading, onSort }) => {
  /** getSortIcon returns a stable element for the active column's direction, or null for an inactive column. */
  const getSortIcon = (columnKey) => {
    if (columnKey !== sortBy) {
      return null;
    }
    return <span>{sortOrder === 'asc' ? ' ↑' : ' ↓'}</span>;
  };

  return (
    <TableHead>
      <TableRow>
        <TableCell>时间</TableCell>
        {userIsAdmin && <TableCell>渠道</TableCell>}
        {userIsAdmin && <TableCell>用户</TableCell>}
        <TableCell>令牌</TableCell>
        <TableCell>类型</TableCell>
        <TableCell>模型</TableCell>
        <TableCell
          style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }}
          onClick={() => onSort('prompt_tokens')}
        >
          提示{getSortIcon('prompt_tokens')}
          {sortLoading && sortBy === 'prompt_tokens' && <span> ⏳</span>}
        </TableCell>
        <TableCell
          style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }}
          onClick={() => onSort('completion_tokens')}
        >
          补全{getSortIcon('completion_tokens')}
          {sortLoading && sortBy === 'completion_tokens' && <span> ⏳</span>}
        </TableCell>
        <TableCell
          style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }}
          onClick={() => onSort('quota')}
        >
          费用{getSortIcon('quota')}
          {sortLoading && sortBy === 'quota' && <span> ⏳</span>}
        </TableCell>
        <TableCell
          style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }}
          onClick={() => onSort('elapsed_time')}
        >
          Latency{getSortIcon('elapsed_time')}
          {sortLoading && sortBy === 'elapsed_time' && <span> ⏳</span>}
        </TableCell>
        <TableCell>详情</TableCell>
      </TableRow>
    </TableHead>
  );
};

export default LogTableHead;

LogTableHead.propTypes = {
  userIsAdmin: PropTypes.bool,
  sortBy: PropTypes.string,
  sortOrder: PropTypes.string,
  onSort: PropTypes.func
};
