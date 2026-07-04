import React, { useEffect, useState } from 'react';
import { API, copy, isAdmin, showError, showSuccess, timestamp2string } from '../helpers';

import { Avatar, Button, Form, Layout, Modal, Select, Space, Spin, Table, Tag, AutoComplete, IconButton } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { ITEMS_PER_PAGE } from '../constants';
import { renderNumber, renderQuota, stringToColor } from '../helpers/render';
import Paragraph from '@douyinfe/semi-ui/lib/es/typography/paragraph';
import TracingModal from './TracingModal';
import './LogsTable.mobile.css';

const { Header } = Layout;



const MODE_OPTIONS = [{ key: 'all', text: '全部用户', value: 'all' }, { key: 'self', text: '当前用户', value: 'self' }];

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
  const columns = [{
    title: '时间', dataIndex: 'timestamp2string'
  }, {
    title: '渠道',
    dataIndex: 'channel',
    className: isAdmin() ? 'tableShow' : 'tableHiddle',
    render: (text, record, index) => {
      return (isAdminUser ? record.type === 0 || record.type === 2 ? <div>
        {<Tag color={colors[parseInt(text) % colors.length]} size="large"> {text} </Tag>}
      </div> : <></> : <></>);
    }
  }, {
    title: '用户',
    dataIndex: 'username',
    className: isAdmin() ? 'tableShow' : 'tableHiddle',
    render: (text, record, index) => {
      return (isAdminUser ? <div>
        <Avatar size="small" color={stringToColor(text)} style={{ marginRight: 4 }}
          onClick={() => showUserInfo(record.user_uuid || record.user_id)}>
          {typeof text === 'string' && text.slice(0, 1)}
        </Avatar>
        {text}
      </div> : <></>);
    }
  }, {
    title: '令牌', dataIndex: 'token_name', render: (text, record, index) => {
      return (record.type === 0 || record.type === 2 ? <div>
        <Tag color="grey" size="large" onClick={() => {
          copyText(text);
        }}> {text} </Tag>
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
          copyText(text);
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
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('prompt_tokens')}>
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
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('completion_tokens')}>
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
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('quota')}>
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
    title: <span style={{ cursor: sortLoading ? 'wait' : 'pointer', opacity: sortLoading ? 0.6 : 1 }} onClick={() => handleSort('elapsed_time')}>
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

  const [logs, setLogs] = useState([]);
  const [showStat, setShowStat] = useState(false);
  const [loading, setLoading] = useState(false);
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
          id: user.id
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

  const setLogsFormat = (logs) => {
    for (let i = 0; i < logs.length; i++) {
      const fullTimestamp = timestamp2string(logs[i].created_at);
      // Extract MM-DD HH:MM:SS from YYYY-MM-DD HH:MM:SS for compact display
      const compactTimestamp = fullTimestamp.slice(5); // Remove YYYY- part
      logs[i].timestamp2string = (
        <span title={fullTimestamp}>
          {compactTimestamp}
        </span>
      );
      logs[i].key = '' + logs[i].id;
    }
    // data.key = '' + data.id
    setLogs(logs);
    setLogCount(logs.length + ITEMS_PER_PAGE);
    // console.log(logCount);
  };

  const loadLogs = async (startIdx, pageSize, logType = 0) => {
    setLoading(true);

    let url = '';
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let sortParams = '';
    if (sortBy) {
      sortParams = `&sort_by=${sortBy}&sort_order=${sortOrder}`;
    }
    if (isAdminUser) {
      url = `/api/log/?p=${startIdx}&page_size=${pageSize}&type=${logType}&username=${username}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&channel=${channel}${sortParams}`;
    } else {
      url = `/api/log/self?p=${startIdx}&page_size=${pageSize}&type=${logType}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}${sortParams}`;
    }
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
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
    setLoading(false);
  };

  const pageData = logs.slice((activePage - 1) * pageSize, activePage * pageSize);

  const handlePageChange = page => {
    setActivePage(page);
    if (page === Math.ceil(logs.length / pageSize) + 1) {
      // In this case we have to load more data and then append them.
      loadLogs(page - 1, pageSize).then(r => {
      });
    }
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    loadLogs(0, size)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  const handleSort = async (columnKey) => {
    // Prevent multiple sort requests
    if (sortLoading) return;

    if (sortBy === columnKey) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(columnKey);
      setSortOrder('desc');
    }
    setActivePage(1);
    setSortLoading(true);

    try {
      await loadLogs(0, pageSize, logType);
    } finally {
      setSortLoading(false);
    }
  };

  const getSortIcon = (columnKey) => {
    if (columnKey !== sortBy) {
      return null;
    }
    return sortOrder === 'asc' ? ' ↑' : ' ↓';
  };

  const refresh = async (localLogType) => {
    // setLoading(true);
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

  const searchLogs = async () => {
    if (searchKeyword === '') {
      // if keyword is blank, load files instead.
      await loadLogs(0, pageSize);
      setActivePage(1);
      return;
    }
    setSearching(true);
    const res = await API.get(`/api/log/self/search?keyword=${searchKeyword}`);
    const { success, message, data } = res.data;
    if (success) {
      setLogs(data);
      setActivePage(1);
    } else {
      showError(message);
    }
    setSearching(false);
  };

  return (<>
    <Layout className="logs-layout">
      <Header className="logs-header">
        <Spin spinning={loadingStat}>
          <h3>使用明细（总消耗额度：
            <span onClick={handleEyeClick} style={{
              cursor: 'pointer', color: 'gray'
            }}>{showStat ? renderQuota(stat.quota) : '点击查看'}</span>
            {showStat && (
              <IconButton
                icon={<IconRefresh />}
                size="small"
                onClick={handleStatRefresh}
                loading={isStatRefreshing}
                disabled={isStatRefreshing}
                style={{ marginLeft: '8px' }}
                theme="borderless"
                title="刷新配额数据"
              />
            )}
            ）
          </h3>
        </Spin>
      </Header>
      <Form layout="horizontal" className="logs-form" style={{ marginTop: 10 }}>
        <>
          <Form.Input field="token_name" label="令牌名称" style={{ width: 176 }} value={token_name}
            placeholder={'可选值'} name="token_name"
            onChange={value => handleInputChange(value, 'token_name')} />
          <Form.Input field="model_name" label="模型名称" style={{ width: 176 }} value={model_name}
            placeholder="可选值"
            name="model_name"
            onChange={value => handleInputChange(value, 'model_name')} />
          <Form.DatePicker field="start_timestamp" label="起始时间" style={{ width: 272 }}
            initValue={start_timestamp}
            value={start_timestamp} type="dateTime"
            name="start_timestamp"
            onChange={value => handleInputChange(value, 'start_timestamp')} />
          <Form.DatePicker field="end_timestamp" fluid label="结束时间" style={{ width: 272 }}
            initValue={end_timestamp}
            value={end_timestamp} type="dateTime"
            name="end_timestamp"
            onChange={value => handleInputChange(value, 'end_timestamp')} />
          {isAdminUser && <>
            <Form.Input field="channel" label="渠道 ID" style={{ width: 176 }} value={channel}
              placeholder="可选值" name="channel"
              onChange={value => handleInputChange(value, 'channel')} />
            <Form.Field field="username" label="用户名称" style={{ width: 176 }}>
              <AutoComplete
                data={userOptions}
                value={username}
                placeholder="搜索用户名称"
                onSearch={searchUsers}
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
                      @{option.username} • ID: {option.id}
                    </div>
                  </div>
                )}
              />
            </Form.Field>
          </>}
          <Form.Section>
            <Button label="查询" type="primary" htmlType="submit" className="btn-margin-right"
              onClick={refresh} loading={loading}>查询</Button>
          </Form.Section>
        </>
      </Form>
      <Table
        className="logs-table"
        style={{ marginTop: 5 }}
        columns={columns}
        dataSource={pageData}
        onRow={(record) => ({
          onClick: () => handleRowClick(record),
          style: { cursor: 'pointer' }
        })}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: logCount,
          pageSizeOpts: [10, 20, 50, 100],
          showSizeChanger: true,
          onPageSizeChange: (size) => {
            handlePageSizeChange(size).then();
          },
          onPageChange: handlePageChange
        }} />
      <Select className="logs-select" defaultValue="0" style={{ width: 120 }} onChange={(value) => {
        setLogType(parseInt(value));
        refresh(parseInt(value)).then();
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
