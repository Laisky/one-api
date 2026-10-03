import { showError as reportUIError } from '../helpers/utils';
import React, { useState, useEffect } from 'react';
import { Modal, Table, Tag, Typography, Space, Spin, Notification, Divider, Card, Row, Col } from '@douyinfe/semi-ui';
import { IconClock, IconAlertCircle, IconPlay, IconSend, IconReply, IconCheckCircleStroked, IconFlag, IconArrowRight } from '@douyinfe/semi-icons';
import { API, timestamp2string } from '../helpers';

const { Title, Text } = Typography;

const TracingModal = ({ visible, onCancel, logId }) => {
  const [loading, setLoading] = useState(false);
  const [traceData, setTraceData] = useState(null);

  useEffect(() => {
    if (visible && logId) {
      fetchTraceData().catch(reportUIError);
    }
  }, [visible, logId]);

  const fetchTraceData = async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/trace/log/${logId}`);
      if (res.data.success) {
        setTraceData(res.data.data);
      } else {
        Notification.error({
          title: '错误',
          content: res.data.message || '获取追踪数据失败',
        });
      }
    } catch (err) {
      Notification.error({
        title: '错误',
        content: err.response?.data?.message || '获取追踪数据失败',
      });
    } finally {
      setLoading(false);
    }
  };

  const formatDuration = (milliseconds) => {
    if (!milliseconds) return 'N/A';
    if (milliseconds < 1000) {
      return `${milliseconds}ms`;
    }
    return `${(milliseconds / 1000).toFixed(2)}s`;
  };

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'N/A';
    return timestamp2string(Math.floor(timestamp / 1000));
  };

  const getStatusColor = (status) => {
    if (status >= 200 && status < 300) return 'green';
    if (status >= 300 && status < 400) return 'yellow';
    if (status >= 400 && status < 500) return 'orange';
    if (status >= 500) return 'red';
    return 'grey';
  };

  const getEventIcon = (eventType) => {
    switch (eventType) {
      case 'received': return <IconPlay style={{ color: '#1890ff' }} />;
      case 'forwarded': return <IconArrowRight style={{ color: '#13c2c2' }} />;
      case 'upstream_response': return <IconReply style={{ color: '#722ed1' }} />;
      case 'client_response': return <IconSend style={{ color: '#fa8c16' }} />;
      case 'upstream_completed': return <IconCheckCircleStroked style={{ color: '#52c41a' }} />;
      case 'completed': return <IconFlag style={{ color: '#52c41a' }} />;
      default: return <IconClock />;
    }
  };

  const renderTimeline = () => {
    if (!traceData?.timestamps) return null;

    const { timestamps, durations } = traceData;
    const timelineEvents = [];

    if (timestamps.request_received) {
      timelineEvents.push({
        key: 'received',
        title: '请求接收',
        timestamp: timestamps.request_received,
        duration: null,
      });
    }

    if (timestamps.request_forwarded) {
      timelineEvents.push({
        key: 'forwarded',
        title: '转发到上游',
        timestamp: timestamps.request_forwarded,
        duration: durations?.processing_time,
      });
    }

    if (timestamps.first_upstream_response) {
      timelineEvents.push({
        key: 'upstream_response',
        title: '上游首次响应',
        timestamp: timestamps.first_upstream_response,
        duration: durations?.upstream_response_time,
      });
    }

    if (timestamps.first_client_response) {
      timelineEvents.push({
        key: 'client_response',
        title: '客户端首次响应',
        timestamp: timestamps.first_client_response,
        duration: durations?.response_processing_time,
      });
    }

    if (timestamps.upstream_completed) {
      timelineEvents.push({
        key: 'upstream_completed',
        title: '上游完成',
        timestamp: timestamps.upstream_completed,
        duration: durations?.streaming_time,
      });
    }

    if (timestamps.request_completed) {
      timelineEvents.push({
        key: 'completed',
        title: '请求完成',
        timestamp: timestamps.request_completed,
        duration: null,
      });
    }

    const columns = [
      {
        title: '事件',
        dataIndex: 'title',
        key: 'title',
        render: (text, record) => (
          <Space>
            {getEventIcon(record.key)}
            <Text>{text}</Text>
          </Space>
        ),
      },
      {
        title: '时间戳',
        dataIndex: 'timestamp',
        key: 'timestamp',
        render: (timestamp) => (
          <Text code>{formatTimestamp(timestamp)}</Text>
        ),
      },
      {
        title: '耗时',
        dataIndex: 'duration',
        key: 'duration',
        render: (duration) => (
          <Text>{duration ? formatDuration(duration) : '-'}</Text>
        ),
      },
    ];

    return (
      <Card title={
        <Space>
          <IconClock />
          <Text>请求时间线</Text>
        </Space>
      }>
        <Table
          columns={columns}
          dataSource={timelineEvents}
          pagination={false}
          size="small"
        />

        {durations?.total_time && (
          <div style={{ marginTop: 16, padding: 12, backgroundColor: '#e6f7ff', borderRadius: 6 }}>
            <Text strong style={{ color: '#1890ff' }}>
              总请求时间: {formatDuration(durations.total_time)}
            </Text>
          </div>
        )}
      </Card>
    );
  };

  const renderRequestInfo = () => {
    if (!traceData) return null;

    return (
      <Card title={
        <Space>
          <IconAlertCircle />
          <Text>请求信息</Text>
        </Space>
      }>
        <Row gutter={16}>
          <Col span={12}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">URL</Text>
              <div style={{ marginTop: 4, wordBreak: 'break-all', fontFamily: 'monospace' }}>
                <Text code>{traceData.url}</Text>
              </div>
            </div>
          </Col>
          <Col span={6}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">方法</Text>
              <div style={{ marginTop: 4 }}>
                <Tag color="blue">{traceData.method}</Tag>
              </div>
            </div>
          </Col>
          <Col span={6}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">状态码</Text>
              <div style={{ marginTop: 4 }}>
                <Tag color={getStatusColor(traceData.status)}>
                  {traceData.status || 'N/A'}
                </Tag>
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">请求体大小</Text>
              <div style={{ marginTop: 4 }}>
                <Text>{traceData.body_size ? `${traceData.body_size} bytes` : 'N/A'}</Text>
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">用户</Text>
              <div style={{ marginTop: 4 }}>
                <Text>{traceData.log?.username || 'N/A'}</Text>
              </div>
            </div>
          </Col>
          <Col span={24}>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary">追踪ID</Text>
              <div style={{ marginTop: 4, fontFamily: 'monospace' }}>
                <Text code>{traceData.trace_id}</Text>
              </div>
            </div>
          </Col>
        </Row>
      </Card>
    );
  };

  return (
    <Modal
      title={
        <Space>
          <IconClock />
          <Text>请求追踪详情</Text>
        </Space>
      }
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={900}
      style={{ top: 20 }}
    >
      {loading && (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Spin size="large" />
          <div style={{ marginTop: 12 }}>
            <Text>加载追踪数据...</Text>
          </div>
        </div>
      )}

      {traceData && !loading && (
        <div>
          {renderRequestInfo()}
          <Divider />
          {renderTimeline()}
        </div>
      )}
    </Modal>
  );
};

export default TracingModal;
