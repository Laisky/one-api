import { showError as reportUIError } from '../../utils/common';
import { useState, useEffect } from 'react';
import { showError, renderQuota } from 'utils/common';

import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableContainer from '@mui/material/TableContainer';
import PerfectScrollbar from 'react-perfect-scrollbar';
import TablePagination from '@mui/material/TablePagination';
import LinearProgress from '@mui/material/LinearProgress';
import ButtonGroup from '@mui/material/ButtonGroup';
import Toolbar from '@mui/material/Toolbar';

import { Button, Card, Stack, Container, Typography, Box, Alert, IconButton, Chip } from '@mui/material';
import { IconRefresh, IconSearch } from '@tabler/icons-react';
import LogTableRow from './component/TableRow';
import LogTableHead from './component/TableHead';
import TableToolBar from './component/TableToolBar';
import TracingModal from './component/TracingModal';
import { API } from 'utils/api';
import { isAdmin } from 'utils/common';
import { ITEMS_PER_PAGE } from 'constants';
import './LogTable.mobile.css';

/**
 * nextLogSort returns the sort selected by clicking a sortable log column.
 *
 * Parameters:
 *   - current: the committed sort, as { sortBy, sortOrder }.
 *   - columnKey: the clicked column.
 *
 * Return value: the next { sortBy, sortOrder }. Clicking the active column
 * toggles its direction; clicking another column sorts it descending.
 */
function nextLogSort(current, columnKey) {
  if (current.sortBy === columnKey) {
    return { sortBy: columnKey, sortOrder: current.sortOrder === 'asc' ? 'desc' : 'asc' };
  }
  return { sortBy: columnKey, sortOrder: 'desc' };
}

export default function Log() {
  let now = new Date();
  let sevenDaysAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);

  const originalKeyword = {
    p: 0,
    keyword: '',
    username: '',
    token_name: '',
    model_name: '',
    start_timestamp: Math.floor(sevenDaysAgo.getTime() / 1000),
    // Whole seconds: the server parses timestamps as integers.
    end_timestamp: Math.floor(new Date().getTime() / 1000) + 3600,
    type: 0,
    channel: ''
  };
  const [logs, setLogs] = useState([]);
  const [activePage, setActivePage] = useState(0);
  const [searching, setSearching] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState(originalKeyword);
  const [initPage, setInitPage] = useState(true);
  const [stat, setStat] = useState({ quota: 0 });
  const [showStat, setShowStat] = useState(false);
  const [isStatRefreshing, setIsStatRefreshing] = useState(false);
  const [sortBy, setSortBy] = useState('');
  const [sortOrder, setSortOrder] = useState('desc');
  const [sortLoading, setSortLoading] = useState(false);
  const [tracingModalOpen, setTracingModalOpen] = useState(false);
  const [selectedLogId, setSelectedLogId] = useState(null);
  const userIsAdmin = isAdmin();
  // The stat endpoint only understands the structured filters, so it cannot
  // describe a keyword result set. Suppress it instead of showing a total that
  // belongs to a different set of rows than the table displays.
  const keywordActive = (searchKeyword.keyword || '').trim() !== '';

  // loadLogs fetches one page of logs for the supplied query.
  //
  // Parameters:
  //   - startIdx: zero-based page index.
  //   - options.filters: the filters and keyword to request; defaults to the
  //     current form values.
  //   - options.sort: the { sortBy, sortOrder } to request; defaults to the
  //     committed sort.
  //
  // A handler that has just changed filters or sort must pass the new values,
  // because state set in the same event is not visible in this closure.
  //
  // Return values: none; errors are surfaced through showError.
  const loadLogs = async (startIdx, { filters = searchKeyword, sort = { sortBy, sortOrder } } = {}) => {
    try {
      setSearching(true);
      const query = { ...filters };

      query.p = startIdx;
      if (sort.sortBy) {
        query.sort_by = sort.sortBy;
        query.sort_order = sort.sortOrder;
      }
      if (!userIsAdmin) {
        delete query.username;
        delete query.channel;
      }

      // A non-empty keyword goes to the server-side log search endpoint, which
      // also resolves a pasted log/user/token UUID. Admins search every log,
      // regular users only their own.
      const keyword = (query.keyword || '').trim();
      let url;
      let params;
      if (keyword) {
        url = userIsAdmin ? '/api/log/search' : '/api/log/self/search';
        params = { keyword: keyword, p: startIdx, size: ITEMS_PER_PAGE };
        if (sort.sortBy) {
          params.sort = sort.sortBy;
          params.order = sort.sortOrder;
        }
      } else {
        url = userIsAdmin ? '/api/log/' : '/api/log/self';
        delete query.keyword;
        params = query;
      }
      const res = await API.get(url, { params: params });
      const { success, message, data } = res.data;
      if (success) {
        if (startIdx === 0) {
          setLogs(data);
        } else {
          let newLogs = [...logs];
          newLogs.splice(startIdx * ITEMS_PER_PAGE, data.length, ...data);
          setLogs(newLogs);
        }
      } else {
        showError(message);
      }
    } finally {
      setSearching(false);
    }
  };

  const onPaginationChange = (event, activePage) => {
    (async () => {
      if (activePage === Math.ceil(logs.length / ITEMS_PER_PAGE)) {
        // In this case we have to load more data and then append them.
        await loadLogs(activePage);
      }
      setActivePage(activePage);
    })();
  };

  /** handleSort applies the sort selected by clicking the supplied column and reloads the first page with that sort. */
  const handleSort = async (columnKey) => {
    // Prevent multiple sort requests
    if (sortLoading) return;

    // The request must use the selected sort, not the previous render's sort.
    const nextSort = nextLogSort({ sortBy, sortOrder }, columnKey);
    setSortBy(nextSort.sortBy);
    setSortOrder(nextSort.sortOrder);
    setActivePage(0);
    setSortLoading(true);

    try {
      await loadLogs(0, { sort: nextSort });
    } finally {
      setSortLoading(false);
    }
  };

  const searchLogs = async (event) => {
    event.preventDefault();
    await loadLogs(0);
    setActivePage(0);
    return;
  };

  const handleSearchKeyword = (event) => {
    setSearchKeyword({ ...searchKeyword, [event.target.name]: event.target.value });
  };

  const getLogStat = async () => {
    const query = { ...searchKeyword };
    delete query.p;
    // The stat endpoint only understands the structured filters.
    delete query.keyword;
    if (!userIsAdmin) {
      delete query.username;
      delete query.channel;
    }
    const url = userIsAdmin ? '/api/log/stat' : '/api/log/self/stat';
    const res = await API.get(url, { params: query });
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const handleStatRefresh = async () => {
    setIsStatRefreshing(true);
    try {
      await getLogStat();
    } finally {
      setIsStatRefreshing(false);
    }
  };

  const handleShowStat = async () => {
    if (!showStat) {
      await getLogStat();
    }
    setShowStat(!showStat);
  };

  // 处理刷新
  const handleRefresh = () => {
    setInitPage(true);
  };

  const handleRowClick = (logId) => {
    setSelectedLogId(logId);
    setTracingModalOpen(true);
  };

  const handleTracingModalClose = () => {
    setTracingModalOpen(false);
    setSelectedLogId(null);
  };

  // Mounting and refreshing load once with cleared filters. Resetting initPage
  // re-runs this effect, which must then do nothing; the cleared filters are
  // passed explicitly because setSearchKeyword has not committed yet.
  useEffect(() => {
    if (!initPage) {
      return;
    }
    setSearchKeyword(originalKeyword);
    setActivePage(0);
    loadLogs(0, { filters: originalKeyword })
      .then()
      .catch((reason) => {
        showError(reason);
      });
    setInitPage(false);
  }, [initPage]);

  return (
    <>
      <Stack direction="row" alignItems="center" justifyContent="space-between" mb={2.5}>
        <Typography variant="h4">日志</Typography>
      </Stack>

      {/* Quota Statistics Section */}
      <Card sx={{ mb: 2 }}>
        <Box sx={{ p: 2 }}>
          <Stack direction="row" alignItems="center" spacing={2}>
            <Typography variant="h6">
              使用详情（总配额：
              {keywordActive ? (
                <Typography variant="body2" component="span" color="text.secondary">
                  关键字搜索模式下不统计
                </Typography>
              ) : showStat ? (
                <>
                  <Chip
                    label={renderQuota(stat.quota)}
                    color="primary"
                    variant="outlined"
                    size="small"
                  />
                  <IconButton
                    size="small"
                    onClick={(...uiArgs) => handleStatRefresh(...uiArgs).catch(reportUIError)}
                    disabled={isStatRefreshing}
                    sx={{ ml: 1 }}
                    title="刷新配额数据"
                  >
                    <IconRefresh
                      width="16px"
                      style={{
                        animation: isStatRefreshing ? 'spin 1s linear infinite' : 'none'
                      }}
                    />
                  </IconButton>
                </>
              ) : (
                <Button
                  size="small"
                  variant="text"
                  onClick={(...uiArgs) => handleShowStat(...uiArgs).catch(reportUIError)}
                  sx={{ textTransform: 'none', color: 'text.secondary' }}
                >
                  点击查看
                </Button>
              )}
              ）
            </Typography>
          </Stack>
        </Box>
      </Card>
      <Card>
        <Box component="form" onSubmit={(...uiArgs) => searchLogs(...uiArgs).catch(reportUIError)} noValidate sx={{marginTop: 2}}>
          <TableToolBar filterName={searchKeyword} handleFilterName={handleSearchKeyword} userIsAdmin={userIsAdmin} />
        </Box>
        <Toolbar
          sx={{
            textAlign: 'right',
            height: 50,
            display: 'flex',
            justifyContent: 'space-between',
            p: (theme) => theme.spacing(0, 1, 0, 3)
          }}
        >
          <Container>
            <ButtonGroup variant="outlined" aria-label="outlined small primary button group" sx={{marginBottom: 2}}>
              <Button onClick={handleRefresh} startIcon={<IconRefresh width={'18px'} />}>
                刷新/清除搜索条件
              </Button>

              <Button onClick={(...uiArgs) => searchLogs(...uiArgs).catch(reportUIError)} startIcon={<IconSearch width={'18px'} />}>
                搜索
              </Button>
            </ButtonGroup>
          </Container>
        </Toolbar>
        {searching && <LinearProgress />}
        <PerfectScrollbar component="div">
          <TableContainer sx={{ overflow: 'unset' }}>
            <Table sx={{ minWidth: 800 }}>
              <LogTableHead
                userIsAdmin={userIsAdmin}
                sortBy={sortBy}
                sortOrder={sortOrder}
                sortLoading={sortLoading}
                onSort={handleSort}
              />
              <TableBody>
                {logs.slice(activePage * ITEMS_PER_PAGE, (activePage + 1) * ITEMS_PER_PAGE).map((row, index) => (
                  <LogTableRow item={row} key={row.uuid || row.id || index} userIsAdmin={userIsAdmin} onRowClick={handleRowClick} />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </PerfectScrollbar>
        <TablePagination
          page={activePage}
          component="div"
          count={logs.length + (logs.length % ITEMS_PER_PAGE === 0 ? 1 : 0)}
          rowsPerPage={ITEMS_PER_PAGE}
          onPageChange={onPaginationChange}
          rowsPerPageOptions={[ITEMS_PER_PAGE]}
        />
      </Card>

      <TracingModal
        open={tracingModalOpen}
        onClose={handleTracingModalClose}
        logId={selectedLogId}
      />
    </>
  );
}
