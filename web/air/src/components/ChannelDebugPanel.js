import React, { useState } from 'react';
import { Button, Modal, Typography, List, Tag, Notification } from '@douyinfe/semi-ui';
import { API } from '../helpers';

const { Title, Text } = Typography;

const ChannelDebugPanel = ({ channelId, channelType, channelName }) => {
  const [visible, setVisible] = useState(false);
  const [loading, setLoading] = useState(false);
  const [migrationStatus, setMigrationStatus] = useState(null);

  const showError = (message) => {
    Notification.error({
      title: '错误',
      content: message,
      duration: 3,
    });
  };

  const showSuccess = (message) => {
    Notification.success({
      title: '成功',
      content: message,
      duration: 3,
    });
  };

  const checkMigrationStatus = async () => {
    try {
      setLoading(true);
      try {
        const res = await API.get(`/api/debug/channel/${channelId}/migration-status`);
        if (res.data.success) {
          setMigrationStatus(res.data.data);
        } else {
          showError('获取迁移状态失败: ' + res.data.message);
        }
      } catch (error) {
        showError('检查迁移状态失败: ' + error.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const fixChannel = async () => {
    try {
      setLoading(true);
      try {
        const res = await API.post(`/api/debug/channel/${channelId}/fix`);
        if (res.data.success) {
          showSuccess('渠道修复成功，请刷新页面。');
          await checkMigrationStatus(); // Refresh status
        } else {
          showError('修复渠道失败: ' + res.data.message);
        }
      } catch (error) {
        showError('修复渠道失败: ' + error.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const debugChannel = async () => {
    try {
      setLoading(true);
      try {
        const res = await API.post(`/api/debug/channel/${channelId}/debug`);
        if (res.data.success) {
          showSuccess('调试信息已记录，请查看应用程序日志。');
        } else {
          showError('调试渠道失败: ' + res.data.message);
        }
      } catch (error) {
        showError('调试渠道失败: ' + error.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const getMigrationStatusColor = (status) => {
    switch (status) {
      case 'migrated': return 'green';
      case 'migrated_with_legacy': return 'orange';
      case 'needs_migration': return 'red';
      case 'empty': return 'grey';
      default: return 'grey';
    }
  };

  const getMigrationStatusText = (status) => {
    switch (status) {
      case 'migrated': return '完全迁移';
      case 'migrated_with_legacy': return '已迁移（存在遗留数据）';
      case 'needs_migration': return '需要迁移';
      case 'empty': return '无定价数据';
      default: return '未知';
    }
  };

  return (
    <>
      <Button 
        size="small" 
        theme="borderless"
        onClick={() => {
          setVisible(true);
          checkMigrationStatus();
        }}
        style={{ marginLeft: '10px' }}
      >
        调试
      </Button>

      <Modal
        title={`渠道调试: ${channelName} (ID: ${channelId})`}
        visible={visible}
        onCancel={() => setVisible(false)}
        width={600}
        footer={
          <div style={{ textAlign: 'right' }}>
            <Button onClick={debugChannel} loading={loading} style={{ marginRight: 8 }}>
              记录调试信息
            </Button>
            <Button onClick={checkMigrationStatus} loading={loading} style={{ marginRight: 8 }}>
              刷新状态
            </Button>
            {migrationStatus && migrationStatus.migration_status === 'needs_migration' && (
              <Button type="warning" onClick={fixChannel} loading={loading} style={{ marginRight: 8 }}>
                修复渠道
              </Button>
            )}
            <Button onClick={() => setVisible(false)}>
              关闭
            </Button>
          </div>
        }
      >
        <div style={{ padding: '16px 0' }}>
          <Title heading={5}>迁移状态</Title>
          
          {migrationStatus && (
            <>
              <Tag color={getMigrationStatusColor(migrationStatus.migration_status)} size="large">
                {getMigrationStatusText(migrationStatus.migration_status)}
              </Tag>
              
              <div style={{ marginTop: '15px' }}>
                <Text strong>渠道信息:</Text>
                <List size="small" style={{ marginTop: '8px' }}>
                  <List.Item>ID: {migrationStatus.channel_id}</List.Item>
                  <List.Item>名称: {migrationStatus.channel_name}</List.Item>
                  <List.Item>类型: {migrationStatus.channel_type}</List.Item>
                </List>
              </div>

              <div style={{ marginTop: '15px' }}>
                <Text strong>数据状态:</Text>
                <List size="small" style={{ marginTop: '8px' }}>
                  <List.Item>
                    <Tag color={migrationStatus.has_model_configs ? 'green' : 'red'} size="small">
                      {migrationStatus.has_model_configs ? '是' : '否'}
                    </Tag>
                    <span style={{ marginLeft: '8px' }}>
                      有模型配置（统一格式）
                      {migrationStatus.model_configs_count && (
                        <span> - {migrationStatus.model_configs_count} 个模型</span>
                      )}
                    </span>
                  </List.Item>
                  <List.Item>
                    <Tag color={migrationStatus.has_model_ratio ? 'orange' : 'grey'} size="small">
                      {migrationStatus.has_model_ratio ? '是' : '否'}
                    </Tag>
                    <span style={{ marginLeft: '8px' }}>
                      有模型比率（遗留）
                      {migrationStatus.model_ratio_count && (
                        <span> - {migrationStatus.model_ratio_count} 个模型</span>
                      )}
                    </span>
                  </List.Item>
                  <List.Item>
                    <Tag color={migrationStatus.has_completion_ratio ? 'orange' : 'grey'} size="small">
                      {migrationStatus.has_completion_ratio ? '是' : '否'}
                    </Tag>
                    <span style={{ marginLeft: '8px' }}>有完成比率（遗留）</span>
                  </List.Item>
                </List>
              </div>

              {migrationStatus.model_configs_models && (
                <div style={{ marginTop: '15px' }}>
                  <Text strong>统一配置中的模型:</Text>
                  <div style={{ marginTop: '5px' }}>
                    {migrationStatus.model_configs_models.map(model => (
                      <Tag key={model} size="small" style={{ margin: '2px' }}>
                        {model}
                      </Tag>
                    ))}
                  </div>
                </div>
              )}

              {migrationStatus.model_ratio_models && (
                <div style={{ marginTop: '15px' }}>
                  <Text strong>遗留比率中的模型:</Text>
                  <div style={{ marginTop: '5px' }}>
                    {migrationStatus.model_ratio_models.map(model => (
                      <Tag key={model} size="small" color="orange" style={{ margin: '2px' }}>
                        {model}
                      </Tag>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </Modal>
    </>
  );
};

export default ChannelDebugPanel;
