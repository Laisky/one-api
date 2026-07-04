import React, { useEffect, useState } from 'react';
import { API, isMobile, shouldShowPrompt, showError, showInfo, showSuccess, timestamp2string } from '../helpers';

import { CHANNEL_OPTIONS, ITEMS_PER_PAGE } from '../constants';
import { renderGroup, renderNumberWithPoint, renderQuota } from '../helpers/render';
import {
  Button,
  Dropdown,
  Form,
  InputNumber,
  Popconfirm,
  Space,
  SplitButtonGroup,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography
} from '@douyinfe/semi-ui';
import EditChannel from '../pages/Channel/EditChannel';
import PricingModal from './PricingModal';
import { IconTreeTriangleDown } from '@douyinfe/semi-icons';

function renderTimestamp(timestamp) {
  return (
    <>
      {timestamp2string(timestamp)}
    </>
  );
}

let type2label = undefined;

const channelRef = (channel) => channel?.uuid || channel?.id || '';

function renderType(type) {
  if (!type2label) {
    type2label = new Map();
    for (let i = 0; i < CHANNEL_OPTIONS.length; i++) {
      type2label[CHANNEL_OPTIONS[i].value] = CHANNEL_OPTIONS[i];
    }
    type2label[0] = { value: 0, text: '未知类型', color: 'grey' };
  }
  return <Tag size="large" color={type2label[type]?.color}>{type2label[type]?.text}</Tag>;
}

const ChannelsTable = () => {
  const columns = [
    // {
    //     title: '',
    //     dataIndex: 'checkbox',
    //     className: 'checkbox',
    // },
    {
      title: '名称',
      dataIndex: 'name',
      render: (text, record) => (
        <Tooltip content={`ID: ${channelRef(record)}`}>
          <span className="resource-name-with-id">{text}</span>
        </Tooltip>
      )
    },
    // {
    //   title: '分组',
    //   dataIndex: 'group',
    //   render: (text, record, index) => {
    //     return (
    //       <div>
    //         <Space spacing={2}>
    //           {
    //             text.split(',').map((item, index) => {
    //               return (renderGroup(item));
    //             })
    //           }
    //         </Space>
    //       </div>
    //     );
    //   }
    // },
    {
      title: '类型',
      dataIndex: 'type',
      render: (text, record, index) => {
        return (
          <div>
            {renderType(text)}
          </div>
        );
      }
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: (text, record, index) => {
        return (
          <div>
            {renderStatus(text)}
          </div>
        );
      }
    },
    {
      title: '响应时间',
      dataIndex: 'response_time',
      render: (text, record, index) => {
        return (
          <div>
            {renderResponseTime(text)}
          </div>
        );
      }
    },
    {
      title: '已用/剩余',
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        return (
          <div>
            <Space spacing={1}>
              <Tooltip content={'已用额度'}>
                <Tag color="white" type="ghost" size="large">{renderQuota(record.used_quota)}</Tag>
              </Tooltip>
              <Tooltip content={'剩余额度' + record.balance + '，点击更新'}>
                <Tag color="white" type="ghost" size="large" onClick={() => {
                  updateChannelBalance(record);
                }}>${renderNumberWithPoint(record.balance)}</Tag>
              </Tooltip>
            </Space>
          </div>
        );
      }
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      render: (text, record, index) => {
        return (
          <div>
            <InputNumber
              style={{ width: 70 }}
              name="priority"
              onBlur={e => {
                manageChannel(channelRef(record), 'priority', record, e.target.value);
              }}
              keepFocus={true}
              innerButtons
              defaultValue={record.priority}
              min={-999}
            />
          </div>
        );
      }
    },
    // {
    //   title: '权重',
    //   dataIndex: 'weight',
    //   render: (text, record, index) => {
    //     return (
    //       <div>
    //         <InputNumber
    //           style={{ width: 70 }}
    //           name="weight"
    //           onBlur={e => {
    //             manageChannel(record.id, 'weight', record, e.target.value);
    //           }}
    //           keepFocus={true}
    //           innerButtons
    //           defaultValue={record.weight}
    //           min={0}
    //         />
    //       </div>
    //     );
    //   }
    // },
    {
      title: '',
      dataIndex: 'operate',
      render: (text, record, index) => (
        <div>
          {/* <SplitButtonGroup style={{ marginRight: 1 }} aria-label="测试操作项目组">
            <Button theme="light" onClick={() => {
              testChannel(record, '');
            }}>测试</Button>
            <Dropdown trigger="click" position="bottomRight" menu={record.test_models}
            >
              <Button style={{ padding: '8px 4px' }} type="primary" icon={<IconTreeTriangleDown />}></Button>
            </Dropdown>
          </SplitButtonGroup> */}
          <Button theme='light' type='primary' style={{ marginRight: 1 }} onClick={() => testChannel(record)}>测试</Button>
          <Popconfirm
            title="确定是否要删除此渠道？"
            content="此修改将不可逆"
            okType={'danger'}
            position={'left'}
            onConfirm={() => {
              manageChannel(channelRef(record), 'delete', record).then(
                () => {
                  removeRecord(channelRef(record));
                }
              );
            }}
          >
            <Button theme="light" type="danger" style={{ marginRight: 1 }}>删除</Button>
          </Popconfirm>
          {
            record.status === 1 ?
              <Button theme="light" type="warning" style={{ marginRight: 1 }} onClick={
                async () => {
                  manageChannel(
                    channelRef(record),
                    'disable',
                    record
                  );
                }
              }>禁用</Button> :
              <Button theme="light" type="secondary" style={{ marginRight: 1 }} onClick={
                async () => {
                  manageChannel(
                    channelRef(record),
                    'enable',
                    record
                  );
                }
              }>启用</Button>
          }
          <Button theme="light" type="tertiary" style={{ marginRight: 1 }} onClick={
            () => {
              setEditingChannel(record);
              setShowEdit(true);
            }
          }>编辑</Button>
          <Button theme="light" type="warning" style={{ marginRight: 1 }} onClick={
            () => openPricingModal(record)
          }>定价</Button>
        </div>
      )
    }
  ];

  const [channels, setChannels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [idSort, setIdSort] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searchGroup, setSearchGroup] = useState('');
  const [searchModel, setSearchModel] = useState('');
  const [searching, setSearching] = useState(false);
  const [updatingBalance, setUpdatingBalance] = useState(false);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [showPrompt, setShowPrompt] = useState(shouldShowPrompt('channel-test'));
  const [channelCount, setChannelCount] = useState(pageSize);
  const [groupOptions, setGroupOptions] = useState([]);
  const [showEdit, setShowEdit] = useState(false);
  const [enableBatchDelete, setEnableBatchDelete] = useState(false);
  const [editingChannel, setEditingChannel] = useState({
    id: undefined
  });
  const [selectedChannels, setSelectedChannels] = useState([]);
  const [pricingModalVisible, setPricingModalVisible] = useState(false);
  const [selectedChannel, setSelectedChannel] = useState(null);

  const removeRecord = id => {
    let newDataSource = [...channels];
    if (id != null) {
      let idx = newDataSource.findIndex(data => channelRef(data) === id || data.id === id);

      if (idx > -1) {
        newDataSource.splice(idx, 1);
        setChannels(newDataSource);
      }
    }
  };

  const setChannelFormat = (list, totalCount = 0) => {
    const formatted = Array.isArray(list) ? [...list] : [];
    for (let i = 0; i < formatted.length; i++) {
      formatted[i].key = '' + channelRef(formatted[i]);
      const testModels = [];
      const modelStr = formatted[i].models || '';
      modelStr.split(',').forEach((item) => {
        if (!item) {
          return;
        }
        testModels.push({
          node: 'item',
          name: item,
          onClick: () => {
            testChannel(formatted[i], item);
          }
        });
      });
      formatted[i].test_models = testModels;
    }
    setChannels(formatted);
    const resolvedTotal = totalCount > 0 ? totalCount : formatted.length;
    setChannelCount(resolvedTotal);
  };

  const loadChannels = async (startIdx, currentPageSize, sortByIdAsc) => {
    setLoading(true);
    try {
      const sortParams = sortByIdAsc ? '&sort=id&order=asc' : '&sort=id&order=desc';
      const res = await API.get(`/api/channel/?p=${startIdx}&size=${currentPageSize}${sortParams}`);
      const { success, message, data, total } = res.data;
      if (success) {
        const totalCount = typeof total === 'number' ? total : (Array.isArray(data) ? data.length : 0);
        if (startIdx === 0) {
          setChannelFormat(data, totalCount);
        } else {
          const pageData = Array.isArray(data) ? data : [];
          const newChannels = [...channels];
          newChannels.splice(startIdx * currentPageSize, pageData.length, ...pageData);
          setChannelFormat(newChannels, totalCount);
        }
      } else {
        showError(message);
      }
    } catch (err) {
      showError(err?.message || err);
    }
    setLoading(false);
  };

  const refresh = async () => {
    await loadChannels(activePage - 1, pageSize, idSort);
  };

  useEffect(() => {
    // console.log('default effect')
    const localIdSort = localStorage.getItem('id-sort') === 'true';
    const localPageSize = parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    setIdSort(localIdSort);
    setPageSize(localPageSize);
    loadChannels(0, localPageSize, localIdSort)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    fetchGroups().then();
  }, []);

  const manageChannel = async (id, action, record, value) => {
    let data = typeof id === 'string' ? { uuid: id } : { id };
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(`/api/channel/${id}`);
        break;
      case 'enable':
        data.status = 1;
        res = await API.put('/api/channel/?status_only=1', data);
        break;
      case 'disable':
        data.status = 2;
        res = await API.put('/api/channel/?status_only=1', data);
        break;
      case 'priority':
        if (value === '') {
          return;
        }
        data.priority = parseInt(value, 10);
        if (Number.isNaN(data.priority)) {
          showError('优先级必须是数字');
          return;
        }
        data.name = record?.name;
        if (!data.name) {
          const found = channels.find((item) => channelRef(item) === id || item.id === id);
          data.name = found?.name;
        }
        if (!data.name) {
          showError('未找到对应的渠道名称，稍后重试');
          return;
        }
        res = await API.put('/api/channel/', data);
        break;
      case 'weight':
        if (value === '') {
          return;
        }
        data.weight = parseInt(value, 10);
        if (data.weight < 0) {
          data.weight = 0;
        }
        data.name = record?.name;
        if (!data.name) {
          const found = channels.find((item) => channelRef(item) === id || item.id === id);
          data.name = found?.name;
        }
        if (!data.name) {
          showError('未找到对应的渠道名称，稍后重试');
          return;
        }
        res = await API.put('/api/channel/', data);
        break;
      default:
        showError(`未知操作类型: ${action}`);
        return;
    }
    const { success, message, data: responseData } = res.data;
    if (success) {
      showSuccess('操作成功完成！');
      const channel = responseData || {};
      let newChannels = [...channels];
      if (action === 'delete') {

      } else {
        if (action === 'enable') {
          record.status = 1;
        } else if (action === 'disable') {
          record.status = 2;
        } else if (action === 'priority') {
          record.priority = channel.priority ?? data.priority;
        } else if (action === 'weight') {
          record.weight = channel.weight ?? data.weight;
        }
      }
      setChannels(newChannels);
    } else {
      showError(message);
    }
  };

  const renderStatus = (status) => {
    switch (status) {
      case 1:
        return <Tag size="large" color="green">已启用</Tag>;
      case 2:
        return (
          <Tag size="large" color="yellow">
            已禁用
          </Tag>
        );
      case 3:
        return (
          <Tag size="large" color="yellow">
            自动禁用
          </Tag>
        );
      default:
        return (
          <Tag size="large" color="grey">
            未知状态
          </Tag>
        );
    }
  };

  const renderResponseTime = (responseTime) => {
    let time = responseTime / 1000;
    time = time.toFixed(2) + ' 秒';
    if (responseTime === 0) {
      return <Tag size="large" color="grey">未测试</Tag>;
    } else if (responseTime <= 1000) {
      return <Tag size="large" color="green">{time}</Tag>;
    } else if (responseTime <= 3000) {
      return <Tag size="large" color="lime">{time}</Tag>;
    } else if (responseTime <= 5000) {
      return <Tag size="large" color="yellow">{time}</Tag>;
    } else {
      return <Tag size="large" color="red">{time}</Tag>;
    }
  };

  const searchChannels = async (searchKeyword, searchGroup, searchModel) => {
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      // if keyword is blank, load files instead.
      await loadChannels(0, pageSize, idSort);
      setActivePage(1);
      return;
    }
    setSearching(true);
    try {
      const res = await API.get(`/api/channel/search?keyword=${searchKeyword}`);
      const { success, message, data } = res.data;
      if (success) {
        setChannelFormat(data, Array.isArray(data) ? data.length : 0);
        setActivePage(1);
      } else {
        showError(message);
      }
    } catch (err) {
      showError(err?.message || err);
    }
    setSearching(false);
  };

  const testChannel = async (record, model) => {
    const res = await API.get(`/api/channel/test/${channelRef(record)}?model=${model}`);
    const { success, message, time } = res.data;
    if (success) {
      record.response_time = time * 1000;
      record.test_time = Date.now() / 1000;
      showInfo(`渠道 ${record.name} 测试成功，耗时 ${time.toFixed(2)} 秒。`);
    } else {
      showError(message);
    }
  };

  const testChannels = async (scope) => {
    const res = await API.get(`/api/channel/test?scope=${scope}`);
    const { success, message } = res.data;
    if (success) {
      showInfo('已成功开始测试渠道，请刷新页面查看结果。');
    } else {
      showError(message);
    }
  };

  const deleteAllDisabledChannels = async () => {
    const res = await API.delete(`/api/channel/disabled`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(`已删除所有禁用渠道，共计 ${data} 个`);
      await refresh();
    } else {
      showError(message);
    }
  };

  const updateChannelBalance = async (record) => {
    const res = await API.get(`/api/channel/update_balance/${channelRef(record)}/`);
    const { success, message, balance } = res.data;
    if (success) {
      record.balance = balance;
      record.balance_updated_time = Date.now() / 1000;
      showInfo(`渠道 ${record.name} 余额更新成功！`);
    } else {
      showError(message);
    }
  };

  const updateAllChannelsBalance = async () => {
    setUpdatingBalance(true);
    const res = await API.get(`/api/channel/update_balance`);
    const { success, message } = res.data;
    if (success) {
      showInfo('已更新完毕所有已启用渠道余额！');
    } else {
      showError(message);
    }
    setUpdatingBalance(false);
  };

  const batchDeleteChannels = async () => {
    if (selectedChannels.length === 0) {
      showError('请先选择要删除的渠道！');
      return;
    }
    setLoading(true);
    let ids = [];
    selectedChannels.forEach((channel) => {
      ids.push(channelRef(channel));
    });
    const res = await API.post(`/api/channel/batch`, { ids: ids });
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(`已删除 ${data} 个渠道！`);
      await refresh();
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const fixChannelsAbilities = async () => {
    const res = await API.post(`/api/channel/fix`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(`已修复 ${data} 个渠道！`);
      await refresh();
    } else {
      showError(message);
    }
  };

  let pageData = channels.slice((activePage - 1) * pageSize, activePage * pageSize);

  const handlePageChange = page => {
    setActivePage(page);
    if (page === Math.ceil(channels.length / pageSize) + 1) {
      // In this case we have to load more data and then append them.
      loadChannels(page - 1, pageSize, idSort).then(r => {
      });
    }
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    loadChannels(0, size, idSort)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      // add 'all' option
      // res.data.data.unshift('all');
      setGroupOptions(res.data.data.map((group) => ({
        label: group,
        value: group
      })));
    } catch (error) {
      showError(error.message);
    }
  };

  const openPricingModal = (channel) => {
    setSelectedChannel(channel);
    setPricingModalVisible(true);
  };

  const closePricingModal = () => {
    setPricingModalVisible(false);
    setSelectedChannel(null);
  };

  const closeEdit = () => {
    setShowEdit(false);
  };

  const handleRow = (record, index) => {
    if (record.status !== 1) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)'
        }
      };
    } else {
      return {};
    }
  };


  return (
    <>
      <EditChannel refresh={refresh} visible={showEdit} handleClose={closeEdit} editingChannel={editingChannel} />
      <div style={{ display: "flex", placeItems: "center", justifyContent: "space-between" }}>
        <Form onSubmit={() => {
          searchChannels(searchKeyword, searchGroup, searchModel);
        }} labelPosition="left">
          <div style={{ display: 'flex' }}>
            <Space>
              <Form.Input
                field="search_keyword"
                label="搜索"
                placeholder="ID，名称和密钥 ..."
                value={searchKeyword}
                loading={searching}
                onChange={(v) => {
                  setSearchKeyword(v.trim());
                }}
              />
              {/* <Form.Input
              field="search_model"
              label="模型"
              placeholder="模型关键字"
              value={searchModel}
              loading={searching}
              onChange={(v) => {
                setSearchModel(v.trim());
              }}
            />
            <Form.Select field="group" label="分组" optionList={groupOptions} onChange={(v) => {
              setSearchGroup(v);
              searchChannels(searchKeyword, v, searchModel);
            }} /> */}
              <Button label="查询" type="primary" htmlType="submit" className="btn-margin-right"
                style={{ marginRight: 8 }}>查询</Button>
            </Space>
          </div>
        </Form>
        <div style={{
          display: isMobile() ? '' : 'flex',
          marginTop: isMobile() ? 0 : -45,
          zIndex: 999,
          position: 'relative',
          pointerEvents: 'none'
        }}>
          <Space style={{ pointerEvents: 'auto', marginTop: isMobile() ? 0 : 45 }}>
            <Button theme="light" type="primary" style={{ marginRight: 8 }} onClick={
              () => {
                setEditingChannel({
                  id: undefined
                });
                setShowEdit(true);
              }
            }>添加新的渠道</Button>
            <Popconfirm
              title="确定？"
              okType={'warning'}
              onConfirm={() => { testChannels("all") }}
              position={isMobile() ? 'top' : 'left'}
            >
              <Button theme="light" type="warning" style={{ marginRight: 8 }}>测试所有渠道</Button>
            </Popconfirm>
            <Popconfirm
              title="确定？"
              okType={'warning'}
              onConfirm={() => { testChannels("disabled") }}
              position={isMobile() ? 'top' : 'left'}
            >
              <Button theme="light" type="warning" style={{ marginRight: 8 }}>测试禁用渠道</Button>
            </Popconfirm>
            {/* <Popconfirm
            title="确定？"
            okType={'secondary'}
            onConfirm={updateAllChannelsBalance}
          >
            <Button theme="light" type="secondary" style={{ marginRight: 8 }}>更新所有已启用渠道余额</Button>
          </Popconfirm> */}
            <Popconfirm
              title="确定是否要删除禁用渠道？"
              content="此修改将不可逆"
              okType={'danger'}
              onConfirm={deleteAllDisabledChannels}
              position={isMobile() ? 'top' : 'left'}
            >
              <Button theme="light" type="danger" style={{ marginRight: 8 }}>删除禁用渠道</Button>
            </Popconfirm>

            <Button theme="light" type="primary" style={{ marginRight: 8 }} onClick={refresh}>刷新</Button>
          </Space>
          {/*<div style={{width: '100%', pointerEvents: 'none', position: 'absolute'}}>*/}

          {/*</div>*/}
        </div>
        {/* <div style={{ marginTop: 20 }}>
          <Space>
            <Typography.Text strong>开启批量删除</Typography.Text>
            <Switch label="开启批量删除" uncheckedText="关" aria-label="是否开启批量删除" onChange={(v) => {
              setEnableBatchDelete(v);
            }}></Switch>
            <Popconfirm
              title="确定是否要删除所选渠道？"
              content="此修改将不可逆"
              okType={'danger'}
              onConfirm={batchDeleteChannels}
              disabled={!enableBatchDelete}
              position={'top'}
            >
              <Button disabled={!enableBatchDelete} theme="light" type="danger"
                style={{ marginRight: 8 }}>删除所选渠道</Button>
            </Popconfirm>
            <Popconfirm
              title="确定是否要修复数据库一致性？"
              content="进行该操作时，可能导致渠道访问错误，请仅在数据库出现问题时使用"
              okType={'warning'}
              onConfirm={fixChannelsAbilities}
              position={'top'}
            >
              <Button theme="light" type="secondary" style={{ marginRight: 8 }}>修复数据库一致性</Button>
            </Popconfirm>
          </Space>
        </div>
        <div style={{ marginTop: 10, display: 'flex' }}>
          <Space>
            <Space>
              <Typography.Text strong>使用ID排序</Typography.Text>
              <Switch checked={idSort} label="使用ID排序" uncheckedText="关" aria-label="是否用ID排序" onChange={(v) => {
                localStorage.setItem('id-sort', v + '');
                setIdSort(v);
                loadChannels(0, pageSize, v)
                  .then()
                  .catch((reason) => {
                    showError(reason);
                  });
              }}></Switch>
            </Space>
          </Space>
        </div> */}
      </div>
      <Table className={'channel-table'} style={{ marginTop: 15 }} columns={columns} dataSource={pageData} pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: channelCount,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        formatPageText: (page) => '',
        onPageSizeChange: (size) => {
          handlePageSizeChange(size).then();
        },
        onPageChange: handlePageChange
      }} loading={loading} onRow={handleRow} rowSelection={
        enableBatchDelete ?
          {
            onChange: (selectedRowKeys, selectedRows) => {
              // console.log(`selectedRowKeys: ${selectedRowKeys}`, 'selectedRows: ', selectedRows);
              setSelectedChannels(selectedRows);
            }
          } : null
      } />
      <PricingModal
        visible={pricingModalVisible}
        onClose={closePricingModal}
        channelId={channelRef(selectedChannel)}
        channelName={selectedChannel?.name}
        channelType={selectedChannel?.type}
      />
    </>
  );
};

export default ChannelsTable;
