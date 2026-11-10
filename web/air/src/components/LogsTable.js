import { createLatestRequest } from '../../../legacy/latest-request.mjs';
import { showError as reportUIError } from '../helpers/utils';
import React, { useEffect, useState } from 'react';
import { API, copy, isAdmin, showError, showSuccess, timestamp2string } from '../helpers';

import { Avatar, Button, Form, Layout, Modal, Select, Space, Spin, Table, Tag, AutoComplete, IconButton } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { ITEMS_PER_PAGE } from '../constants';
import { renderNumber, renderQuota, stringToColor } from '../helpers/render';
import Paragraph from '@douyinfe/semi-ui/lib/es/typography/paragraph';
import TracingModal from './TracingModal';
import ResourceRefTooltip from './ResourceRefTooltip';
import './LogsTable.mobile.css';

const { Header } = Layout;



const MODE_OPTIONS = [{ key: 'all', text: '全部用户', value: 'all' }, { key: 'self', text: '当前用户', value: 'self' }];

/**
 * stableHash maps an arbitrary string to a non-negative integer so display
 * colours stay consistent for the same identifier across renders and reloads.
 *
 * Parameters:
 *   - value: any, coerced to string; undefined/null hash to 0.
 *
 * Return value: a non-negative 32-bit integer.
 */
function stableHash(value) {
  const str = value === undefined || value === null ? '' : String(value);
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = (hash * 31 + str.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

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

const colors = ['amber', 'blue', 'cyan', 'green', 'grey', 'indigo', 'light-blue', 'lime', 'orange', 'pink', 'purple', 'red', 'teal', 'violet', 'yellow'];

function renderType(type) {
  switch (type) {
    case 1:
      return <Tag color="cyan" size="large"> 充值 </Tag>;
    case 2:
      return <Tag color="lime" size="large"> 消费 </Tag>;
    case 3:
      return <Tag color="orange" size="large"> 管理 </Tag>;
    case 4:
      return <Tag color="purple" size="large"> 系统 </Tag>;
    case 5:
      return <Tag color="violet" size="large"> 测试 </Tag>;
    default:
      return <Tag color="black" size="large"> 未知 </Tag>;
  }
}

function renderIsStream(bool) {
  if (bool) {
    return <Tag color="blue" size="large">流</Tag>;
  } else {
    return <Tag color="purple" size="large">非流</Tag>;
  }
}

function renderUseTime(type) {
  const time = parseInt(type);
  if (time < 101) {
    return <Tag color="green" size="large"> {time} s </Tag>;
  } else if (time < 300) {
    return <Tag color="orange" size="large"> {time} s </Tag>;
  } else {
    return <Tag color="red" size="large"> {time} s </Tag>;
  }
}

const LogsTable = () => {
  /** getColumns builds table columns after state and sort helpers have initialized and returns their definitions. */
  const getColumns = () => [{
    title: '时间', dataIndex: 'timestamp2string'
  }, {
    title: '渠道',
    dataIndex: 'channel_uuid',
    className: isAdmin() ? 'tableShow' : 'tableHiddle',
    render: (text, record, index) => {
      const channelRef = record.channel_uuid;
      const channelLabel = record.channel_name || channelRef;
      return (isAdminUser ? record.type === 0 || record.type === 2 ? <div>
        <ResourceRefTooltip refId={channelRef} label="渠道 ID">
          <Tag color={colors[stableHash(channelRef) % colors.length]} size="large"> {channelLabel} </Tag>
        </ResourceRefTooltip>
      </div> : <></> : <></>);
    }
  }, {
    title: '用户',
    dataIndex: 'username',
    className: isAdmin() ? 'tableShow' : 'tableHiddle',
    render: (text, record, index) => {
      return (isAdminUser ? <div>
        <Avatar size="small" color={stringToColor(text)} style={{ marginRight: 4 }}
          onClick={() => showUserInfo(record.user_uuid || record.user_id).catch(reportUIError)}>
          {typeof text === 'string' && text.slice(0, 1)}
        </Avatar>
        <ResourceRefTooltip refId={record.user_uuid || record.user_id} label="用户 ID">{text}</ResourceRefTooltip>
      </div> : <></>);
    }
  }, {
    title: '令牌', dataIndex: 'token_name', render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        <ResourceRefTooltip refId={record.token_uuid} label="令牌 ID">
          <Tag color="grey" size="large" onClick={() => {
            copyText(text).catch(reportUIError);
          }}> {text} </Tag>
        </ResourceRefTooltip>
      </div> : <></>);
    }
  }, {
    title: '类型', dataIndex: 'type', render: (text, record, index) => {
      return (<div>
        {renderType(text)}
      </div>);
    }
  }, {
    title: '模型', dataIndex: 'model_name', render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        <Tag color={stringToColor(text)} size="large" onClick={() => {
          copyText(text).catch(reportUIError);
        }}> {text} </Tag>
      </div> : <></>);
    }
  },
  // {
  //   title: '用时', dataIndex: 'use_time', render: (text, record, index) => {
  //     return (<div>
  //       <Space>
  //         {renderUseTime(text)}
  //         {renderIsStream(record.is_stream)}
  //       </Space>
  //     </div>);
  //   }
  // },
  {
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('prompt_tokens').catch(reportUIError)}>
      提示{getSortIcon('prompt_tokens')}
      {sortLoading && sortBy === 'prompt_tokens' && <span> ⏳</span>}
    </span>,
    dataIndex: 'prompt_tokens',
    render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        {<span> {text} </span>}
      </div> : <></>);
    }
  }, {
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('completion_tokens').catch(reportUIError)}>
      补全{getSortIcon('completion_tokens')}
      {sortLoading && sortBy === 'completion_tokens' && <span> ⏳</span>}
    </span>,
    dataIndex: 'completion_tokens',
    render: (text, record, index) => {
      return (parseInt(text) > 0 && (record.type === 0 || record.type === 2) ? <div>
        {<span> {text} </span>}
      </div> : <></>);
    }
  }, {
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('quota').catch(reportUIError)}>
      花费{getSortIcon('quota')}
      {sortLoading && sortBy === 'quota' && <span> ⏳</span>}
    </span>,
    dataIndex: 'quota',
    render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        {renderQuota(text, 6)}
      </div> : <></>);
    }
  }, {
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('elapsed_time').catch(reportUIError)}>
      Latency{getSortIcon('elapsed_time')}
      {sortLoading && sortBy === 'elapsed_time' && <span> ⏳</span>}
    </span>,
    dataIndex: 'elapsed_time',
    render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        {text ? `${text} ms` : ''}
      </div> : <></>);
    }
  }, {
    title: '详情', dataIndex: 'content', render: (text, record, index) => {
      return <Paragraph ellipsis={{ rows: 2, showTooltip: { type: 'popover', opts: { style: { width: 240 } } } }}
        style={{ maxWidth: 240 }}>
        {text}
      </Paragraph>;
    }
  }];

  const [logRequests] = useState(createLatestRequest);
  useEffect(logRequests.mount, [logRequests]);
  const [logs, setLogs] = useState([]);
  const [showStat, setShowStat] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingPage, setLoadingPage] = useState(false);
  const [loadingStat, setLoadingStat] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(ITEMS_PER_PAGE);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searching, setSearching] = useState(false);
  const [logType, setLogType] = useState(0);
  const isAdminUser = isAdmin();
  let now = new Date();
  // 初始化start_timestamp为7天前
  let sevenDaysAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
  const [inputs, setInputs] = useState({
    username: '',
    token_name: '',
    model_name: '',
    start_timestamp: timestamp2string(sevenDaysAgo.getTime() / 1000),
    end_timestamp: timestamp2string(now.getTime() / 1000 + 3600),
    channel: ''
  });
  const [sortBy, setSortBy] = useState('');
  const [sortOrder, setSortOrder] = useState('desc');
  const [sortLoading, setSortLoading] = useState(false);
  const { username, token_name, model_name, start_timestamp, end_timestamp, channel } = inputs;

  const [stat, setStat] = useState({
    quota: 0, token: 0
  });
  const [isStatRefreshing, setIsStatRefreshing] = useState(false);
  const [userOptions, setUserOptions] = useState([]);
  const [userSearchLoading, setUserSearchLoading] = useState(false);
  const [tracingModalVisible, setTracingModalVisible] = useState(false);
  const [selectedLogId, setSelectedLogId] = useState(null);
  // activeSearch holds the keyword the currently displayed rows were produced
  // with. It is '' whenever the table shows the structured-filter listing, so
  // pagination knows which endpoint it must keep calling.
  const [activeSearch, setActiveSearch] = useState('');
  const keywordActive = searchKeyword.trim() !== '';

  const handleInputChange = (value, name) => {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const searchUsers = async (searchQuery) => {
    if (!searchQuery.trim()) {
      setUserOptions([]);
      return;
    }

    setUserSearchLoading(true);
    try {
      const res = await API.get(`/api/user/search?keyword=${searchQuery}`);
      const { success, data } = res.data;
      if (success) {
        const options = data.map(user => ({
          value: user.username,
          label: `${user.display_name || user.username} (@${user.username})`,
          username: user.username,
          display_name: user.display_name,
          uuid: user.uuid
        }));
        setUserOptions(options);
      }
    } catch (error) {
      console.error('Failed to search users:', error);
    } finally {
      setUserSearchLoading(false);
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

  const getLogSelfStat = async () => {
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let res = await API.get(`/api/log/self/stat?type=${logType}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const getLogStat = async () => {
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let res = await API.get(`/api/log/stat?type=${logType}&username=${username}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&channel=${channel}`);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const handleEyeClick = async () => {
    setLoadingStat(true);
    if (isAdminUser) {
      await getLogStat();
    } else {
      await getLogSelfStat();
    }
    setShowStat(true);
    setLoadingStat(false);
  };

  const showUserInfo = async (userId) => {
    if (!isAdminUser) {
      return;
    }
    const res = await API.get(`/api/user/${userId}`);
    const { success, message, data } = res.data;
    if (success) {
      Modal.info({
        title: '用户信息', content: <div style={{ padding: 12 }}>
          <p>用户名: {data.username}</p>
          <p>余额: {renderQuota(data.quota)}</p>
          <p>已用额度：{renderQuota(data.used_quota)}</p>
          <p>请求次数：{renderNumber(data.request_count)}</p>
        </div>, centered: true
      });
    } else {
      showError(message);
    }
  };

  // setLogsFormat decorates rows for display and refreshes the pager total.
  //
  // Parameters:
  //   - logs: the rows to display.
  //   - total: the authoritative row count when the caller knows it (the search
  //     endpoint returns one). Omit it for the listing endpoint, which has no
  //     count, so the pager keeps its optimistic "one more page" behaviour.
  const setLogsFormat = (logs, total) => {
    for (let i = 0; i < logs.length; i++) {
      const fullTimestamp = timestamp2string(logs[i].created_at);
      // Extract MM-DD HH:MM:SS from YYYY-MM-DD HH:MM:SS for compact display
      const compactTimestamp = fullTimestamp.slice(5); // Remove YYYY- part
      logs[i].timestamp2string = (
        <span title={fullTimestamp}>
          {compactTimestamp}
        </span>
      );
      logs[i].key = '' + (logs[i].uuid ?? logs[i].id ?? i);
    }
    // data.key = '' + data.id
    setLogs(logs);
    setLogCount(total === undefined ? logs.length + ITEMS_PER_PAGE : total);
    // console.log(logCount);
  };

  // loadLogs fetches one page from the structured log listing.
  //
  // Parameters:
  //   - startIdx: zero-based page index.
  //   - pageSize: page size.
  //   - localLogType: the log type to list; defaults to the selected type.
  //   - sort: the { sortBy, sortOrder } to request; defaults to the committed
  //     sort. A handler that has just changed the sort must pass the new value,
  //     because state set in the same event is not visible in this closure.
  //   - options.sorting: whether this request was initiated by a sort action.
  //   - options.pagination: whether this request extends the current query cache.
  //
  // Return values: none; errors are surfaced through showError.
  const loadLogs = async (startIdx, pageSize, localLogType = logType, sort = { sortBy, sortOrder }, { sorting = false, pagination = false } = {}) => {
    const isCurrent = logRequests.begin();
    setLoading(true);
    setSearching(false);
    setSortLoading(sorting);
    setLoadingPage(pagination);
    try {

      let url = '';
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;
      let sortParams = '';
      if (sort.sortBy) {
        sortParams = `&sort_by=${sort.sortBy}&sort_order=${sort.sortOrder}`;
      }
      // Both Go listing handlers read `size`; `page_size` is silently ignored.
      if (isAdminUser) {
        url = `/api/log/?p=${startIdx}&size=${pageSize}&type=${localLogType}&username=${username}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&channel=${channel}${sortParams}`;
      } else {
        url = `/api/log/self?p=${startIdx}&size=${pageSize}&type=${localLogType}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}${sortParams}`;
      }
      const res = await API.get(url);
      if (!isCurrent()) return;
      const { success, message, data } = res.data;
      if (success) {
        // Leaving search mode: the rows now come from the structured listing.
        setActiveSearch('');
        if (startIdx === 0) {
          setLogsFormat(data);
        } else {
          let newLogs = [...logs];
          newLogs.splice(startIdx * pageSize, data.length, ...data);
          setLogsFormat(newLogs);
        }
      } else {
        showError(message);
      }
    } catch (error) {
      if (isCurrent()) throw error;
    } finally {
      if (isCurrent()) {
        setLoading(false);
        setLoadingPage(false);
        setSortLoading(false);
      }
    }
  };

  // loadSearchPage fetches one page from the keyword search endpoint and keeps
  // the table in search mode, so paging never silently falls back to the
  // unfiltered listing.
  //
  // Parameters:
  //   - keyword: the non-empty keyword to search with.
  //   - startIdx: zero-based page index.
  //   - size: page size.
  //   - sort: the { sortBy, sortOrder } to request; defaults to the committed sort.
  //   - options.sorting: whether this request was initiated by a sort action.
  //   - options.pagination: whether this request extends the current query cache.
  //
  // Return values: none; errors are surfaced through showError.
  const loadSearchPage = async (keyword, startIdx, size, sort = { sortBy, sortOrder }, { sorting = false, pagination = false } = {}) => {
    const isCurrent = logRequests.begin();
    setSearching(true);
    setLoading(false);
    setSortLoading(sorting);
    setLoadingPage(pagination);
    try {
      const url = isAdminUser ? '/api/log/search' : '/api/log/self/search';
      let sortParams = '';
      if (sort.sortBy) {
        sortParams = `&sort=${sort.sortBy}&order=${sort.sortOrder}`;
      }
      const res = await API.get(`${url}?keyword=${encodeURIComponent(keyword)}&p=${startIdx}&size=${size}${sortParams}`);
      if (!isCurrent()) return;
      const { success, message, data, total } = res.data;
      if (success) {
        setActiveSearch(keyword);
        if (startIdx === 0) {
          setLogsFormat(data, total);
        } else {
          let newLogs = [...logs];
          newLogs.splice(startIdx * size, data.length, ...data);
          setLogsFormat(newLogs, total);
        }
      } else {
        showError(message);
      }
    } catch (error) {
      if (isCurrent()) throw error;
    } finally {
      if (isCurrent()) {
        setSearching(false);
        setLoadingPage(false);
        setSortLoading(false);
      }
    }
  };

  const pageData = logs.slice((activePage - 1) * pageSize, activePage * pageSize);

  const handlePageChange = page => {
    if ((loading || searching) && !loadingPage) return;
    setActivePage(page);
    if (page === Math.ceil(logs.length / pageSize) + 1) {
      // In this case we have to load more data and then append them.
      if (activeSearch !== '') {
        loadSearchPage(activeSearch, page - 1, pageSize, undefined, { pagination: true }).then(r => {
        }).catch(reportUIError);
        return;
      }
      loadLogs(page - 1, pageSize, logType, undefined, { pagination: true }).then(r => {
      }).catch(reportUIError);
    }
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    const reload = activeSearch !== '' ? loadSearchPage(activeSearch, 0, size) : loadLogs(0, size);
    reload
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  /** handleSort applies the sort selected by clicking the supplied column and reloads the first page with that sort. */
  const handleSort = async (columnKey) => {
    // Prevent multiple sort requests
    if (sortLoading) return;

    // The request must use the selected sort, not the previous render's sort.
    const nextSort = nextLogSort({ sortBy, sortOrder }, columnKey);
    setSortBy(nextSort.sortBy);
    setSortOrder(nextSort.sortOrder);
    setActivePage(1);
    if (activeSearch !== '') {
      await loadSearchPage(activeSearch, 0, pageSize, nextSort, { sorting: true });
    } else {
      await loadLogs(0, pageSize, logType, nextSort, { sorting: true });
    }
  };

  /** getSortIcon returns a stable element for the active column's direction, or null for an inactive column. */
  const getSortIcon = (columnKey) => {
    if (columnKey !== sortBy) {
      return null;
    }
    return <span>{sortOrder === 'asc' ? ' ↑' : ' ↓'}</span>;
  };

  /** refresh reloads the first listing page for the supplied log type, defaulting to the selected type. */
  const refresh = async (localLogType = logType) => {
    setActivePage(1);
    await loadLogs(0, pageSize, localLogType);
  };

  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess('已复制：' + text);
    } else {
      // setSearchKeyword(text);
      Modal.error({ title: '无法复制到剪贴板，请手动复制', content: text });
    }
  };

  const handleRowClick = (record) => {
    setSelectedLogId(record.uuid || record.id);
    setTracingModalVisible(true);
  };

  const handleTracingModalClose = () => {
    setTracingModalVisible(false);
    setSelectedLogId(null);
  };

  useEffect(() => {
    // console.log('default effect')
    const localPageSize = parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(0, localPageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, []);

  // searchLogs resolves the keyword box against the server-side log search
  // endpoint, which also matches a pasted log/user/token UUID. Admins search
  // every log, regular users only their own.
  const searchLogs = async () => {
    const keyword = searchKeyword.trim();
    setActivePage(1);
    if (keyword === '') {
      // if keyword is blank, load logs instead.
      await loadLogs(0, pageSize, logType);
      return;
    }
    await loadSearchPage(keyword, 0, pageSize);
  };

  return (<>
    <Layout className="logs-layout">
      <Header className="logs-header">
        <Spin spinning={loadingStat}>
          <h3>使用明细（总消耗额度：
            <span onClick={(...uiArgs) => handleEyeClick(...uiArgs).catch(reportUIError)} style={{
              cursor: 'pointer', color: 'gray'
            }}>{showStat ? renderQuota(stat.quota) : '点击查看'}</span>
            {showStat && (
              <IconButton
                icon={<IconRefresh />}
                size="small"
                onClick={(...uiArgs) => handleStatRefresh(...uiArgs).catch(reportUIError)}
                loading={isStatRefreshing}
                disabled={isStatRefreshing}
                style={{ marginLeft: '8px' }}
                theme="borderless"
                title="刷新配额数据"
              />
            )}
            ）
          </h3>
          {keywordActive && (
            <div style={{ color: 'var(--semi-color-warning)', fontSize: 12, marginTop: -8 }}>
              关键字搜索模式：额度统计仍按下方筛选条件计算，不含关键字。
            </div>
          )}
        </Spin>
      </Header>
      <Form layout="horizontal" className="logs-form" style={{ marginTop: 10 }}>
        <>
          <Form.Input field="token_name" label="令牌名称" style={{ width: 176 }} value={token_name}
            placeholder={'可选值'} name="token_name" disabled={keywordActive}
            onChange={value => handleInputChange(value, 'token_name')} />
          <Form.Input field="model_name" label="模型名称" style={{ width: 176 }} value={model_name}
            placeholder="可选值"
            name="model_name" disabled={keywordActive}
            onChange={value => handleInputChange(value, 'model_name')} />
          <Form.DatePicker field="start_timestamp" label="起始时间" style={{ width: 272 }}
            initValue={start_timestamp}
            value={start_timestamp} type="dateTime"
            name="start_timestamp" disabled={keywordActive}
            onChange={value => handleInputChange(value, 'start_timestamp')} />
          <Form.DatePicker field="end_timestamp" fluid label="结束时间" style={{ width: 272 }}
            initValue={end_timestamp}
            value={end_timestamp} type="dateTime"
            name="end_timestamp" disabled={keywordActive}
            onChange={value => handleInputChange(value, 'end_timestamp')} />
          {isAdminUser && <>
            <Form.Input field="channel" label="渠道 UUID" style={{ width: 176 }} value={channel}
              placeholder="可选，渠道 UUID" name="channel" disabled={keywordActive}
              onChange={value => handleInputChange(value, 'channel')} />
            <Form.Slot label="用户名称">
              <AutoComplete
                style={{ width: 176 }}
                data={userOptions}
                value={username}
                placeholder="搜索用户名称"
                disabled={keywordActive}
                onSearch={(...uiArgs) => searchUsers(...uiArgs).catch(reportUIError)}
                onChange={value => handleInputChange(value, 'username')}
                loading={userSearchLoading}
                emptyContent="未找到用户"
                renderSelectedItem={(option) => option.username || option.value}
                renderItem={(option) => (
                  <div style={{ display: 'flex', flexDirection: 'column', padding: '4px 0' }}>
                    <div style={{ fontWeight: 'bold' }}>
                      {option.display_name || option.username}
                    </div>
                    <div style={{ fontSize: '12px', color: '#666' }}>
                      @{option.username}{option.uuid ? ` • ID: ${String(option.uuid).slice(0, 8)}` : ''}
                    </div>
                  </div>
                )}
              />
            </Form.Slot>
          </>}
          <Form.Input field="search_keyword" label="关键字" style={{ width: 272 }} value={searchKeyword}
            placeholder="按 UUID 或详情搜索 ..."
            extraText={keywordActive ? '关键字搜索会忽略上方筛选条件与日志类型。' : ''}
            name="search_keyword"
            onChange={value => setSearchKeyword(value)}
            onEnterPress={() => {
              searchLogs().then().catch(reportUIError);
            }} />
          <Form.Section>
            <Button label="查询" type="primary" htmlType="submit" className="btn-margin-right"
              onClick={() => refresh().catch(reportUIError)} loading={loading} disabled={keywordActive}>查询</Button>
            <Button label="搜索" type="tertiary" className="btn-margin-right"
              onClick={() => {
                searchLogs().then().catch(reportUIError);
              }} loading={searching}>搜索</Button>
          </Form.Section>
        </>
      </Form>
      <Table
        className="logs-table"
        style={{ marginTop: 5 }}
        columns={getColumns()}
        dataSource={pageData}
        onRow={(record) => ({
          onClick: () => handleRowClick(record),
          style: { cursor: 'pointer' }
        })}
        pagination={{
          disabled: (loading || searching) && !loadingPage,
          currentPage: activePage,
          pageSize: pageSize,
          total: logCount,
          pageSizeOpts: [10, 20, 50, 100],
          showSizeChanger: true,
          onPageSizeChange: (size) => {
            handlePageSizeChange(size).then().catch(reportUIError);
          },
          onPageChange: handlePageChange
        }} />
      <Select className="logs-select" defaultValue="0" style={{ width: 120 }} disabled={keywordActive} onChange={(value) => {
        setLogType(parseInt(value));
        refresh(parseInt(value)).then().catch(reportUIError);
      }}>
        <Select.Option value="0">全部</Select.Option>
        <Select.Option value="1">充值</Select.Option>
        <Select.Option value="2">消费</Select.Option>
        <Select.Option value="3">管理</Select.Option>
        <Select.Option value="4">系统</Select.Option>
      </Select>

      <TracingModal
        visible={tracingModalVisible}
        onCancel={handleTracingModalClose}
        logId={selectedLogId}
      />
    </Layout>
  </>);
};

export default LogsTable;
